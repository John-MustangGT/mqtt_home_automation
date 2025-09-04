# MQTT Home Automation - Debian Package

This directory contains the Debian packaging system for the MQTT Home Automation project.

## Quick Start

### Building the Package

1. **Install prerequisites:**
   ```bash
   sudo apt-get update
   sudo apt-get install build-essential dpkg-dev golang-go
   ```

2. **Build the package:**
   ```bash
   # Using the build script directly
   ./scripts/build-deb.sh
   
   # Or using the root Makefile (if updated)
   make deb
   ```

3. **Install the package:**
   ```bash
   sudo dpkg -i build/mqtt-home-automation_1.0.0_amd64.deb
   sudo apt-get install -f  # Fix any dependency issues
   ```

### Starting the Services

After installation:

```bash
# Start the services
sudo systemctl start mqtt-home-automation
sudo systemctl start command-runner

# Check status
sudo systemctl status mqtt-home-automation
sudo systemctl status command-runner
```

## Package Contents

- **Home Automation Dashboard:** http://localhost:8080
- **Command Runner Interface:** http://localhost:8000
- **Configuration:** `/etc/mqtt-home-automation/`
- **Services:** `mqtt-home-automation.service`, `command-runner.service`

## Configuration

Edit `/etc/mqtt-home-automation/config.xml` to configure MQTT settings and devices.
Edit `/etc/mqtt-home-automation/command_runner.xml` to customize available commands.

For detailed documentation, see the generated package or project repository.
