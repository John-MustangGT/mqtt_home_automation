package main

import (
	"log"
	"strconv"
	"strings"
	"time"
)

func (app *App) startAutomationScheduler() {
	log.Println("Starting automation scheduler...")
	
	for _, automation := range app.config.Automations {
		if automation.Enabled {
			app.scheduleAutomation(automation)
		}
	}
}

func (app *App) scheduleAutomation(automation Automation) {
	app.automationMutex.Lock()
	defer app.automationMutex.Unlock()

	// Stop existing job if it exists
	if existingJob, exists := app.automationJobs[automation.ID]; exists {
		if existingJob.Timer != nil {
			existingJob.Timer.Stop()
		}
		if existingJob.StopTimer != nil {
			existingJob.StopTimer.Stop()
		}
	}

	job := &AutomationJob{
		ID:         automation.ID,
		Automation: automation,
		Running:    false,
	}

	switch automation.Schedule.Type {
	case "time":
		app.scheduleTimeBasedAutomation(job)
	case "interval":
		app.scheduleIntervalBasedAutomation(job)
	case "duration":
		app.scheduleDurationBasedAutomation(job)
	default:
		log.Printf("Unknown schedule type: %s for automation %s", automation.Schedule.Type, automation.ID)
		return
	}

	app.automationJobs[automation.ID] = job
	log.Printf("Scheduled automation: %s (%s)", automation.Name, automation.Schedule.Type)
}

func (app *App) scheduleTimeBasedAutomation(job *AutomationJob) {
	schedule := job.Automation.Schedule
	
	// Parse time (HH:MM format)
	timeParts := strings.Split(schedule.Time, ":")
	if len(timeParts) != 2 {
		log.Printf("Invalid time format for automation %s: %s", job.ID, schedule.Time)
		return
	}
	
	hour, err1 := strconv.Atoi(timeParts[0])
	minute, err2 := strconv.Atoi(timeParts[1])
	if err1 != nil || err2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		log.Printf("Invalid time values for automation %s: %s", job.ID, schedule.Time)
		return
	}
	
	// Parse allowed days if specified
	allowedDays := make(map[time.Weekday]bool)
	if schedule.Days != "" {
		dayMap := map[string]time.Weekday{
			"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday,
			"wed": time.Wednesday, "thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
		}
		
		for _, day := range strings.Split(strings.ToLower(schedule.Days), ",") {
			day = strings.TrimSpace(day)
			if weekday, exists := dayMap[day]; exists {
				allowedDays[weekday] = true
			}
		}
	} else {
		// If no days specified, allow all days
		for i := time.Sunday; i <= time.Saturday; i++ {
			allowedDays[i] = true
		}
	}
	
	// Calculate next execution time
	now := time.Now()
	nextRun := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	
	// If the time has passed today, move to tomorrow
	if nextRun.Before(now) {
		nextRun = nextRun.Add(24 * time.Hour)
	}
	
	// Find next allowed day
	for !allowedDays[nextRun.Weekday()] {
		nextRun = nextRun.Add(24 * time.Hour)
	}
	
	// Check date range if specified
	if schedule.StartDate != "" {
		if startDate, err := time.Parse("2006-01-02", schedule.StartDate); err == nil && nextRun.Before(startDate) {
			nextRun = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), hour, minute, 0, 0, now.Location())
		}
	}
	
	if schedule.EndDate != "" {
		if endDate, err := time.Parse("2006-01-02", schedule.EndDate); err == nil && nextRun.After(endDate) {
			log.Printf("Automation %s is past its end date, not scheduling", job.ID)
			return
		}
	}
	
	job.NextRun = nextRun
	duration := time.Until(nextRun)
	
	job.Timer = time.AfterFunc(duration, func() {
		app.executeAutomation(job)
		// Reschedule for next day
		go func() {
			time.Sleep(1 * time.Second) // Small delay to avoid race conditions
			app.scheduleTimeBasedAutomation(job)
		}()
	})
	
	log.Printf("Time-based automation %s scheduled for %s (in %v)", job.ID, nextRun.Format("2006-01-02 15:04:05"), duration)
}

