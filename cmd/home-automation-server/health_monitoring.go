package main

import (
	"log"
	"time"
	"fmt"
)

func (app *App) startHealthMonitoring() {
	log.Println("Starting device health monitoring...")
	
	for _, device := range app.config.Devices {
		if device.HealthTopic != "" && device.HealthInterval > 0 {
			app.startDeviceHealthCheck(device)
		}
	}
}

func (app *App) startDeviceHealthCheck(device Device) {
	app.healthMutex.Lock()
	defer app.healthMutex.Unlock()
	
	// Stop existing health checker if it exists
	if ticker, exists := app.healthCheckers[device.ID]; exists {
		ticker.Stop()
	}
	
	// Set default timeout if not specified
	timeout := device.HealthTimeout
	if timeout <= 0 {
		timeout = device.HealthInterval * 2 // Default to 2x the interval
	}
	
	// Create ticker for health checks
	ticker := time.NewTicker(time.Duration(device.HealthInterval) * time.Second)
	app.healthCheckers[device.ID] = ticker
	
	log.Printf("Started health monitoring for device %s (interval: %ds, timeout: %ds)", 
		device.ID, device.HealthInterval, timeout)
	
	// Start health checking goroutine
	go func() {
		defer ticker.Stop()
		
		for {
			select {
			case <-ticker.C:
				app.checkDeviceHealth(device, timeout)
			}
		}
	}()
}

func (app *App) checkDeviceHealth(device Device, timeoutSeconds int) {
	app.statusMutex.RLock()
	deviceStatus, exists := app.deviceStatus[device.ID]
	app.statusMutex.RUnlock()
	
	if !exists {
		return
	}
	
	// Check if device has been seen recently
	timeout := time.Duration(timeoutSeconds) * time.Second
	timeSinceLastSeen := time.Since(deviceStatus.LastSeen)
	
	previousStatus := deviceStatus.HealthStatus
	var newStatus string
	
	if timeSinceLastSeen > timeout {
		newStatus = "offline"
	} else {
		newStatus = "online"
	}
	
	// Update status if it changed
	if newStatus != previousStatus {
		app.statusMutex.Lock()
		deviceStatus.HealthStatus = newStatus
		app.statusMutex.Unlock()
		
		log.Printf("Device %s health status changed: %s -> %s (last seen: %v ago)", 
			device.ID, previousStatus, newStatus, timeSinceLastSeen)
		
		// Broadcast health update
		app.broadcastHealthUpdate(device.ID, newStatus)
		
		// Log health status change
		app.addMQTTLogEntry(device.HealthTopic+" (HEALTH)", 
			fmt.Sprintf(`{"status":"%s","lastSeen":"%s","timeSinceLastSeen":"%v"}`, 
				newStatus, deviceStatus.LastSeen.Format(time.RFC3339), timeSinceLastSeen))
	}
}

func (app *App) stopHealthMonitoring() {
	app.healthMutex.Lock()
	defer app.healthMutex.Unlock()
	
	for deviceID, ticker := range app.healthCheckers {
		ticker.Stop()
		log.Printf("Stopped health monitoring for device %s", deviceID)
	}
	
	app.healthCheckers = make(map[string]*time.Ticker)
}

func (app *App) getDeviceHealthSummary() map[string]interface{} {
	app.statusMutex.RLock()
	defer app.statusMutex.RUnlock()
	
	summary := map[string]interface{}{
		"totalDevices":   len(app.deviceStatus),
		"onlineDevices":  0,
		"offlineDevices": 0,
		"unknownDevices": 0,
		"devices":        make(map[string]interface{}),
	}
	
	deviceDetails := summary["devices"].(map[string]interface{})
	
	for deviceID, status := range app.deviceStatus {
		deviceDetails[deviceID] = map[string]interface{}{
			"name":         status.Name,
			"category":     status.Category,
			"healthStatus": status.HealthStatus,
			"lastSeen":     status.LastSeen.Format(time.RFC3339),
			"timeSinceLastSeen": time.Since(status.LastSeen).String(),
		}
		
		switch status.HealthStatus {
		case "online":
			summary["onlineDevices"] = summary["onlineDevices"].(int) + 1
		case "offline":
			summary["offlineDevices"] = summary["offlineDevices"].(int) + 1
		default:
			summary["unknownDevices"] = summary["unknownDevices"].(int) + 1
		}
	}
	
	return summary
}
