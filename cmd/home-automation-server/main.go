package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"github.com/gorilla/websocket"
)

func main() {
	// Parse command line flags
	configFile := flag.String("config", "config.xml", "Path to configuration file")
	suppressTimestamp := flag.Bool("no-timestamp", false, "Suppress timestamps in log output")
	webDir := flag.String("webdir", ".", "Parent directory containing 'static' and 'templates' subdirectories")
	enableWildcard := flag.Bool("log-all-mqtt", false, "Log all MQTT messages using wildcard subscription")
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

	// Optionally subscribe to all messages for logging
	if *enableWildcard {
		app.subscribeToAllMessages()
	}

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

