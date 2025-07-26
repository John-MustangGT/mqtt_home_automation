package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

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

func (app *App) subscribeToAllMessages() {
	// Subscribe to all topics with wildcard
	token := app.mqttClient.Subscribe("#", 0, func(client mqtt.Client, msg mqtt.Message) {
		app.addMQTTLogEntry(msg.Topic(), string(msg.Payload()))
	})

	if token.Wait() && token.Error() != nil {
		log.Printf("Failed to subscribe to wildcard topic: %v", token.Error())
	} else {
		log.Printf("Subscribed to wildcard topic for MQTT logging")
	}
}

func (app *App) subscribeToStatusTopics() {
	for _, device := range app.config.Devices {
		if device.StatusTopic != "" {
			topic := device.StatusTopic
			deviceID := device.ID

			token := app.mqttClient.Subscribe(topic, 1, func(client mqtt.Client, msg mqtt.Message) {
				// Add MQTT logging here
				app.addMQTTLogEntry(msg.Topic(), string(msg.Payload()))
				// Handle the status update
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
