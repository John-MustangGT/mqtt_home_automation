package main

import (
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func (app *App) executeLocalCommand(command string) {
	log.Printf("Executing local command: %s", command)

	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()

	if err != nil {
		log.Printf("Local command failed: %v, Output: %s", err, string(output))
	} else {
		log.Printf("Local command executed successfully. Output: %s", string(output))
	}
}

func (app *App) getSystemStats() SystemStats {
	stats := SystemStats{}

	// Get uptime
	if output, err := exec.Command("uptime", "-p").Output(); err == nil {
		stats.Uptime = strings.TrimSpace(string(output))
	}

	// Get load average
	if output, err := exec.Command("cat", "/proc/loadavg").Output(); err == nil {
		fields := strings.Fields(string(output))
		if len(fields) >= 3 {
			if val, err := strconv.ParseFloat(fields[0], 64); err == nil {
				stats.LoadAvg1 = val
			}
			if val, err := strconv.ParseFloat(fields[1], 64); err == nil {
				stats.LoadAvg5 = val
			}
			if val, err := strconv.ParseFloat(fields[2], 64); err == nil {
				stats.LoadAvg15 = val
			}
		}
	}

	// Get memory info
	if output, err := exec.Command("free", "-m").Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			// Parse memory line: Mem: total used free shared buff/cache available
			memLine := regexp.MustCompile(`\s+`).Split(lines[1], -1)
			if len(memLine) >= 3 {
				if total, err := strconv.ParseFloat(memLine[1], 64); err == nil {
					stats.MemoryTotal = total
				}
				if used, err := strconv.ParseFloat(memLine[2], 64); err == nil {
					stats.MemoryUsed = used
				}
			}
		}
	}

	// Get CPU count
	if output, err := exec.Command("nproc").Output(); err == nil {
		if cpus, err := strconv.Atoi(strings.TrimSpace(string(output))); err == nil {
			stats.CPUCount = cpus
		}
	}

	return stats
}
