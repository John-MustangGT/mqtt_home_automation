package main

import (
	"encoding/xml"
	"html/template"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

// Configuration structures
type Config struct {
	XMLName           xml.Name         `xml:"config"`
	Server            ServerConfig     `xml:"server"`
	MQTT              MQTTConfig       `xml:"mqtt"`
	Devices           []Device         `xml:"devices>device"`
	Categories        []Category       `xml:"categories>category"`
	Automations       []Automation     `xml:"automations>automation"`
	SuppressTimestamp bool             `xml:"suppressTimestamp,attr"`
	MQTTLogSize       int              `xml:"mqttLogSize,attr"`
}

type ServerConfig struct {
	EnableTLS    bool   `xml:"enableTLS,attr"`
	CertFile     string `xml:"certFile,attr"`
	KeyFile      string `xml:"keyFile,attr"`
	Port         int    `xml:"port,attr"`
	TLSPort      int    `xml:"tlsPort,attr"`
	AuthEnabled  bool   `xml:"authEnabled,attr"`
	Username     string `xml:"username,attr"`
	Password     string `xml:"password,attr"`
}

type MQTTConfig struct {
	Broker        string `xml:"broker,attr"`
	Port          int    `xml:"port,attr"`
	Username      string `xml:"username,attr"`
	Password      string `xml:"password,attr"`
	ClientID      string `xml:"clientId,attr"`
	RetryInterval int    `xml:"retryInterval,attr"` // seconds between connection attempts
	MaxRetries    int    `xml:"maxRetries,attr"`    // 0 = infinite retries
	EnableTLS     bool   `xml:"enableTLS,attr"`
	CAFile        string `xml:"caFile,attr"`
	CertFile      string `xml:"certFile,attr"`
	KeyFile       string `xml:"keyFile,attr"`
	InsecureSkip  bool   `xml:"insecureSkipVerify,attr"`
}

type Device struct {
	ID              string    `xml:"id,attr"`
	Name            string    `xml:"name,attr"`
	Category        string    `xml:"category,attr"`
	StatusTopic     string    `xml:"statusTopic"`
	HealthTopic     string    `xml:"healthTopic"`
	HealthInterval  int       `xml:"healthInterval,attr"` // seconds
	HealthTimeout   int       `xml:"healthTimeout,attr"`  // seconds
	Controls        []Control `xml:"controls>control"`
}

type Control struct {
	Type         string `xml:"type,attr"` // button, slider, toggle
	Label        string `xml:"label,attr"`
	Topic        string `xml:"topic,attr,omitempty"`
	Payload      string `xml:"payload,attr,omitempty"`
	LocalCommand string `xml:"localCommand,attr,omitempty"`
	Min          int    `xml:"min,attr,omitempty"`
	Max          int    `xml:"max,attr,omitempty"`
	// Input validation
	MinValue     *float64 `xml:"minValue,attr,omitempty"`
	MaxValue     *float64 `xml:"maxValue,attr,omitempty"`
	AllowedValues []string `xml:"allowedValues,attr,omitempty"`
}

type Category struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"name,attr"`
	Icon string `xml:"icon,attr"`
}

type Automation struct {
	ID          string      `xml:"id,attr"`
	Name        string      `xml:"name,attr"`
	Enabled     bool        `xml:"enabled,attr"`
	DeviceID    string      `xml:"deviceId,attr"`
	ControlType string      `xml:"controlType,attr"`
	Schedule    Schedule    `xml:"schedule"`
	Action      AutoAction  `xml:"action"`
}

type Schedule struct {
	Type      string `xml:"type,attr"` // time, interval, duration
	Time      string `xml:"time,attr,omitempty"` // HH:MM format for daily execution
	Interval  string `xml:"interval,attr,omitempty"` // e.g., "1h", "30m", "10s"
	Duration  string `xml:"duration,attr,omitempty"` // how long to run
	Days      string `xml:"days,attr,omitempty"` // comma-separated: mon,tue,wed,thu,fri,sat,sun
	StartDate string `xml:"startDate,attr,omitempty"` // YYYY-MM-DD
	EndDate   string `xml:"endDate,attr,omitempty"`   // YYYY-MM-DD
}

type AutoAction struct {
	Topic        string `xml:"topic,attr"`
	Payload      string `xml:"payload,attr,omitempty"`
	LocalCommand string `xml:"localCommand,attr,omitempty"`
	OnPayload    string `xml:"onPayload,attr,omitempty"`  // payload to turn on
	OffPayload   string `xml:"offPayload,attr,omitempty"` // payload to turn off
}

type MQTTLogEntry struct {
	Timestamp string `json:"timestamp"`
	Topic     string `json:"topic"`
	Payload   string `json:"payload"`
}

// Runtime structures
type DeviceStatus struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Category     string                 `json:"category"`
	Status       map[string]interface{} `json:"status"`
	Controls     []Control              `json:"controls"`
	LastSeen     time.Time              `json:"lastSeen"`
	HealthStatus string                 `json:"healthStatus"` // "online", "offline", "unknown"
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

type AutomationJob struct {
	ID         string
	Automation Automation
	NextRun    time.Time
	Running    bool
	Timer      *time.Timer
	StopTimer  *time.Timer
}

// Application state
type App struct {
	config          Config
	mqttClient      mqtt.Client
	deviceStatus    map[string]*DeviceStatus
	statusMutex     sync.RWMutex
	wsClients       map[*websocket.Conn]bool
	wsMutex         sync.RWMutex
	wsUpgrader      websocket.Upgrader
	templates       *template.Template
	webDir          string
	mqttLog         []MQTTLogEntry
	mqttLogMutex    sync.RWMutex
	automationJobs  map[string]*AutomationJob
	automationMutex sync.RWMutex
	healthCheckers  map[string]*time.Ticker
	healthMutex     sync.RWMutex
}