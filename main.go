package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

// Configuration structures
type Config struct {
	XMLName    xml.Name   `xml:"config"`
	MQTT       MQTTConfig `xml:"mqtt"`
	Devices    []Device   `xml:"devices>device"`
	Categories []Category `xml:"categories>category"`
}

type MQTTConfig struct {
	Broker   string `xml:"broker,attr"`
	Port     int    `xml:"port,attr"`
	Username string `xml:"username,attr"`
	Password string `xml:"password,attr"`
	ClientID string `xml:"clientId,attr"`
}

type Device struct {
	ID          string   `xml:"id,attr"`
	Name        string   `xml:"name,attr"`
	Category    string   `xml:"category,attr"`
	StatusTopic string   `xml:"statusTopic"`
	Controls    []Control `xml:"controls>control"`
}

type Control struct {
	Type         string `xml:"type,attr"` // button, slider, toggle
	Label        string `xml:"label,attr"`
	Topic        string `xml:"topic,omitempty"`
	Payload      string `xml:"payload,omitempty"`
	LocalCommand string `xml:"localCommand,omitempty"`
	Min          int    `xml:"min,attr,omitempty"`
	Max          int    `xml:"max,attr,omitempty"`
}

type Category struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
	Icon string `xml:"icon,attr"`
}

// Runtime structures
type DeviceStatus struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Category string                 `json:"category"`
	Status   map[string]interface{} `json:"status"`
	Controls []Control              `json:"controls"`
}

type SystemStats struct {
	Uptime      string  `json:"uptime"`
	LoadAvg1    float64 `json:"loadAvg1"`
	LoadAvg5    float64 `json:"loadAvg5"`
	LoadAvg15   float64 `json:"loadAvg15"`
	MemoryUsed  float64 `json:"memoryUsed"`
	MemoryTotal float64 `json:"memoryTotal"`
	CPUCount    int     `json:"cpuCount"`
}

type WebSocketMessage struct {
	Type    string      `json:"type"`
	DeviceID string     `json:"deviceId,omitempty"`
	Data    interface{} `json:"data"`
}

// Application state
type App struct {
	config        Config
	mqttClient    mqtt.Client
	deviceStatus  map[string]*DeviceStatus
	statusMutex   sync.RWMutex
	wsClients     map[*websocket.Conn]bool
	wsMutex       sync.RWMutex
	wsUpgrader    websocket.Upgrader
	templates     *template.Template
}

func main() {
	app := &App{
		deviceStatus: make(map[string]*DeviceStatus),
		wsClients:    make(map[*websocket.Conn]bool),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	// Load configuration
	if err := app.loadConfig("config.xml"); err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Connect to MQTT
	if err := app.connectMQTT(); err != nil {
		log.Fatal("Failed to connect to MQTT:", err)
	}

	// Load HTML templates
	if err := app.loadTemplates(); err != nil {
		log.Fatal("Failed to load templates:", err)
	}

	// Initialize device status
	app.initializeDeviceStatus()

	// Subscribe to status topics
	app.subscribeToStatusTopics()

	// Setup HTTP routes
	http.HandleFunc("/", app.handleIndex)
	http.HandleFunc("/ws", app.handleWebSocket)
	http.HandleFunc("/api/control", app.handleControl)
	http.HandleFunc("/api/status", app.handleStatus)
	http.HandleFunc("/api/system-stats", app.handleSystemStats)
	
	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (app *App) loadConfig(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	return xml.Unmarshal(data, &app.config)
}

func (app *App) loadTemplates() error {
	var err error
	
	// Parse all HTML templates from the templates directory
	templatePattern := filepath.Join("templates", "*.html")
	app.templates, err = template.ParseGlob(templatePattern)
	if err != nil {
		return fmt.Errorf("failed to parse templates: %v", err)
	}
	
	log.Printf("Loaded templates: %v", app.templates.DefinedTemplates())
	return nil
}

func (app *App) connectMQTT() error {
	opts := mqtt.NewClientOptions()
	broker := fmt.Sprintf("tcp://%s:%d", app.config.MQTT.Broker, app.config.MQTT.Port)
	opts.AddBroker(broker)
	opts.SetClientID(app.config.MQTT.ClientID)
	opts.SetUsername(app.config.MQTT.Username)
	opts.SetPassword(app.config.MQTT.Password)

	// Set message callback
	opts.SetDefaultPublishHandler(app.onMQTTMessage)

	// Connection lost callback
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
	})

	// On connect callback
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Println("Connected to MQTT broker")
	})

	app.mqttClient = mqtt.NewClient(opts)
	
	if token := app.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

func (app *App) initializeDeviceStatus() {
	app.statusMutex.Lock()
	defer app.statusMutex.Unlock()

	for _, device := range app.config.Devices {
		app.deviceStatus[device.ID] = &DeviceStatus{
			ID:       device.ID,
			Name:     device.Name,
			Category: device.Category,
			Status:   make(map[string]interface{}),
			Controls: device.Controls,
		}
	}
}

func (app *App) subscribeToStatusTopics() {
	for _, device := range app.config.Devices {
		if device.StatusTopic != "" {
			topic := device.StatusTopic
			deviceID := device.ID
			
			token := app.mqttClient.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				app.handleStatusUpdate(deviceID, msg.Topic(), string(msg.Payload()))
			})
			
			if token.Wait() && token.Error() != nil {
				log.Printf("Failed to subscribe to %s: %v", topic, token.Error())
			} else {
				log.Printf("Subscribed to status topic: %s for device: %s", topic, deviceID)
			}
		}
	}
}

