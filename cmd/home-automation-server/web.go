package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

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

// Rate limiting middleware
func (app *App) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		
		// Allow 60 requests per minute per IP
		if !globalRateLimiter.Allow(clientIP, 60, time.Minute) {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		
		next(w, r)
	}
}

func (app *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Config     Config
		Categories []Category
		Devices    []Device
		Title      string
	}{
		Config:     app.config,
		Categories: app.config.Categories,
		Devices:    app.config.Devices,
		Title:      "Home Automation Control",
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
		
		// Send health status
		healthMessage := WebSocketMessage{
			Type:     "health_update",
			DeviceID: deviceID,
			Data:     map[string]interface{}{"status": status.HealthStatus},
		}
		conn.WriteJSON(healthMessage)
	}
	app.statusMutex.RUnlock()

	// Send initial MQTT log to new client
	app.mqttLogMutex.RLock()
	for _, entry := range app.mqttLog {
		message := WebSocketMessage{
			Type: "mqtt_log",
			Data: entry,
		}
		conn.WriteJSON(message)
	}
	app.mqttLogMutex.RUnlock()

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
		Device       string      `json:"device"`
		Topic        string      `json:"topic"`
		Payload      string      `json:"payload"`
		LocalCommand string      `json:"localCommand"`
		Value        interface{} `json:"value"`
		ControlType  string      `json:"controlType"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Input validation
	if err := validateDeviceID(req.Device); err != nil {
		http.Error(w, fmt.Sprintf("Invalid device ID: %v", err), http.StatusBadRequest)
		return
	}

	if req.Topic != "" {
		if err := validateMQTTTopic(req.Topic); err != nil {
			http.Error(w, fmt.Sprintf("Invalid MQTT topic: %v", err), http.StatusBadRequest)
			return
		}
		
		if err := validateMQTTPayload(req.Payload); err != nil {
			http.Error(w, fmt.Sprintf("Invalid MQTT payload: %v", err), http.StatusBadRequest)
			return
		}
	}

	if req.LocalCommand != "" {
		if err := validateLocalCommand(req.LocalCommand); err != nil {
			http.Error(w, fmt.Sprintf("Invalid local command: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Find device and control for validation
	var device *Device
	var control *Control
	
	for i := range app.config.Devices {
		if app.config.Devices[i].ID == req.Device {
			device = &app.config.Devices[i]
			// Look for matching control
			for j := range app.config.Devices[i].Controls {
				c := &app.config.Devices[i].Controls[j]
				// Match by control type or topic
				if (req.ControlType != "" && c.Type == req.ControlType) || 
				   (req.Topic != "" && c.Topic == req.Topic) {
					control = c
					break
				}
			}
			break
		}
	}

	if device == nil {
		log.Printf("Device not found: %s", req.Device)
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	// Validate control value if provided
	if control != nil && req.Value != nil {
		if err := validateControlValue(*control, req.Value); err != nil {
			http.Error(w, fmt.Sprintf("Invalid control value: %v", err), http.StatusBadRequest)
			return
		}
	}

	// Sanitize inputs
	req.Topic = sanitizeInput(req.Topic)
	req.Payload = sanitizeInput(req.Payload)
	req.LocalCommand = sanitizeInput(req.LocalCommand)

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

		// Log the outgoing message
		app.addMQTTLogEntry(req.Topic+" (OUT)", req.Payload)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
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

func (app *App) handleAutomations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// Return automation status
		status := app.getAutomationStatus()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		
	case "POST":
		// Enable/disable automation or trigger manual execution
		var req struct {
			AutomationID string `json:"automationId"`
			Action       string `json:"action"` // "enable", "disable", "trigger"
		}
		
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		
		// Validate automation ID
		if req.AutomationID == "" {
			http.Error(w, "Automation ID required", http.StatusBadRequest)
			return
		}
		
		// Find automation in config
		var automation *Automation
		for _, a := range app.config.Automations {
			if a.ID == req.AutomationID {
				automation = &a
				break
			}
		}
		
		if automation == nil {
			http.Error(w, "Automation not found", http.StatusNotFound)
			return
		}
		
		switch req.Action {
		case "enable":
			automation.Enabled = true
			app.scheduleAutomation(*automation)
			log.Printf("Enabled automation: %s", automation.Name)
			
		case "disable":
			automation.Enabled = false
			app.stopAutomation(req.AutomationID)
			log.Printf("Disabled automation: %s", automation.Name)
			
		case "trigger":
			// Manual trigger
			if job, exists := app.automationJobs[req.AutomationID]; exists {
				go app.executeAutomation(job)
				log.Printf("Manually triggered automation: %s", automation.Name)
			} else {
				http.Error(w, "Automation not scheduled", http.StatusBadRequest)
				return
			}
			
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *App) handleDeviceHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	
	summary := app.getDeviceHealthSummary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
