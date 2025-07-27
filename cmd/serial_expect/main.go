package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"go.bug.st/serial"
)

// XML configuration structures
type Config struct {
	XMLName xml.Name      `xml:"config"`
	Serial  Serial        `xml:"serial"`
	Timeout Timeout       `xml:"timeout"`
	Scripts []NamedScript `xml:"script"`
}

type Serial struct {
	Device string `xml:"device,attr"`
	Speed  int    `xml:"speed,attr"`
	Parity bool   `xml:"parity,attr"`
	Bits   int    `xml:"bits,attr"`
}

type Timeout struct {
	Script  string `xml:"script,attr"`
	Receive string `xml:"receive,attr"`
}

type NamedScript struct {
	Name    string `xml:"name,attr"`
	Content string `xml:",chardata"`
}

type Command struct {
	Type  string // "send" or "expect"
	Value string
}

// Expect matching types
const (
	MatchCaseInsensitive = iota // single quotes 'text'
	MatchCaseSensitive          // double quotes "text"
	MatchRegex                  // forward slashes /regex/
)

type ExpectPattern struct {
	Pattern   string
	MatchType int
	Regex     *regexp.Regexp
}

type SerialExpect struct {
	port           serial.Port
	buffer         strings.Builder
	logger         *log.Logger
	commands       []Command
	scriptTimeout  time.Duration
	receiveTimeout time.Duration
}

func main() {
	var configFile = flag.String("config", "", "XML configuration file")
	var noTimestamp = flag.Bool("no-timestamp", false, "Disable timestamp in log output")
	var dryRun = flag.String("dry-run", "", "Dry run mode: specify text file with captured serial input")
	flag.Parse()

	if *configFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -config <xml-file> [-no-timestamp] [-dry-run <input-file>] [script1] [script2] ...\n", os.Args[0])
		os.Exit(1)
	}

	// Get script names from command line arguments
	scriptNames := flag.Args()

	// Setup logger
	logger := log.New(os.Stdout, "", log.LstdFlags)
	if *noTimestamp {
		logger.SetFlags(0)
	}

	// Read and parse XML configuration
	config, err := parseConfig(*configFile)
	if err != nil {
		logger.Fatalf("Failed to parse config: %v", err)
	}

	// Parse timeout values
	scriptTimeout, receiveTimeout, err := parseTimeouts(config.Timeout)
	if err != nil {
		logger.Fatalf("Failed to parse timeouts: %v", err)
	}

	// Create SerialExpect instance
	se := &SerialExpect{
		logger:         logger,
		scriptTimeout:  scriptTimeout,
		receiveTimeout: receiveTimeout,
	}

	// Determine which scripts to execute
	scriptsToExecute, err := selectScripts(config, scriptNames)
	if err != nil {
		logger.Fatalf("Failed to select scripts: %v", err)
	}

	// Parse and combine all selected scripts
	var allCommands []Command
	for i, scriptContent := range scriptsToExecute {
		logger.Printf("Parsing script %d: %s", i+1, scriptContent.Name)
		commands, err := parseScript(scriptContent.Content)
		if err != nil {
			logger.Fatalf("Failed to parse script %q: %v", scriptContent.Name, err)
		}
		allCommands = append(allCommands, commands...)
	}
	
	se.commands = allCommands

	logger.Printf("Script timeout: %v, Receive timeout: %v", scriptTimeout, receiveTimeout)
	logger.Printf("Executing %d scripts with %d total commands", len(scriptsToExecute), len(allCommands))

	if *dryRun != "" {
		// Dry run mode
		logger.Printf("Running in dry-run mode with input file: %s", *dryRun)
		if err := se.executeDryRun(*dryRun); err != nil {
			logger.Fatalf("Dry run failed: %v", err)
		}
	} else {
		// Normal mode - open serial port
		if err := se.openSerial(config.Serial); err != nil {
			logger.Fatalf("Failed to open serial port: %v", err)
		}
		defer se.port.Close()

		logger.Printf("Connected to %s at %d baud", config.Serial.Device, config.Serial.Speed)

		// Execute script with timeout
		if err := se.executeScriptWithTimeout(); err != nil {
			logger.Fatalf("Script execution failed: %v", err)
		}
	}

	logger.Println("Script completed successfully")
}

func selectScripts(config *Config, scriptNames []string) ([]NamedScript, error) {
	if len(config.Scripts) == 0 {
		return nil, fmt.Errorf("no scripts found in configuration")
	}

	// Handle case where scripts might not have names (backward compatibility)
	var availableScripts []NamedScript
	for i, script := range config.Scripts {
		if script.Name == "" {
			// Unnamed script - give it a default name for backward compatibility
			script.Name = fmt.Sprintf("script%d", i+1)
		}
		availableScripts = append(availableScripts, script)
	}

	// If no script names specified on command line
	if len(scriptNames) == 0 {
		// Run only the first script
		return []NamedScript{availableScripts[0]}, nil
	}

	// Find and validate requested scripts
	var selectedScripts []NamedScript
	for _, requestedName := range scriptNames {
		found := false
		for _, availableScript := range availableScripts {
			if availableScript.Name == requestedName {
				selectedScripts = append(selectedScripts, availableScript)
				found = true
				break
			}
		}
		if !found {
			// List available scripts for error message
			var availableNames []string
			for _, script := range availableScripts {
				availableNames = append(availableNames, script.Name)
			}
			return nil, fmt.Errorf("script %q not found. Available scripts: %s", 
				requestedName, strings.Join(availableNames, ", "))
		}
	}

	return selectedScripts, nil
}

func parseConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := xml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func parseTimeouts(timeout Timeout) (time.Duration, time.Duration, error) {
	// Default values
	scriptTimeout := 60 * time.Second  // 1 minute default
	receiveTimeout := 30 * time.Second // 30 seconds default

	var err error

	// Parse script timeout if provided
	if timeout.Script != "" {
		scriptTimeout, err = time.ParseDuration(timeout.Script)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid script timeout format %q: %v", timeout.Script, err)
		}
	}

	// Parse receive timeout if provided
	if timeout.Receive != "" {
		receiveTimeout, err = time.ParseDuration(timeout.Receive)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid receive timeout format %q: %v", timeout.Receive, err)
		}
	}

	return scriptTimeout, receiveTimeout, nil
}

func (se *SerialExpect) executeScriptWithTimeout() error {
	// Create context with script timeout
	ctx, cancel := context.WithTimeout(context.Background(), se.scriptTimeout)
	defer cancel()

	// Start reading from serial port in goroutine
	readChan := make(chan string, 100)
	go se.readSerial(readChan)

	// Execute script with context
	done := make(chan error, 1)
	go func() {
		done <- se.executeScript(readChan)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("script timeout exceeded (%v)", se.scriptTimeout)
	}
}

func (se *SerialExpect) executeScript(readChan <-chan string) error {
	for i, cmd := range se.commands {
		se.logger.Printf("Executing command %d/%d: %s %s", i+1, len(se.commands), cmd.Type, cmd.Value)
		
		switch cmd.Type {
		case "send":
			if err := se.handleSend(cmd.Value); err != nil {
				return err
			}
		case "expect":
			if err := se.handleExpect(cmd.Value, readChan); err != nil {
				return err
			}
		}
	}

	return nil
}

func (se *SerialExpect) executeDryRun(inputFile string) error {
	// Read the input file
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read input file: %v", err)
	}

	input := string(data)
	lines := strings.Split(input, "\n")
	
	se.logger.Println("=== DRY RUN MODE ===")
	
	lineIndex := 0
	
	for i, cmd := range se.commands {
		se.logger.Printf("Command %d/%d: %s %s", i+1, len(se.commands), cmd.Type, cmd.Value)
		
		switch cmd.Type {
		case "send":
			// Handle send command - show what would be sent in bold
			toSend := se.formatSendValue(cmd.Value)
			fmt.Printf("\033[1mTX: %q\033[0m\n", toSend)
			
		case "expect":
			// Handle expect command - find matching line
			expectPattern, err := parseExpectPattern(cmd.Value)
			if err != nil {
				return fmt.Errorf("invalid expect pattern %q: %v", cmd.Value, err)
			}
			
			se.logger.Printf("EXPECT: %s", cmd.Value)
			
			found := false
			startIndex := lineIndex
			
			// Search through remaining lines for a match
			for lineIndex < len(lines) {
				line := lines[lineIndex]
				se.logger.Printf("RX: %s", line)
				
				// Check if this line matches our expect pattern
				if se.checkDryRunMatch(expectPattern, line, lineIndex-startIndex) {
					se.logger.Printf("MATCHED: %s", cmd.Value)
					found = true
					lineIndex++ // Move to next line for next command
					break
				}
				lineIndex++
			}
			
			if !found {
				return fmt.Errorf("pattern not found in remaining input: %s", cmd.Value)
			}
		}
	}
	
	se.logger.Println("=== DRY RUN COMPLETED ===")
	return nil
}

func (se *SerialExpect) formatSendValue(value string) string {
	// Parse send syntax same as handleSend but just return the formatted string
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		content := value[1 : len(value)-1]
		
		switch {
		case first == '\'' && last == '\'':
			// Single quotes: send with carriage return
			return content + "\r"
		case first == '"' && last == '"':
			// Double quotes: send as-is, but handle escape sequences
			toSend := strings.ReplaceAll(content, "\\r", "\r")
			toSend = strings.ReplaceAll(toSend, "\\n", "\n")
			return toSend
		default:
			// No quotes, treat as literal
			return value
		}
	}
	return value
}

func (se *SerialExpect) checkDryRunMatch(ep *ExpectPattern, line string, linesSinceStart int) bool {
	switch ep.MatchType {
	case MatchCaseInsensitive:
		// Case-insensitive match anywhere in line
		return strings.Contains(strings.ToLower(line), strings.ToLower(ep.Pattern))
		
	case MatchCaseSensitive:
		// Case-sensitive match at start of line
		return strings.HasPrefix(strings.TrimSpace(line), ep.Pattern)
		
	case MatchRegex:
		// Regex match on line
		return ep.Regex.MatchString(line)
	}
	
	return false
}