func (app *App) onMQTTMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received MQTT message on topic %s: %s", msg.Topic(), string(msg.Payload()))
}

func (app *App) handleStatusUpdate(deviceID, topic, payload string) {
	app.statusMutex.Lock()
	defer app.statusMutex.Unlock()

	if deviceStatus, exists := app.deviceStatus[deviceID]; exists {
		// Try to parse as JSON, fallback to string
		var jsonData interface{}
		if err := json.Unmarshal([]byte(payload), &jsonData); err != nil {
			deviceStatus.Status["value"] = payload
		} else {
			deviceStatus.Status = jsonData.(map[string]interface{})
		}
		
		deviceStatus.Status["lastUpdate"] = time.Now().Format(time.RFC3339)
		
		// Broadcast update to WebSocket clients
		app.broadcastUpdate(deviceID, deviceStatus.Status)
	}
}

func (app *App) broadcastUpdate(deviceID string, status map[string]interface{}) {
	app.wsMutex.RLock()
	defer app.wsMutex.RUnlock()

	message := WebSocketMessage{
		Type:     "status_update",
		DeviceID: deviceID,
		Data:     status,
	}

	for client := range app.wsClients {
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Error sending WebSocket message: %v", err)
			client.Close()
			delete(app.wsClients, client)
		}
	}
}

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Config     Config
		Categories []Category
		Devices    []Device
		Title      string
		ID	   string
	}{
		Config:     app.config,
		Categories: app.config.Categories,
		Devices:    app.config.Devices,
		Title:      "Home Automation Control",
		ID:	    "StaticID",
	}

	if err := app.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("Template execution error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func (app *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := app.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	app.wsMutex.Lock()
	app.wsClients[conn] = true
	app.wsMutex.Unlock()

	// Send initial status to new client
	app.statusMutex.RLock()
	for deviceID, status := range app.deviceStatus {
		message := WebSocketMessage{
			Type:     "status_update",
			DeviceID: deviceID,
			Data:     status.Status,
		}
		conn.WriteJSON(message)
	}
	app.statusMutex.RUnlock()

	// Keep connection alive
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			app.wsMutex.Lock()
			delete(app.wsClients, conn)
			app.wsMutex.Unlock()
			break
		}
	}
}

func (app *App) handleControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Device       string `json:"device"`
		Topic        string `json:"topic"`
		Payload      string `json:"payload"`
		LocalCommand string `json:"localCommand"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Execute local command if specified
	if req.LocalCommand != "" {
		go app.executeLocalCommand(req.LocalCommand)
	}

	// Send MQTT command if topic is specified
	if req.Topic != "" {
		token := app.mqttClient.Publish(req.Topic, 1, false, req.Payload)
		if token.Wait() && token.Error() != nil {
			log.Printf("Failed to publish MQTT message: %v", token.Error())
			http.Error(w, "Failed to send MQTT command", http.StatusInternalServerError)
			return
		}
		log.Printf("Sent MQTT command - Topic: %s, Payload: %s", req.Topic, req.Payload)
	}

	w.WriteHeader(http.StatusOK)
}

func (app *App) executeLocalCommand(command string) {
	log.Printf("Executing local command: %s", command)
	
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		log.Printf("Local command failed: %v, Output: %s", err, string(output))
	} else {
		log.Printf("Local command executed successfully. Output: %s", string(output))
	}
}

func (app *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	app.statusMutex.RLock()
	defer app.statusMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app.deviceStatus)
}

func (app *App) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	stats := app.getSystemStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (app *App) getSystemStats() SystemStats {
	stats := SystemStats{}
	
	// Get uptime
	if output, err := exec.Command("uptime", "-p").Output(); err == nil {
		stats.Uptime = strings.TrimSpace(string(output))
	}
	
	// Get load average
	if output, err := exec.Command("cat", "/proc/loadavg").Output(); err == nil {
		fields := strings.Fields(string(output))
		if len(fields) >= 3 {
			if val, err := strconv.ParseFloat(fields[0], 64); err == nil {
				stats.LoadAvg1 = val
			}
			if val, err := strconv.ParseFloat(fields[1], 64); err == nil {
				stats.LoadAvg5 = val
			}
			if val, err := strconv.ParseFloat(fields[2], 64); err == nil {
				stats.LoadAvg15 = val
			}
		}
	}
	
	// Get memory info
	if output, err := exec.Command("free", "-m").Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			// Parse memory line: Mem: total used free shared buff/cache available
			memLine := regexp.MustCompile(`\s+`).Split(lines[1], -1)
			if len(memLine) >= 3 {
				if total, err := strconv.ParseFloat(memLine[1], 64); err == nil {
					stats.MemoryTotal = total
				}
				if used, err := strconv.ParseFloat(memLine[2], 64); err == nil {
					stats.MemoryUsed = used
				}
			}
		}
	}
	
	// Get CPU count
	if output, err := exec.Command("nproc").Output(); err == nil {
		if cpus, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil {
			stats.CPUCount = cpus
		}
	}
	
	return stats
}
