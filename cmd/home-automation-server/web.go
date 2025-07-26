package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	uuid "github.com/google/uuid"
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

		// Log the outgoing message
		app.addMQTTLogEntry(req.Topic+" (OUT)", req.Payload)
	}

	w.WriteHeader(http.StatusOK)
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

