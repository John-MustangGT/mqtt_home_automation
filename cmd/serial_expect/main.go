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
	"strconv"
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
	Tries   []TryBlock    `xml:"try"`
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

type TryBlock struct {
	Name   string `xml:"name,attr"`
	Script string `xml:"script,attr"`
	Except string `xml:"except,attr"`
	Retry  bool   `xml:"retry,attr"`
}

type Command struct {
	Type       string // "send", "expect", "monitor", or "try"
	Value      string
	TryBlock   *TryBlock // Only used for try commands
	ScriptMap  map[string]NamedScript // Only used for try commands
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
	config         *Config
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
		config:         config,
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
		
		// Check if this is a try block
		if strings.HasPrefix(scriptContent.Content, "__TRY_BLOCK__") {
			tryBlockName := strings.TrimPrefix(scriptContent.Content, "__TRY_BLOCK__")
			commands, err := se.parseTryBlock(config, tryBlockName)
			if err != nil {
				logger.Fatalf("Failed to parse try block %q: %v", tryBlockName, err)
			}
			allCommands = append(allCommands, commands...)
		} else {
			commands, err := parseScript(scriptContent.Content)
			if err != nil {
				logger.Fatalf("Failed to parse script %q: %v", scriptContent.Name, err)
			}
			allCommands = append(allCommands, commands...)
		}
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
	// Build a map of all available scripts
	scriptMap := make(map[string]NamedScript)
	
	// Add regular scripts
	for i, script := range config.Scripts {
		name := script.Name
		if name == "" {
			// Unnamed script - give it a default name for backward compatibility
			name = fmt.Sprintf("script%d", i+1)
			script.Name = name
		}
		scriptMap[name] = script
	}
	
	// Add try blocks as executable units
	for _, tryBlock := range config.Tries {
		if tryBlock.Name == "" {
			return nil, fmt.Errorf("try block must have a name attribute")
		}
		// Validate that referenced scripts exist
		if _, exists := scriptMap[tryBlock.Script]; !exists {
			return nil, fmt.Errorf("try block %q references non-existent script %q", tryBlock.Name, tryBlock.Script)
		}
		if tryBlock.Except != "" {
			if _, exists := scriptMap[tryBlock.Except]; !exists {
				return nil, fmt.Errorf("try block %q references non-existent except script %q", tryBlock.Name, tryBlock.Except)
			}
		}
		// Add try block as a virtual script
		scriptMap[tryBlock.Name] = NamedScript{
			Name:    tryBlock.Name,
			Content: fmt.Sprintf("__TRY_BLOCK__%s", tryBlock.Name), // Special marker
		}
	}

	if len(scriptMap) == 0 {
		return nil, fmt.Errorf("no scripts or try blocks found in configuration")
	}

	// If no script names specified on command line
	if len(scriptNames) == 0 {
		// Run only the first available script/try block
		for _, script := range config.Scripts {
			name := script.Name
			if name == "" {
				name = "script1"
				script.Name = name
			}
			return []NamedScript{script}, nil
		}
		for _, tryBlock := range config.Tries {
			return []NamedScript{{Name: tryBlock.Name, Content: fmt.Sprintf("__TRY_BLOCK__%s", tryBlock.Name)}}, nil
		}
	}

	// Find and validate requested scripts
	var selectedScripts []NamedScript
	for _, requestedName := range scriptNames {
		if script, exists := scriptMap[requestedName]; exists {
			selectedScripts = append(selectedScripts, script)
		} else {
			// List available scripts for error message
			var availableNames []string
			for name := range scriptMap {
				availableNames = append(availableNames, name)
			}
			return nil, fmt.Errorf("script or try block %q not found. Available: %s", 
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

	// Check if any commands are monitor commands - if so, extend timeout
	hasMonitorCommand := false
	for _, cmd := range se.commands {
		if cmd.Type == "monitor" {
			hasMonitorCommand = true
			break
		}
	}
	
	// If we have monitor commands, use a much longer timeout
	if hasMonitorCommand {
		ctx, cancel = context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()
		se.logger.Printf("Extended timeout for monitor commands")
	}

	// Execute script with context
	done := make(chan error, 1)
	go func() {
		done <- se.executeScript(readChan)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if hasMonitorCommand {
			return fmt.Errorf("script timeout exceeded (extended for monitor commands)")
		}
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
		case "try":
			if err := se.handleTry(cmd, readChan); err != nil {
				return err
			}
		case "monitor":
			if err := se.handleMonitor(cmd.Value, readChan); err != nil {
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
			
		case "try":
			se.logger.Printf("TRY BLOCK: %s (dry run - executing main script only)", cmd.Value)
			
			// In dry run, just execute the main script
			tryBlock := cmd.TryBlock
			scriptMap := cmd.ScriptMap
			
			mainScript, exists := scriptMap[tryBlock.Script]
			if !exists {
				return fmt.Errorf("script %q referenced by try block not found", tryBlock.Script)
			}
			
			commands, err := parseScript(mainScript.Content)
			if err != nil {
				return fmt.Errorf("failed to parse script in try block: %v", err)
			}
			
			// Execute the main script commands in dry run mode
			for _, subCmd := range commands {
				switch subCmd.Type {
				case "send":
					toSend := se.formatSendValue(subCmd.Value)
					fmt.Printf("\033[1mTX: %q\033[0m\n", toSend)
				case "expect":
					expectPattern, err := parseExpectPattern(subCmd.Value)
					if err != nil {
						return fmt.Errorf("invalid expect pattern %q: %v", subCmd.Value, err)
					}
					
					se.logger.Printf("EXPECT: %s", subCmd.Value)
					
					found := false
					startIndex := lineIndex
					
					for lineIndex < len(lines) {
						line := lines[lineIndex]
						se.logger.Printf("RX: %s", line)
						
						if se.checkDryRunMatch(expectPattern, line, lineIndex-startIndex) {
							se.logger.Printf("MATCHED: %s", subCmd.Value)
							found = true
							lineIndex++
							break
						}
						lineIndex++
					}
					
					if !found {
						se.logger.Printf("PATTERN NOT FOUND (would trigger except): %s", subCmd.Value)
						// In dry run, continue without failing
					}
				case "monitor":
					se.logger.Printf("MONITOR: %s (dry run - showing next 10 lines)", subCmd.Value)
					
					// In dry run, just show some lines from the input
					maxShow := 10
					if lineIndex+maxShow > len(lines) {
						maxShow = len(lines) - lineIndex
					}
					
					for i := 0; i < maxShow && lineIndex < len(lines); i++ {
						se.logger.Printf("RX: %s", lines[lineIndex])
						lineIndex++
					}
				}
			}
		
		case "monitor":
			se.logger.Printf("MONITOR: %s (dry run - showing next 10 lines)", cmd.Value)
			
			// In dry run, just show some lines from the input
			maxShow := 10
			if lineIndex+maxShow > len(lines) {
				maxShow = len(lines) - lineIndex
			}
			
			for i := 0; i < maxShow && lineIndex < len(lines); i++ {
				se.logger.Printf("RX: %s", lines[lineIndex])
				lineIndex++
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

func (se *SerialExpect) parseTryBlock(config *Config, tryBlockName string) ([]Command, error) {
	// Find the try block
	var tryBlock *TryBlock
	for _, tb := range config.Tries {
		if tb.Name == tryBlockName {
			tryBlock = &tb
			break
		}
	}
	
	if tryBlock == nil {
		return nil, fmt.Errorf("try block %q not found", tryBlockName)
	}
	
	// Build script map for the try block
	scriptMap := make(map[string]NamedScript)
	for _, script := range config.Scripts {
		name := script.Name
		if name == "" {
			continue // Skip unnamed scripts in try blocks
		}
		scriptMap[name] = script
	}
	
	// Create a single try command that contains all the information
	tryCommand := Command{
		Type:      "try",
		Value:     tryBlockName,
		TryBlock:  tryBlock,
		ScriptMap: scriptMap,
	}
	
	return []Command{tryCommand}, nil
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
		} else if strings.HasPrefix(line, "monitor ") {
			value := strings.TrimPrefix(line, "monitor ")
			commands = append(commands, Command{Type: "monitor", Value: value})
		} else if line == "monitor" {
			// Monitor without parameters - monitor indefinitely
			commands = append(commands, Command{Type: "monitor", Value: ""})
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

func (se *SerialExpect) handleTry(cmd Command, readChan <-chan string) error {
	tryBlock := cmd.TryBlock
	scriptMap := cmd.ScriptMap
	
	se.logger.Printf("TRY: Executing try block %q with script %q", tryBlock.Name, tryBlock.Script)
	
	// Get the main script to execute
	mainScript, exists := scriptMap[tryBlock.Script]
	if !exists {
		return fmt.Errorf("script %q referenced by try block %q not found", tryBlock.Script, tryBlock.Name)
	}
	
	// Parse the main script commands
	commands, err := parseScript(mainScript.Content)
	if err != nil {
		return fmt.Errorf("failed to parse script %q in try block: %v", tryBlock.Script, err)
	}
	
	maxRetries := 1
	if tryBlock.Retry {
		maxRetries = 2 // Original attempt + 1 retry
	}
	
	var lastError error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			se.logger.Printf("TRY: Retrying script %q (attempt %d/%d)", tryBlock.Script, attempt, maxRetries)
		}
		
		// Execute the main script commands
		lastError = nil
		for _, subCmd := range commands {
			switch subCmd.Type {
			case "send":
				if err := se.handleSend(subCmd.Value); err != nil {
					lastError = err
					break
				}
			case "expect":
				if err := se.handleExpect(subCmd.Value, readChan); err != nil {
					lastError = err
					break
				}
			case "monitor":
				if err := se.handleMonitor(subCmd.Value, readChan); err != nil {
					lastError = err
					break
				}
			}
		}
		
		// If no error occurred, we're done
		if lastError == nil {
			se.logger.Printf("TRY: Script %q completed successfully", tryBlock.Script)
			return nil
		}
		
		se.logger.Printf("TRY: Script %q failed: %v", tryBlock.Script, lastError)
		
		// If this is the last attempt or we're not retrying, break
		if attempt >= maxRetries {
			break
		}
	}
	
	// If we get here, all attempts failed
	se.logger.Printf("TRY: All attempts failed for script %q", tryBlock.Script)
	
	// Execute except script if specified
	if tryBlock.Except != "" {
		se.logger.Printf("TRY: Executing except script %q", tryBlock.Except)
		
		exceptScript, exists := scriptMap[tryBlock.Except]
		if !exists {
			return fmt.Errorf("except script %q referenced by try block %q not found", tryBlock.Except, tryBlock.Name)
		}
		
		exceptCommands, err := parseScript(exceptScript.Content)
		if err != nil {
			return fmt.Errorf("failed to parse except script %q: %v", tryBlock.Except, err)
		}
		
		// Execute except script commands
		for _, exceptCmd := range exceptCommands {
			switch exceptCmd.Type {
			case "send":
				if err := se.handleSend(exceptCmd.Value); err != nil {
					se.logger.Printf("TRY: Error in except script: %v", err)
					// Continue with except script even if there are errors
				}
			case "expect":
				if err := se.handleExpect(exceptCmd.Value, readChan); err != nil {
					se.logger.Printf("TRY: Error in except script: %v", err)
					// Continue with except script even if there are errors
				}
			case "monitor":
				if err := se.handleMonitor(exceptCmd.Value, readChan); err != nil {
					se.logger.Printf("TRY: Error in except script: %v", err)
					// Continue with except script even if there are errors
				}
			}
		}
		
		se.logger.Printf("TRY: Except script %q completed", tryBlock.Except)
	}
	
	// Return the original error from the main script
	return fmt.Errorf("try block %q failed: %v", tryBlock.Name, lastError)
}

func (se *SerialExpect) handleMonitor(parameter string, readChan <-chan string) error {
	se.logger.Printf("MONITOR: Starting monitoring with parameter: %q", parameter)
	
	// Parse the monitor parameter
	var monitorDuration time.Duration
	var maxLines int
	var err error
	
	if parameter == "" {
		// Monitor indefinitely
		se.logger.Printf("MONITOR: Monitoring indefinitely (press Ctrl+C to stop)")
		monitorDuration = 24 * time.Hour // Set a very long duration
	} else {
		// Try to parse as duration first
		monitorDuration, err = time.ParseDuration(parameter)
		if err != nil {
			// Try to parse as line count
			maxLines, err = strconv.Atoi(parameter)
			if err != nil {
				return fmt.Errorf("invalid monitor parameter %q: must be duration (e.g., 5m30s) or line count (e.g., 50)", parameter)
			}
			se.logger.Printf("MONITOR: Monitoring for %d lines", maxLines)
		} else {
			se.logger.Printf("MONITOR: Monitoring for %v", monitorDuration)
		}
	}
	
	var buffer strings.Builder
	lineCount := 0
	startTime := time.Now()
	
	// Set up timeout if monitoring by duration
	var timeout <-chan time.Time
	if maxLines == 0 {
		timeout = time.After(monitorDuration)
	}
	
	for {
		select {
		case char := <-readChan:
			buffer.WriteByte(char[0]) // char is a string of length 1
			
			// Check for newline to count lines and output
			if char == "\n" {
				line := strings.TrimRight(buffer.String(), "\r\n")
				se.logger.Printf("RX: %s", line)
				buffer.Reset()
				lineCount++
				
				// Check if we've reached the line limit
				if maxLines > 0 && lineCount >= maxLines {
					se.logger.Printf("MONITOR: Reached %d lines, stopping", maxLines)
					return nil
				}
			}
			
		case <-timeout:
			if maxLines == 0 {
				elapsed := time.Since(startTime)
				se.logger.Printf("MONITOR: Duration %v elapsed, stopping (received %d lines)", elapsed.Round(time.Second), lineCount)
				return nil
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
