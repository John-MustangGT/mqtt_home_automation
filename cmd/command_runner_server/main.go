package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// XML Configuration structures
type Config struct {
	XMLName   xml.Name `xml:"config"`
	Server    Server   `xml:"server"`
	Buttons   []Button `xml:"buttons>button"`
}

type Server struct {
	Interface   string `xml:"interface"`
	Port        string `xml:"port"`
	WebDir      string `xml:"webdir"`
	UIFramework string `xml:"ui_framework,omitempty"` // bootstrap or ionic
}

type Button struct {
	Name        string `xml:"name"`
	DisplayName string `xml:"display_name"`
	Command     string `xml:"command"`
	Size        string `xml:"size,omitempty"`    // sm, md, lg
	Color       string `xml:"color,omitempty"`   // primary, secondary, success, danger, warning, info
}

// Global variables
var config Config
var commandOutputs = make(map[string]string)
var templates *template.Template
var configMutex sync.RWMutex
var configFile string
var watchedFiles = make(map[string]time.Time)
var serverStartTime time.Time
var lastReloadTime time.Time
var debugMode bool // Debug flag

// Debug logging function
func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// HTML template
// Templates will be loaded from files

func loadConfig(filename string) error {
	configMutex.Lock()
	defer configMutex.Unlock()
	
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	
	var newConfig Config
	err = xml.Unmarshal(data, &newConfig)
	if err != nil {
		return err
	}
	
	// Set default UI framework if not specified
	if newConfig.Server.UIFramework == "" {
		newConfig.Server.UIFramework = "bootstrap"
	}
	
	// Load templates from webdir
	templatePath := newConfig.Server.WebDir + "/*.html"
	newTemplates, err := template.ParseGlob(templatePath)
	if err != nil {
		return fmt.Errorf("error loading templates from %s: %v", templatePath, err)
	}
	
	// Update global variables
	config = newConfig
	templates = newTemplates
	lastReloadTime = time.Now()
	
	log.Printf("Configuration reloaded from %s", filename)
	log.Printf("Using UI framework: %s", config.Server.UIFramework)
	return nil
}

func runCommandHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		debugLog("Non-POST request to /run, redirecting")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	command := r.FormValue("command")
	name := r.FormValue("name")
	
	debugLog("Received command: %s (name: %s)", command, name)
	
	if command == "" {
		debugLog("Empty command, redirecting")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	
	// Execute command synchronously so output is available immediately
	debugLog("Executing command: %s", command)
	executeCommand(name, command)
	debugLog("Command execution completed, current output length: %d", len(commandOutputs["latest"]))
	
	// Simple redirect back to home page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func outputHandler(w http.ResponseWriter, r *http.Request) {
	output := getLatestOutput()
	debugLog("Output handler called, returning %d characters", len(output))
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(output))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()

	// Check for framework preference in cookie
	currentFramework := config.Server.UIFramework
	if cookie, err := r.Cookie("ui_framework"); err == nil {
		if cookie.Value == "bootstrap" || cookie.Value == "ionic" {
			currentFramework = cookie.Value
		}
	}

	data := struct {
		Buttons        []Button
		Output         string
		CurrentTime    string
		ServerUptime   string
		SystemUptime   string
		SystemLoad     string
		MemoryInfo     string
		LastReload     string
		ConfigFile     string
		ButtonCount    int
		GoVersion      string
		UIFramework    string
	}{
		Buttons:        config.Buttons,
		Output:         getLatestOutput(),
		CurrentTime:    time.Now().Format("2006-01-02 15:04:05 MST"),
		ServerUptime:   formatDuration(time.Since(serverStartTime)),
		SystemUptime:   getSystemUptime(),
		SystemLoad:     getSystemLoad(),
		MemoryInfo:     getMemoryInfo(),
		LastReload:     getLastReloadTime(),
		ConfigFile:     configFile,
		ButtonCount:    len(config.Buttons),
		GoVersion:      runtime.Version(),
		UIFramework:    currentFramework,
	}

	// Choose template based on current framework
	templateName := "index.html"
	if currentFramework == "ionic" {
		templateName = "ionic.html"
	}

	err := templates.ExecuteTemplate(w, templateName, data)
	if err != nil {
		http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
	}
}

func setFrameworkHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	framework := r.FormValue("framework")
	if framework != "bootstrap" && framework != "ionic" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Set cookie for 30 days
	cookie := &http.Cookie{
		Name:     "ui_framework",
		Value:    framework,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60, // 30 days
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	debugLog("UI Framework changed to: %s (saved in cookie)", framework)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func executeCommand(name, command string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	output := fmt.Sprintf("[%s] Executing: %s\n", timestamp, name)
	
	debugLog("Starting execution of command: %s", command)
	
	// Split command into parts
	parts := strings.Fields(command)
	if len(parts) == 0 {
		errorMsg := "Error: Empty command\n\n"
		appendOutput(output + errorMsg)
		debugLog("Empty command parts")
		return
	}
	
	// Execute command
	cmd := exec.Command(parts[0], parts[1:]...)
	result, err := cmd.CombinedOutput()
	
	if err != nil {
		output += fmt.Sprintf("Error: %v\n", err)
		debugLog("Command execution error: %v", err)
	} else {
		debugLog("Command executed successfully, output length: %d", len(result))
	}
	
	output += string(result) + "\n" + strings.Repeat("-", 50) + "\n\n"
	appendOutput(output)
	
	debugLog("Command output appended, total output length: %d", len(commandOutputs["latest"]))
}

func appendOutput(text string) {
	// Keep only last 10KB of output to prevent memory issues
	const maxOutputSize = 10240
	
	commandOutputs["latest"] += text
	
	if len(commandOutputs["latest"]) > maxOutputSize {
		commandOutputs["latest"] = commandOutputs["latest"][len(commandOutputs["latest"])-maxOutputSize:]
	}
	
	debugLog("Output appended, current total length: %d", len(commandOutputs["latest"]))
}

func getLatestOutput() string {
	if output, exists := commandOutputs["latest"]; exists {
		debugLog("Returning output of length: %d", len(output))
		return output
	}
	debugLog("No output exists, returning default message")
	return "No commands executed yet."
}

// System information functions
func getSystemUptime() string {
	if runtime.GOOS == "linux" {
		if data, err := ioutil.ReadFile("/proc/uptime"); err == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				if seconds, err := strconv.ParseFloat(parts[0], 64); err == nil {
					duration := time.Duration(seconds) * time.Second
					return formatDuration(duration)
				}
			}
		}
	}
	// Fallback for non-Linux systems
	cmd := exec.Command("uptime")
	if output, err := cmd.Output(); err == nil {
		return strings.TrimSpace(string(output))
	}
	return "Unable to determine system uptime"
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func getSystemLoad() string {
	if runtime.GOOS == "linux" {
		if data, err := ioutil.ReadFile("/proc/loadavg"); err == nil {
			parts := strings.Fields(string(data))
			if len(parts) >= 3 {
				return fmt.Sprintf("%s %s %s (1m 5m 15m)", parts[0], parts[1], parts[2])
			}
		}
	}
	// Fallback for non-Linux systems
	cmd := exec.Command("uptime")
	if output, err := cmd.Output(); err == nil {
		uptime := string(output)
		if idx := strings.Index(uptime, "load average:"); idx != -1 {
			return strings.TrimSpace(uptime[idx+13:])
		}
	}
	return "Unable to determine system load"
}

func getMemoryInfo() string {
	if runtime.GOOS == "linux" {
		if data, err := ioutil.ReadFile("/proc/meminfo"); err == nil {
			lines := strings.Split(string(data), "\n")
			//memTotal, memFree, memAvailable := "", "", ""
			memTotal, _, memAvailable := "", "", ""
			
			for _, line := range lines {
				if strings.HasPrefix(line, "MemTotal:") {
					memTotal = strings.Fields(line)[1]
				//} else if strings.HasPrefix(line, "MemFree:") {
				//	memFree = strings.Fields(line)[1]
				} else if strings.HasPrefix(line, "MemAvailable:") {
					memAvailable = strings.Fields(line)[1]
				}
			}
			
			if memTotal != "" && memAvailable != "" {
				if total, err1 := strconv.Atoi(memTotal); err1 == nil {
					if available, err2 := strconv.Atoi(memAvailable); err2 == nil {
						used := total - available
						usedPercent := float64(used) / float64(total) * 100
						return fmt.Sprintf("%.1f%% used (%d MB / %d MB)", 
							usedPercent, used/1024, total/1024)
					}
				}
			}
		}
	}
	return "Unable to determine memory usage"
}

func getLastReloadTime() string {
	if lastReloadTime.IsZero() {
		return "Never"
	}
	return lastReloadTime.Format("2006-01-02 15:04:05 MST")
}

// New handlers
func xmlConfigHandler(w http.ResponseWriter, r *http.Request) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		http.Error(w, "Error reading config file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=\"config.xml\"")
	w.Write(data)
}

func apiTimeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(time.Now().Format("2006-01-02 15:04:05 MST")))
}

