package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"time"
	"net/http"
	"path/filepath"

	"github.com/gorilla/websocket"
)

// Basic authentication middleware
func (app *App) basicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.config.Server.AuthEnabled {
			next(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok || username != app.config.Server.Username || password != app.config.Server.Password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Home Automation"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

// HTTPS redirect middleware
func httpsRedirect(w http.ResponseWriter, r *http.Request) {
	target := "https://" + r.Host + r.URL.Path
	if len(r.URL.RawQuery) > 0 {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}

func main() {
	// Parse command line flags
	configFile := flag.String("config", "config.xml", "Path to configuration file")
	suppressTimestamp := flag.Bool("no-timestamp", false, "Suppress timestamps in log output")
	webDir := flag.String("webdir", ".", "Parent directory containing 'static' and 'templates' subdirectories")
	enableWildcard := flag.Bool("log-all-mqtt", false, "Log all MQTT messages using wildcard subscription")
	flag.Parse()

	app := &App{
		deviceStatus:   make(map[string]*DeviceStatus),
		wsClients:      make(map[*websocket.Conn]bool),
		automationJobs: make(map[string]*AutomationJob),
		healthCheckers: make(map[string]*time.Ticker),
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

	// Set default values
	if app.config.Server.Port == 0 {
		app.config.Server.Port = 8080
	}
	if app.config.Server.TLSPort == 0 {
		app.config.Server.TLSPort = 8443
	}
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

	// Initialize device status and health monitoring
	app.initializeDeviceStatus()
	app.startHealthMonitoring()

	// Subscribe to status topics
	app.subscribeToStatusTopics()

	// Optionally subscribe to all messages for logging
	if *enableWildcard {
		app.subscribeToAllMessages()
	}

	// Start automation scheduler
	app.startAutomationScheduler()

	// Setup HTTP routes with authentication
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.basicAuthMiddleware(app.handleIndex))
	mux.HandleFunc("/ws", app.basicAuthMiddleware(app.handleWebSocket))
	mux.HandleFunc("/api/control", app.basicAuthMiddleware(app.handleControl))
	mux.HandleFunc("/api/status", app.basicAuthMiddleware(app.handleStatus))
	mux.HandleFunc("/api/system-stats", app.basicAuthMiddleware(app.handleSystemStats))
	mux.HandleFunc("/api/mqtt-log", app.basicAuthMiddleware(app.handleMQTTLog))
	mux.HandleFunc("/api/automations", app.basicAuthMiddleware(app.handleAutomations))
	mux.HandleFunc("/api/device-health", app.basicAuthMiddleware(app.handleDeviceHealth))

	// Serve static files
	staticDir := filepath.Join(app.webDir, "static")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	log.Printf("Using web directory: %s", app.webDir)
	log.Printf("Static files served from: %s", staticDir)

	// Start servers
	if app.config.Server.EnableTLS {
		// Validate TLS configuration
		if app.config.Server.CertFile == "" || app.config.Server.KeyFile == "" {
			log.Fatal("TLS enabled but cert/key files not specified")
		}

		// Create TLS configuration
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}

		// Start HTTPS server
		httpsServer := &http.Server{
			Addr:      fmt.Sprintf(":%d", app.config.Server.TLSPort),
			Handler:   mux,
			TLSConfig: tlsConfig,
		}

		go func() {
			log.Printf("Starting HTTPS server on port %d", app.config.Server.TLSPort)
			if err := httpsServer.ListenAndServeTLS(app.config.Server.CertFile, app.config.Server.KeyFile); err != nil {
				log.Fatalf("HTTPS server failed: %v", err)
			}
		}()

		// Start HTTP server for redirects
		httpServer := &http.Server{
			Addr:    fmt.Sprintf(":%d", app.config.Server.Port),
			Handler: http.HandlerFunc(httpsRedirect),
		}

		log.Printf("Starting HTTP redirect server on port %d", app.config.Server.Port)
		log.Fatal(httpServer.ListenAndServe())
	} else {
		// Start HTTP server only
		httpServer := &http.Server{
			Addr:    fmt.Sprintf(":%d", app.config.Server.Port),
			Handler: mux,
		}

		log.Printf("Starting HTTP server on port %d", app.config.Server.Port)
		if app.config.Server.AuthEnabled {
			log.Printf("Basic authentication enabled")
		}
		log.Fatal(httpServer.ListenAndServe())
	}
}
