package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
)


func (app *App) loadConfig(filename string) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read config file '%s': %v", filename, err)
	}

	if err := xml.Unmarshal(data, &app.config); err != nil {
		return fmt.Errorf("failed to parse XML config: %v", err)
	}

	// Set default MQTT log size if not specified
	if app.config.MQTTLogSize <= 0 {
		app.config.MQTTLogSize = 20
	}

	// Debug: Print parsed configuration
	log.Printf("Loaded configuration from: %s", filename)
	log.Printf("Loaded %d devices", len(app.config.Devices))
	for _, device := range app.config.Devices {
		log.Printf("Device: %s (%s)", device.Name, device.ID)
		for i, control := range device.Controls {
			log.Printf("  Control %d: Type=%s, Label=%s, Topic='%s', Payload='%s', LocalCommand='%s'",
				i, control.Type, control.Label, control.Topic, control.Payload, control.LocalCommand)
		}
	}

	return nil
}