func apiStatsHandler(w http.ResponseWriter, r *http.Request) {
	configMutex.RLock()
	defer configMutex.RUnlock()
	
	stats := map[string]interface{}{
		"server_uptime":   formatDuration(time.Since(serverStartTime)),
		"system_uptime":   getSystemUptime(),
		"system_load":     getSystemLoad(),
		"memory_info":     getMemoryInfo(),
		"last_reload":     getLastReloadTime(),
		"button_count":    len(config.Buttons),
		"go_version":      runtime.Version(),
		"current_time":    time.Now().Format("2006-01-02 15:04:05 MST"),
		"ui_framework":    config.Server.UIFramework,
	}
	
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
		"server_uptime":"%s",
		"system_uptime":"%s",
		"system_load":"%s",
		"memory_info":"%s",
		"last_reload":"%s",
		"button_count":%d,
		"go_version":"%s",
		"current_time":"%s",
		"ui_framework":"%s"
	}`, stats["server_uptime"], stats["system_uptime"], stats["system_load"], 
		stats["memory_info"], stats["last_reload"], stats["button_count"], 
		stats["go_version"], stats["current_time"], stats["ui_framework"])
}

// File monitoring functions
func getFileModTime(filename string) (time.Time, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func getWatchedFiles() []string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	
	files := []string{configFile}
	
	// Add HTML template files
	templatePattern := config.Server.WebDir + "/*.html"
	matches, err := filepath.Glob(templatePattern)
	if err == nil {
		files = append(files, matches...)
	}
	
	// Add CSS and JS files
	cssPattern := config.Server.WebDir + "/static/css/*.css"
	cssMatches, err := filepath.Glob(cssPattern)
	if err == nil {
		files = append(files, cssMatches...)
	}
	
	jsPattern := config.Server.WebDir + "/static/js/*.js"
	jsMatches, err := filepath.Glob(jsPattern)
	if err == nil {
		files = append(files, jsMatches...)
	}
	
	return files
}

func initFileWatcher() {
	files := getWatchedFiles()
	for _, file := range files {
		if modTime, err := getFileModTime(file); err == nil {
			watchedFiles[file] = modTime
		}
	}
}

func checkForChanges() bool {
	files := getWatchedFiles()
	changed := false
	
	for _, file := range files {
		currentModTime, err := getFileModTime(file)
		if err != nil {
			continue
		}
		
		if lastModTime, exists := watchedFiles[file]; !exists || currentModTime.After(lastModTime) {
			debugLog("File changed: %s", file)
			watchedFiles[file] = currentModTime
			changed = true
		}
	}
	
	return changed
}

func startFileWatcher() {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		for range ticker.C {
			if checkForChanges() {
				debugLog("Changes detected, reloading configuration...")
				if err := loadConfig(configFile); err != nil {
					log.Printf("Error reloading config: %v", err)
				} else {
					debugLog("Configuration successfully reloaded")
				}
			}
		}
	}()
}

func main() {
	// Parse command line arguments
	configFilePtr := flag.String("config", "config.xml", "Path to the XML configuration file")
	debugPtr := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()
	
	configFile = *configFilePtr
	debugMode = *debugPtr
	serverStartTime = time.Now()
	
	if debugMode {
		log.Println("Debug mode enabled")
	}
	
	// Load initial configuration
	if err := loadConfig(configFile); err != nil {
		log.Fatal("Error loading config file:", err)
	}
	
	// Set initial reload time
	lastReloadTime = serverStartTime
	
	// Initialize file watcher
	initFileWatcher()
	startFileWatcher()
	
	// Set up static file serving for CSS, JS, images, etc.
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(config.Server.WebDir+"/static/"))))
	
	// Set up routes
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/run", runCommandHandler)
	http.HandleFunc("/output", outputHandler)
	http.HandleFunc("/config.xml", xmlConfigHandler)
	http.HandleFunc("/api/time", apiTimeHandler)
	http.HandleFunc("/api/stats", apiStatsHandler)
	http.HandleFunc("/set-framework", setFrameworkHandler)
	
	// Start server
	address := config.Server.Interface + ":" + config.Server.Port
	fmt.Printf("Server starting on %s\n", address)
	fmt.Printf("Using config file: %s\n", configFile)
	fmt.Printf("Using web directory: %s\n", config.Server.WebDir)
	fmt.Printf("UI Framework: %s\n", config.Server.UIFramework)
	fmt.Printf("File watching enabled - server will auto-reload on changes\n")
	if debugMode {
		fmt.Printf("Debug mode: ENABLED\n")
	}
	
	log.Fatal(http.ListenAndServe(address, nil))
}
