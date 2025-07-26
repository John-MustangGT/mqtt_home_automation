package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type CommandResult struct {
	Output string `json:"output"`
	Status int    `json:"status"`
}

type StatusResponse struct {
	Status           string   `json:"status"`
	AvailableCommands []string `json:"available_commands"`
	Timestamp        string   `json:"timestamp"`
}

type Command struct {
	Name        string `xml:"name,attr"`
	Description string `xml:"description,attr"`
	Command     string `xml:",chardata"`
}

type Commands struct {
	XMLName  xml.Name  `xml:"commands"`
	Commands []Command `xml:"command"`
}

type Config struct {
	BrokerURL   string
	Topic       string
	Command     string
	Username    string
	Password    string
	ClientID    string
	ConfigFile  string
	Commands    map[string]string // map of command name to command string
}

func parseArgs() *Config {
	var config Config
	
	flag.StringVar(&config.BrokerURL, "L", "", "MQTT broker URL (e.g., mqtt://localhost:1883/topic)")
	flag.StringVar(&config.Command, "cmd", "", "Single command to execute when topic is triggered (legacy mode)")
	flag.StringVar(&config.ConfigFile, "config", "", "XML config file with multiple commands")
	flag.StringVar(&config.Username, "u", "", "MQTT username (optional)")
	flag.StringVar(&config.Password, "p", "", "MQTT password (optional)")
	flag.StringVar(&config.ClientID, "client-id", "", "MQTT client ID (optional, will be generated if not provided)")
	
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -L <broker_url/topic> [--cmd <command> | --config <xml_file>]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  Legacy mode: %s -L mqtt://localhost/host1 --cmd \"ping -c 4 1.1.1.1\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  XML mode:    %s -L mqtt://localhost/host1 --config commands.xml\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nXML mode behavior:\n")
		fmt.Fprintf(os.Stderr, "  - host1          -> returns status with available commands\n")
		fmt.Fprintf(os.Stderr, "  - host1/ping     -> executes 'ping' command if defined in XML\n")
		fmt.Fprintf(os.Stderr, "  - host1/invalid  -> returns error for undefined commands\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	
	flag.Parse()
	
	if config.BrokerURL == "" {
		flag.Usage()
		os.Exit(1)
	}
	
	if config.Command == "" && config.ConfigFile == "" {
		fmt.Fprintf(os.Stderr, "Error: Either --cmd or --config must be specified\n\n")
		flag.Usage()
		os.Exit(1)
	}
	
	if config.Command != "" && config.ConfigFile != "" {
		fmt.Fprintf(os.Stderr, "Error: Cannot specify both --cmd and --config\n\n")
		flag.Usage()
		os.Exit(1)
	}
	
	return &config
}

func loadXMLCommands(filename string) (map[string]string, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read XML file: %v", err)
	}
	
	var commands Commands
	err = xml.Unmarshal(data, &commands)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XML: %v", err)
	}
	
	cmdMap := make(map[string]string)
	for _, cmd := range commands.Commands {
		if cmd.Name == "" {
			continue
		}
		cmdMap[cmd.Name] = strings.TrimSpace(cmd.Command)
	}
	
	if len(cmdMap) == 0 {
		return nil, fmt.Errorf("no valid commands found in XML file")
	}
	
	return cmdMap, nil
}

func parseBrokerURL(brokerURL string) (string, string, error) {
	u, err := url.Parse(brokerURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid broker URL: %v", err)
	}
	
	if u.Scheme != "mqtt" && u.Scheme != "tcp" {
		return "", "", fmt.Errorf("unsupported scheme: %s (use mqtt:// or tcp://)", u.Scheme)
	}
	
	// Extract broker address
	broker := fmt.Sprintf("tcp://%s", u.Host)
	if u.Port() == "" {
		broker = fmt.Sprintf("tcp://%s:1883", u.Hostname())
	}
	
	// Extract topic from path
	topic := strings.TrimPrefix(u.Path, "/")
	if topic == "" {
		return "", "", fmt.Errorf("no topic specified in URL")
	}
	
	return broker, topic, nil
}

func executeCommand(cmd string) (string, int) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "No command specified", 1
	}
	
	command := exec.Command(parts[0], parts[1:]...)
	output, err := command.CombinedOutput()
	
	status := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			status = exitError.ExitCode()
		} else {
			status = 1
		}
	}
	
	return string(output), status
}

func getAvailableCommands(commands map[string]string) []string {
	var cmdNames []string
	for name := range commands {
		cmdNames = append(cmdNames, name)
	}
	return cmdNames
}

