package main

import (
	"encoding/xml"
	"html/template"
	"sync"

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

