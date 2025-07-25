package main

import (
	"encoding/json"
	"flag"
	"fmt"
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

type Config struct {
	BrokerURL string
	Topic     string
	Command   string
	Username  string
	Password  string
	ClientID  string
}

func parseArgs() *Config {
	var config Config
	
	flag.StringVar(&config.BrokerURL, "L", "", "MQTT broker URL (e.g., mqtt://localhost:1883/topic)")
	flag.StringVar(&config.Command, "cmd", "", "Command to execute when topic is triggered")
	flag.StringVar(&config.Username, "u", "", "MQTT username (optional)")
	flag.StringVar(&config.Password, "p", "", "MQTT password (optional)")
	flag.StringVar(&config.ClientID, "client-id", "", "MQTT client ID (optional, will be generated if not provided)")
	
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -L <broker_url/topic> --cmd <command>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -L mqtt://localhost/host1/ping --cmd \"ping -c 4 1.1.1.1\"\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	
	flag.Parse()
	
	if config.BrokerURL == "" || config.Command == "" {
		flag.Usage()
		os.Exit(1)
	}
	
	return &config
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

func main() {
	config := parseArgs()
	
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
		
		// Subscribe to the topic
		token := client.Subscribe(config.Topic, 1, func(client mqtt.Client, msg mqtt.Message) {
			log.Printf("Received message on topic '%s': %s", msg.Topic(), string(msg.Payload()))
			
			// Execute the command
			output, status := executeCommand(config.Command)
			
			// Prepare result
			result := CommandResult{
				Output: output,
				Status: status,
			}
			
			// Convert to JSON
			jsonResult, err := json.Marshal(result)
			if err != nil {
				log.Printf("Error marshaling result: %v", err)
				return
			}
			
			// Publish to status topic
			statusTopic := config.Topic + "/status"
			token := client.Publish(statusTopic, 1, false, jsonResult)
			token.Wait()
			
			if token.Error() != nil {
				log.Printf("Error publishing to status topic: %v", token.Error())
			} else {
				log.Printf("Published result to topic '%s'", statusTopic)
			}
		})
		
		token.Wait()
		if token.Error() != nil {
			log.Fatalf("Failed to subscribe to topic '%s': %v", config.Topic, token.Error())
		}
		
		log.Printf("Subscribed to topic: %s", config.Topic)
		log.Printf("Will execute command: %s", config.Command)
		log.Printf("Results will be published to: %s/status", config.Topic)
	})
	
	// Create and start the client
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT broker: %v", token.Error())
	}
	
	log.Printf("MQTT Listener started")
	log.Printf("Broker: %s", broker)
	log.Printf("Topic: %s", config.Topic)
	log.Printf("Command: %s", config.Command)
	
	// Set up graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	
	// Wait for shutdown signal
	<-c
	log.Println("Shutting down...")
	
	client.Disconnect(250)
	log.Println("Disconnected from MQTT broker")
}