func handleMessage(config *Config, client mqtt.Client, msg mqtt.Message) {
	//topicParts := strings.Split(msg.Topic(), "/")
	baseTopic := config.Topic
	
	log.Printf("Received message on topic '%s': %s", msg.Topic(), string(msg.Payload()))
	
	var response interface{}
	var statusTopic string
	
	if config.ConfigFile != "" {
		// XML mode - handle multiple commands
		if msg.Topic() == baseTopic {
			// Base topic - return status
			statusResp := StatusResponse{
				Status:            "listening",
				AvailableCommands: getAvailableCommands(config.Commands),
				Timestamp:         time.Now().Format(time.RFC3339),
			}
			response = statusResp
			statusTopic = baseTopic + "/status"
		} else if strings.HasPrefix(msg.Topic(), baseTopic+"/") {
			// Subtopic - execute command
			cmdName := strings.TrimPrefix(msg.Topic(), baseTopic+"/")
			
			if cmdString, exists := config.Commands[cmdName]; exists {
				// Valid command - execute it
				output, status := executeCommand(cmdString)
				response = CommandResult{
					Output: output,
					Status: status,
				}
				log.Printf("Executed command '%s': %s", cmdName, cmdString)
			} else {
				// Invalid command
				response = CommandResult{
					Output: fmt.Sprintf("Error: Command '%s' not found. Available commands: %s", 
						cmdName, strings.Join(getAvailableCommands(config.Commands), ", ")),
					Status: 1,
				}
				log.Printf("Invalid command requested: %s", cmdName)
			}
			statusTopic = msg.Topic() + "/status"
		} else {
			// Topic doesn't match expected pattern
			log.Printf("Ignoring message on unexpected topic: %s", msg.Topic())
			return
		}
	} else {
		// Legacy mode - single command
		output, status := executeCommand(config.Command)
		response = CommandResult{
			Output: output,
			Status: status,
		}
		statusTopic = config.Topic + "/status"
	}
	
	// Convert response to JSON
	jsonResult, err := json.Marshal(response)
	if err != nil {
		log.Printf("Error marshaling result: %v", err)
		return
	}
	
	// Publish response
	token := client.Publish(statusTopic, 1, false, jsonResult)
	token.Wait()
	
	if token.Error() != nil {
		log.Printf("Error publishing to status topic: %v", token.Error())
	} else {
		log.Printf("Published result to topic '%s'", statusTopic)
	}
}

func main() {
	config := parseArgs()
	
	// Load XML commands if config file is specified
	if config.ConfigFile != "" {
		commands, err := loadXMLCommands(config.ConfigFile)
		if err != nil {
			log.Fatalf("Error loading XML config: %v", err)
		}
		config.Commands = commands
		log.Printf("Loaded %d commands from XML config", len(commands))
		for name := range commands {
			log.Printf("  - %s", name)
		}
	}
	
	// Parse broker URL and extract topic
	broker, topic, err := parseBrokerURL(config.BrokerURL)
	if err != nil {
		log.Fatalf("Error parsing broker URL: %v", err)
	}
	
	config.Topic = topic
	
	// Generate client ID if not provided
	if config.ClientID == "" {
		config.ClientID = fmt.Sprintf("mqtt_listener_%d", time.Now().Unix())
	}
	
	// Configure MQTT client options
	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(config.ClientID)
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(1 * time.Second)
	
	if config.Username != "" {
		opts.SetUsername(config.Username)
	}
	if config.Password != "" {
		opts.SetPassword(config.Password)
	}
	
	// Set up connection lost handler
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("Connection lost: %v", err)
	})
	
	// Set up reconnect handler
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Printf("Connected to MQTT broker")
		
		var subscribeTopics []string
		
		if config.ConfigFile != "" {
			// XML mode - subscribe to base topic and all subtopics
			subscribeTopics = []string{
				config.Topic,       // Base topic for status
				config.Topic + "/+", // All subtopics for commands
			}
		} else {
			// Legacy mode - subscribe to single topic
			subscribeTopics = []string{config.Topic}
		}
		
		for _, topic := range subscribeTopics {
			token := client.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				handleMessage(config, client, msg)
			})
			
			token.Wait()
			if token.Error() != nil {
				log.Fatalf("Failed to subscribe to topic '%s': %v", topic, token.Error())
			}
			log.Printf("Subscribed to topic: %s", topic)
		}
		
		if config.ConfigFile != "" {
			log.Printf("XML mode: Base topic '%s' returns status, subtopics execute commands", config.Topic)
		} else {
			log.Printf("Legacy mode: Will execute command: %s", config.Command)
		}
	})
	
	// Create and start the client
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}
	
	log.Printf("MQTT Listener started")
	log.Printf("Broker: %s", broker)
	log.Printf("Topic: %s", config.Topic)
	
	if config.ConfigFile != "" {
		log.Printf("Mode: XML config file (%s)", config.ConfigFile)
	} else {
		log.Printf("Mode: Legacy single command (%s)", config.Command)
	}
	
	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	// Wait for shutdown signal
	<-c
	log.Println("Shutting down...")
	
	client.Disconnect(250)
	log.Println("Disconnected from MQTT broker")
}
