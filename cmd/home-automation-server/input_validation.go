package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"sync"
)

// Input validation functions
func validateMQTTTopic(topic string) error {
	if topic == "" {
		return fmt.Errorf("topic cannot be empty")
	}
	
	// MQTT topic validation rules
	if len(topic) > 65535 {
		return fmt.Errorf("topic too long (max 65535 characters)")
	}
	
	// Check for invalid characters
	if strings.Contains(topic, "\u0000") {
		return fmt.Errorf("topic contains null character")
	}
	
	// Wildcard characters should not be in publish topics
	if strings.Contains(topic, "+") || strings.Contains(topic, "#") {
		return fmt.Errorf("wildcards not allowed in publish topics")
	}
	
	return nil
}

func validateMQTTPayload(payload string) error {
	// MQTT payload can be any bytes, but we'll add some reasonable limits
	if len(payload) > 268435455 { // 256MB limit
		return fmt.Errorf("payload too large (max 256MB)")
	}
	
	return nil
}

func validateDeviceID(deviceID string) error {
	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}
	
	// Allow alphanumeric, hyphens, and underscores
	matched, _ := regexp.MatchString("^[a-zA-Z0-9_-]+$", deviceID)
	if !matched {
		return fmt.Errorf("device ID contains invalid characters (allowed: a-z, A-Z, 0-9, -, _)")
	}
	
	if len(deviceID) > 100 {
		return fmt.Errorf("device ID too long (max 100 characters)")
	}
	
	return nil
}

func validateLocalCommand(command string) error {
	if command == "" {
		return nil // Empty command is OK
	}
	
	// Basic command injection prevention
	dangerousPatterns := []string{
		";", "&", "&&", "|", "||", "`", "$(",
		"$(", ")", "{", "}", "[", "]", "<", ">",
		"../", "./", "~", "*",
	}
	
	commandLower := strings.ToLower(command)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(commandLower, pattern) {
			return fmt.Errorf("command contains potentially dangerous pattern: %s", pattern)
		}
	}
	
	// Whitelist approach - only allow specific commands
	allowedCommands := []string{
		"gpio-control", "systemctl", "service", "ping", "echo",
		"cat", "ls", "pwd", "date", "uptime", "free", "df",
	}
	
	parts := strings.Fields(command)
	if len(parts) > 0 {
		baseCommand := parts[0]
		allowed := false
		for _, allowedCmd := range allowedCommands {
			if baseCommand == allowedCmd || strings.HasSuffix(baseCommand, "/"+allowedCmd) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("command not in allowed list: %s", baseCommand)
		}
	}
	
	if len(command) > 500 {
		return fmt.Errorf("command too long (max 500 characters)")
	}
	
	return nil
}

func validateControlValue(control Control, value interface{}) error {
	switch control.Type {
	case "slider":
		var numValue float64
		var err error
		
		switch v := value.(type) {
		case string:
			numValue, err = strconv.ParseFloat(v, 64)
			if err != nil {
				return fmt.Errorf("slider value must be a number")
			}
		case float64:
			numValue = v
		case int:
			numValue = float64(v)
		default:
			return fmt.Errorf("slider value must be a number")
		}
		
		// Check against control's min/max
		if numValue < float64(control.Min) || numValue > float64(control.Max) {
			return fmt.Errorf("slider value %v out of range [%d, %d]", numValue, control.Min, control.Max)
		}
		
		// Check against control's custom validation if specified
		if control.MinValue != nil && numValue < *control.MinValue {
			return fmt.Errorf("value %v below minimum %v", numValue, *control.MinValue)
		}
		if control.MaxValue != nil && numValue > *control.MaxValue {
			return fmt.Errorf("value %v above maximum %v", numValue, *control.MaxValue)
		}
		
	case "toggle", "button":
		// For toggle/button, validate payload if it's dynamic
		if strValue, ok := value.(string); ok {
			if len(control.AllowedValues) > 0 {
				allowed := false
				for _, allowedValue := range control.AllowedValues {
					if strValue == allowedValue {
						allowed = true
						break
					}
				}
				if !allowed {
					return fmt.Errorf("value %s not in allowed values: %v", strValue, control.AllowedValues)
				}
			}
		}
	}
	
	return nil
}

func sanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\u0000", "")
	
	// Trim whitespace
	input = strings.TrimSpace(input)
	
	// Replace multiple consecutive spaces with single space
	re := regexp.MustCompile(`\s+`)
	input = re.ReplaceAllString(input, " ")
	
	return input
}

func validateAutomationSchedule(schedule Schedule) error {
	switch schedule.Type {
	case "time":
		if schedule.Time == "" {
			return fmt.Errorf("time is required for time-based automation")
		}
		
		// Validate time format (HH:MM)
		matched, _ := regexp.MatchString(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`, schedule.Time)
		if !matched {
			return fmt.Errorf("invalid time format, use HH:MM")
		}
		
	case "interval":
		if schedule.Interval == "" {
			return fmt.Errorf("interval is required for interval-based automation")
		}
		
		// Validate duration format
		if _, err := time.ParseDuration(schedule.Interval); err != nil {
			return fmt.Errorf("invalid interval format: %v", err)
		}
		
	case "duration":
		if schedule.Interval == "" || schedule.Duration == "" {
			return fmt.Errorf("both interval and duration are required for duration-based automation")
		}
		
		// Validate both duration formats
		if _, err := time.ParseDuration(schedule.Interval); err != nil {
			return fmt.Errorf("invalid interval format: %v", err)
		}
		if _, err := time.ParseDuration(schedule.Duration); err != nil {
			return fmt.Errorf("invalid duration format: %v", err)
		}
		
	default:
		return fmt.Errorf("invalid schedule type: %s", schedule.Type)
	}
	
	// Validate date formats if specified
	if schedule.StartDate != "" {
		if _, err := time.Parse("2006-01-02", schedule.StartDate); err != nil {
			return fmt.Errorf("invalid start date format, use YYYY-MM-DD")
		}
	}
	
	if schedule.EndDate != "" {
		if _, err := time.Parse("2006-01-02", schedule.EndDate); err != nil {
			return fmt.Errorf("invalid end date format, use YYYY-MM-DD")
		}
	}
	
	// Validate days format if specified
	if schedule.Days != "" {
		validDays := map[string]bool{
			"mon": true, "tue": true, "wed": true, "thu": true,
			"fri": true, "sat": true, "sun": true,
		}
		
		for _, day := range strings.Split(strings.ToLower(schedule.Days), ",") {
			day = strings.TrimSpace(day)
			if !validDays[day] {
				return fmt.Errorf("invalid day: %s (use mon,tue,wed,thu,fri,sat,sun)", day)
			}
		}
	}
	
	return nil
}

// Rate limiting for API endpoints
type RateLimiter struct {
	requests map[string][]time.Time
	mutex    sync.RWMutex
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
	}
}

func (rl *RateLimiter) Allow(clientIP string, maxRequests int, timeWindow time.Duration) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()
	
	now := time.Now()
	cutoff := now.Add(-timeWindow)
	
	// Get existing requests for this client
	clientRequests := rl.requests[clientIP]
	
	// Remove old requests
	var validRequests []time.Time
	for _, reqTime := range clientRequests {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	
	// Check if client has exceeded rate limit
	if len(validRequests) >= maxRequests {
		rl.requests[clientIP] = validRequests
		return false
	}
	
	// Add current request
	validRequests = append(validRequests, now)
	rl.requests[clientIP] = validRequests
	
	return true
}

// Global rate limiter instance
var globalRateLimiter = NewRateLimiter()