func (app *App) scheduleIntervalBasedAutomation(job *AutomationJob) {
	schedule := job.Automation.Schedule
	
	// Parse interval (e.g., "1h", "30m", "10s")
	interval, err := time.ParseDuration(schedule.Interval)
	if err != nil {
		log.Printf("Invalid interval format for automation %s: %s", job.ID, schedule.Interval)
		return
	}
	
	job.NextRun = time.Now().Add(interval)
	
	job.Timer = time.AfterFunc(interval, func() {
		app.executeAutomation(job)
		// Reschedule for next interval
		go func() {
			time.Sleep(1 * time.Second)
			app.scheduleIntervalBasedAutomation(job)
		}()
	})
	
	log.Printf("Interval-based automation %s scheduled every %v (next run: %s)", 
		job.ID, interval, job.NextRun.Format("2006-01-02 15:04:05"))
}

func (app *App) scheduleDurationBasedAutomation(job *AutomationJob) {
	schedule := job.Automation.Schedule
	
	// Parse interval and duration
	interval, err1 := time.ParseDuration(schedule.Interval)
	duration, err2 := time.ParseDuration(schedule.Duration)
	
	if err1 != nil {
		log.Printf("Invalid interval format for automation %s: %s", job.ID, schedule.Interval)
		return
	}
	if err2 != nil {
		log.Printf("Invalid duration format for automation %s: %s", job.ID, schedule.Duration)
		return
	}
	
	job.NextRun = time.Now().Add(interval)
	
	job.Timer = time.AfterFunc(interval, func() {
		// Execute ON action
		app.executeAutomationAction(job, true)
		job.Running = true
		
		// Schedule OFF action after duration
		job.StopTimer = time.AfterFunc(duration, func() {
			app.executeAutomationAction(job, false)
			job.Running = false
		})
		
		// Reschedule for next interval
		go func() {
			time.Sleep(1 * time.Second)
			app.scheduleDurationBasedAutomation(job)
		}()
	})
	
	log.Printf("Duration-based automation %s scheduled every %v for %v (next run: %s)", 
		job.ID, interval, duration, job.NextRun.Format("2006-01-02 15:04:05"))
}

func (app *App) executeAutomation(job *AutomationJob) {
	log.Printf("Executing automation: %s (%s)", job.Automation.Name, job.ID)
	
	// For simple automations (time, interval), just execute the action
	app.executeAutomationAction(job, true)
}

func (app *App) executeAutomationAction(job *AutomationJob, isOnAction bool) {
	action := job.Automation.Action
	
	// Determine which payload to use
	var payload string
	if isOnAction && action.OnPayload != "" {
		payload = action.OnPayload
	} else if !isOnAction && action.OffPayload != "" {
		payload = action.OffPayload
	} else {
		payload = action.Payload
	}
	
	// Execute local command if specified
	if action.LocalCommand != "" {
		go app.executeLocalCommand(action.LocalCommand)
		log.Printf("Executed local command for automation %s: %s", job.ID, action.LocalCommand)
	}
	
	// Send MQTT command if specified
	if action.Topic != "" && payload != "" {
		token := app.mqttClient.Publish(action.Topic, 1, false, payload)
		if token.Wait() && token.Error() != nil {
			log.Printf("Failed to publish automation MQTT message: %v", token.Error())
		} else {
			log.Printf("Sent automation MQTT command - Topic: %s, Payload: %s", action.Topic, payload)
			app.addMQTTLogEntry(action.Topic+" (AUTO)", payload)
		}
	}
}

func (app *App) stopAutomation(automationID string) {
	app.automationMutex.Lock()
	defer app.automationMutex.Unlock()
	
	if job, exists := app.automationJobs[automationID]; exists {
		if job.Timer != nil {
			job.Timer.Stop()
		}
		if job.StopTimer != nil {
			job.StopTimer.Stop()
		}
		delete(app.automationJobs, automationID)
		log.Printf("Stopped automation: %s", automationID)
	}
}

func (app *App) getAutomationStatus() map[string]interface{} {
	app.automationMutex.RLock()
	defer app.automationMutex.RUnlock()
	
	status := make(map[string]interface{})
	
	for id, job := range app.automationJobs {
		status[id] = map[string]interface{}{
			"name":     job.Automation.Name,
			"enabled":  job.Automation.Enabled,
			"nextRun":  job.NextRun.Format(time.RFC3339),
			"running":  job.Running,
			"schedule": job.Automation.Schedule,
		}
	}
	
	return status
}
