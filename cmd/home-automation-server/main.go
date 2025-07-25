package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
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

	uuid "github.com/google/uuid"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

// Configuration structures
type Config struct {
	XMLName           xml.Name   `xml:"config"`
	MQTT              MQTTConfig `xml:"mqtt"`
	Devices           []Device   `xml:"devices>device"`
	Categories        []Category `xml:"categories>category"`
	SuppressTimestamp bool       `xml:"suppressTimestamp,attr"`
	MQTTLogSize       int        `xml:"mqttLogSize,attr"`
}

type MQTTConfig struct {
	Broker        string `xml:"broker,attr"`
	Port          int    `xml:"port,attr"`
	Username      string `xml:"username,attr"`
	Password      string `xml:"password,attr"`
	ClientID      string `xml:"clientId,attr"`
	RetryInterval int    `xml:"retryInterval,attr"` // seconds between connection attempts
	MaxRetries    int    `xml:"maxRetries,attr"`    // 0 = infinite retries
}

type Device struct {
	ID          string    `xml:"id,attr"`
	Name        string    `xml:"name,attr"`
	Category    string    `xml:"category,attr"`
	StatusTopic string    `xml:"statusTopic"`
	Controls    []Control `xml:"controls>control"`
}

type Control struct {
	Type         string `xml:"type,attr"` // button, slider, toggle
	Label        string `xml:"label,attr"`
	Topic        string `xml:"topic,attr,omitempty"`
	Payload      string `xml:"payload,attr,omitempty"`
	LocalCommand string `xml:"localCommand,attr,omitempty"`
	Min          int    `xml:"min,attr,omitempty"`
	Max          int    `xml:"max,attr,omitempty"`
}

type Category struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
	Icon string `xml:"icon,attr"`
}

type MQTTLogEntry struct {
	Timestamp string `json:"timestamp"`
	Topic     string `json:"topic"`
	Payload   string `json:"payload"`
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
	Type     string      `json:"type"`
	DeviceID string      `json:"deviceId,omitempty"`
	Data     interface{} `json:"data"`
}

// Application state
type App struct {
	config       Config
	mqttClient   mqtt.Client
	deviceStatus map[string]*DeviceStatus
	statusMutex  sync.RWMutex
	wsClients    map[*websocket.Conn]bool
	wsMutex      sync.RWMutex
	wsUpgrader   websocket.Upgrader
	templates    *template.Template
	webDir       string
	mqttLog      []MQTTLogEntry
	mqttLogMutex sync.RWMutex
}