func parseScript(scriptText string) ([]Command, error) {
	var commands []Command
	lines := strings.Split(strings.TrimSpace(scriptText), "\n")
	
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		if strings.HasPrefix(line, "send ") {
			value := strings.TrimPrefix(line, "send ")
			commands = append(commands, Command{Type: "send", Value: value})
		} else if strings.HasPrefix(line, "expect ") {
			value := strings.TrimPrefix(line, "expect ")
			commands = append(commands, Command{Type: "expect", Value: value})
		} else {
			return nil, fmt.Errorf("invalid command on line %d: %s", i+1, line)
		}
	}
	
	return commands, nil
}

func (se *SerialExpect) openSerial(config Serial) error {
	mode := &serial.Mode{
		BaudRate: config.Speed,
		DataBits: config.Bits,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	if config.Parity {
		mode.Parity = serial.EvenParity
	}

	port, err := serial.Open(config.Device, mode)
	if err != nil {
		return err
	}

	se.port = port
	return nil
}

func (se *SerialExpect) readSerial(readChan chan<- string) {
	reader := bufio.NewReader(se.port)
	for {
		char, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				se.logger.Println("Serial port closed")
				return
			}
			se.logger.Printf("Read error: %v", err)
			continue
		}

		se.buffer.WriteByte(char)
		
		// Send character to channel for real-time processing
		readChan <- string(char)
		
		// Log readable characters (skip control chars except newline)
		if char >= 32 || char == '\n' || char == '\r' {
			if char == '\n' {
				se.logger.Printf("RX: %s", strings.TrimRight(se.buffer.String(), "\r\n"))
				se.buffer.Reset()
			}
		}
	}
}

func (se *SerialExpect) handleSend(value string) error {
	var toSend string
	
	// Parse send syntax
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		content := value[1 : len(value)-1]
		
		switch {
		case first == '\'' && last == '\'':
			// Single quotes: send with carriage return
			toSend = content + "\r"
		case first == '"' && last == '"':
			// Double quotes: send as-is, but handle escape sequences
			toSend = strings.ReplaceAll(content, "\\r", "\r")
			toSend = strings.ReplaceAll(toSend, "\\n", "\n")
		default:
			// No quotes, treat as literal
			toSend = value
		}
	} else {
		toSend = value
	}

	se.logger.Printf("TX: %q", toSend)
	
	_, err := se.port.Write([]byte(toSend))
	if err != nil {
		return fmt.Errorf("failed to send data: %v", err)
	}
	
	return nil
}

func (se *SerialExpect) handleExpect(pattern string, readChan <-chan string) error {
	expectPattern, err := parseExpectPattern(pattern)
	if err != nil {
		return fmt.Errorf("invalid expect pattern %q: %v", pattern, err)
	}

	se.logger.Printf("EXPECT: %s", pattern)
	
	var buffer strings.Builder
	var currentLine strings.Builder
	
	timeout := time.After(se.receiveTimeout)
	
	for {
		select {
		case char := <-readChan:
			buffer.WriteString(char)
			currentLine.WriteString(char)
			
			// Reset current line on newline
			if char == "\n" {
				currentLine.Reset()
			}
			
			// Check for match
			if se.checkMatch(expectPattern, buffer.String(), currentLine.String()) {
				se.logger.Printf("MATCHED: %s", pattern)
				return nil
			}
			
		case <-timeout:
			return fmt.Errorf("receive timeout (%v) waiting for pattern: %s", se.receiveTimeout, pattern)
		}
	}
}

func parseExpectPattern(pattern string) (*ExpectPattern, error) {
	if len(pattern) < 2 {
		return nil, fmt.Errorf("pattern too short")
	}

	first := pattern[0]
	last := pattern[len(pattern)-1]
	content := pattern[1 : len(pattern)-1]

	ep := &ExpectPattern{}

	switch {
	case first == '\'' && last == '\'':
		// Single quotes: case-insensitive anywhere in stream
		ep.Pattern = content
		ep.MatchType = MatchCaseInsensitive
	case first == '"' && last == '"':
		// Double quotes: case-sensitive line start
		ep.Pattern = content
		ep.MatchType = MatchCaseSensitive
	case first == '/' && last == '/':
		// Forward slashes: regex
		regex, err := regexp.Compile(content)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %v", err)
		}
		ep.Pattern = content
		ep.MatchType = MatchRegex
		ep.Regex = regex
	default:
		return nil, fmt.Errorf("invalid pattern format")
	}

	return ep, nil
}

func (se *SerialExpect) checkMatch(ep *ExpectPattern, buffer, currentLine string) bool {
	switch ep.MatchType {
	case MatchCaseInsensitive:
		// Case-insensitive match anywhere in buffer
		return strings.Contains(strings.ToLower(buffer), strings.ToLower(ep.Pattern))
		
	case MatchCaseSensitive:
		// Case-sensitive match at start of current line
		return strings.HasPrefix(strings.TrimSpace(currentLine), ep.Pattern)
		
	case MatchRegex:
		// Regex match on current line
		return ep.Regex.MatchString(currentLine)
	}
	
	return false
}