func main() {
	// Parse command line flags
	configFile := flag.String("config", "config.xml", "Path to configuration file")
	suppressTimestamp := flag.Bool("no-timestamp", false, "Suppress timestamps in log output")
	webDir := flag.String("webdir", ".", "Parent directory containing 'static' and 'templates' subdirectories")
	flag.Parse()

	app := &App{
		deviceStatus: make(map[string]*DeviceStatus),
		wsClients:    make(map[*websocket.Conn]bool),
		wsUpgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		webDir: *webDir,
	}

	// Load configuration
	if err := app.loadConfig(*configFile); err != nil {
		log.Fatalf("Failed to load config from '%s': %v", *configFile, err)
	}

	// Configure logging based on command line flag or XML config
	if *suppressTimestamp || app.config.SuppressTimestamp {
		log.SetFlags(0) // Remove all flags including timestamp
	}

	// Set default MQTT retry values if not specified
	if app.config.MQTT.RetryInterval == 0 {
		app.config.MQTT.RetryInterval = 5 // default 5 seconds
	}
	// MaxRetries of 0 means infinite retries (default behavior)

	// Connect to MQTT with retry logic
	if err := app.connectMQTTWithRetry(); err != nil {
		log.Fatal("Failed to connect to MQTT after all retries:", err)
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
	http.HandleFunc("/api/mqtt-log", app.handleMQTTLog)

	// Serve static files
	staticDir := filepath.Join(app.webDir, "static")
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	log.Printf("Using web directory: %s", app.webDir)
	log.Printf("Static files served from: %s", staticDir)
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (app *App) loadConfig(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read config file '%s': %v", filename, err)
	}

	if err := xml.Unmarshal(data, &app.config); err != nil {
		return fmt.Errorf("failed to parse XML config: %v", err)
	}

	// Set default MQTT log size if not specified
	if app.config.MQTTLogSize <= 0 {
		app.config.MQTTLogSize = 20
	}

	// Debug: Print parsed configuration
	log.Printf("Loaded configuration from: %s", filename)
	log.Printf("Loaded %d devices", len(app.config.Devices))
	for _, device := range app.config.Devices {
		log.Printf("Device: %s (%s)", device.Name, device.ID)
		for i, control := range device.Controls {
			log.Printf("  Control %d: Type=%s, Label=%s, Topic='%s', Payload='%s', LocalCommand='%s'",
				i, control.Type, control.Label, control.Topic, control.Payload, control.LocalCommand)
		}
	}

	return nil
}

func (app *App) connectMQTTWithRetry() error {
	retryCount := 0
	
	for {
		err := app.connectMQTT()
		if err == nil {
			return nil // Success
		}

		retryCount++
		
		// Check if we've exceeded max retries (0 means infinite)
		if app.config.MQTT.MaxRetries > 0 && retryCount >= app.config.MQTT.MaxRetries {
			return fmt.Errorf("failed to connect to MQTT after %d attempts: %v", retryCount, err)
		}

		log.Printf("Failed to connect to MQTT (attempt %d): %v", retryCount, err)
		log.Printf("Waiting %d seconds before retry...", app.config.MQTT.RetryInterval)
		
		time.Sleep(time.Duration(app.config.MQTT.RetryInterval) * time.Second)
	}
}

func (app *App) loadTemplates() error {
	var err error

	// Create template with custom functions
	funcMap := template.FuncMap{
		"safeAttr": func(s string) string {
			// Escape single quotes and ensure empty strings are properly handled
			if s == "" {
				return ""
			}
			return strings.ReplaceAll(s, "'", "\\'")
		},
	}

	// Parse all HTML templates from the templates directory
	templateDir := filepath.Join(app.webDir, "templates")
	templatePattern := filepath.Join(templateDir, "*.html")
	app.templates, err = template.New("").Funcs(funcMap).ParseGlob(templatePattern)
	if err != nil {
		return fmt.Errorf("failed to parse templates from '%s': %v", templateDir, err)
	}

	log.Printf("Loaded templates from: %s", templateDir)
	log.Printf("Available templates: %v", app.templates.DefinedTemplates())
	return nil
}

func (app *App) connectMQTT() error {
	opts := mqtt.NewClientOptions()
	broker := fmt.Sprintf("tcp://%s:%d", app.config.MQTT.Broker, app.config.MQTT.Port)
	opts.AddBroker(broker)
	opts.SetClientID(app.config.MQTT.ClientID)
	opts.SetUsername(app.config.MQTT.Username)
	opts.SetPassword(app.config.MQTT.Password)

	// Set connection timeout
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetPingTimeout(5 * time.Second)

	// Set message callback
	opts.SetDefaultPublishHandler(app.onMQTTMessage)

	// Connection lost callback with reconnection logic
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("MQTT connection lost: %v", err)
		log.Println("Attempting to reconnect to MQTT broker...")
		go app.reconnectMQTT()
	})

	// On connect callback
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Println("Connected to MQTT broker")
		// Resubscribe to status topics after reconnection
		app.subscribeToStatusTopics()
	})

	// Enable automatic reconnection
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(time.Duration(app.config.MQTT.RetryInterval) * time.Second)

	app.mqttClient = mqtt.NewClient(opts)

	log.Printf("Attempting to connect to MQTT broker at %s...", broker)
	if token := app.mqttClient.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

func (app *App) reconnectMQTT() {
	retryCount := 0
	
	for !app.mqttClient.IsConnected() {
		retryCount++
		log.Printf("MQTT reconnection attempt %d...", retryCount)
		
		time.Sleep(time.Duration(app.config.MQTT.RetryInterval) * time.Second)
		
		// The MQTT client will handle reconnection automatically
		// We just need to wait and log the attempts
		if app.mqttClient.IsConnected() {
			log.Println("MQTT reconnection successful")
			return
		}
	}
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
	topic := msg.Topic()
	payload := string(msg.Payload())
	
	log.Printf("Received MQTT message on topic %s: %s", topic, payload)
	app.addMQTTLogEntry(topic, payload)
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

func (app *App) addMQTTLogEntry(topic, payload string) {
	app.mqttLogMutex.Lock()
	defer app.mqttLogMutex.Unlock()

	// Create new log entry
	entry := MQTTLogEntry{
		Timestamp: time.Now().Format("15:04:05"),
		Topic:     topic,
		Payload:   payload,
	}

	// Add to beginning of slice
	app.mqttLog = append([]MQTTLogEntry{entry}, app.mqttLog...)

	// Trim to max size
	maxSize := app.config.MQTTLogSize
	if maxSize <= 0 {
		maxSize = 20 // default
	}
	if len(app.mqttLog) > maxSize {
		app.mqttLog = app.mqttLog[:maxSize]
	}

	// Broadcast to WebSocket clients
	app.broadcastMQTTLog(entry)
}

func (app *App) broadcastMQTTLog(entry MQTTLogEntry) {
	app.wsMutex.RLock()
	defer app.wsMutex.RUnlock()

	message := WebSocketMessage{
		Type: "mqtt_log",
		Data: entry,
	}

	for client := range app.wsClients {
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Error sending MQTT log WebSocket message: %v", err)
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
		ID         string
	}{
		Config:     app.config,
		Categories: app.config.Categories,
		Devices:    app.config.Devices,
		Title:      "Home Automation Control",
		ID:         uuid.NewString(),
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

	log.Printf("Received control request: Device=%s, Topic=%s, Payload=%s, LocalCommand=%s",
		req.Device, req.Topic, req.Payload, req.LocalCommand)

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

func (app *App) handleMQTTLog(w http.ResponseWriter, r *http.Request) {
	app.mqttLogMutex.RLock()
	defer app.mqttLogMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(app.mqttLog)
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