#!/bin/bash
# Generate Debian package control files
# Usage: ./generate-debian-files.sh VERSION ARCH PACKAGE_DIR

set -e

if [ $# -ne 3 ]; then
    echo "Usage: $0 VERSION ARCH PACKAGE_DIR"
    exit 1
fi

VERSION="$1"
ARCH="$2"
PACKAGE_DIR="$3"
DEBIAN_DIR="$PACKAGE_DIR/DEBIAN"

# Create DEBIAN directory if it doesn't exist
mkdir -p "$DEBIAN_DIR"

echo "Generating Debian package files for version $VERSION ($ARCH)..."

# Calculate installed size in KB
INSTALLED_SIZE=$(du -sk "$PACKAGE_DIR" | cut -f1)

# Generate control file
echo "Generating control file..."
cat > "$DEBIAN_DIR/control" << EOF
Package: mqtt-home-automation
Version: $VERSION
Section: net
Priority: optional
Architecture: $ARCH
Depends: systemd, adduser
Maintainer: John-MustangGT <maintainer@example.com>
Description: MQTT Home Automation Server Suite
 A comprehensive MQTT-based home automation server with multiple components:
 .
 - Home Automation Server: Web-based MQTT device control dashboard
 - Command Runner Server: HTTP server for executing system commands
 - MQTT Listener: Command execution via MQTT messages
 - Serial Expect: Serial port automation with expect-like functionality
 .
 The package includes web interfaces, configuration management, and
 systemd service integration for reliable operation.
Homepage: https://github.com/John-MustangGT/mqtt_home_automation
Installed-Size: $INSTALLED_SIZE
EOF

# Generate postinst script
echo "Generating postinst script..."
cat > "$DEBIAN_DIR/postinst" << 'EOF'
#!/bin/bash
# Post-installation script for mqtt-home-automation

set -e

# Configuration
USER_NAME="mqtt-automation"
GROUP_NAME="mqtt-automation"
CONFIG_DIR="/etc/mqtt-home-automation"
LIB_DIR="/usr/local/lib/mqtt-home-automation"
LOG_DIR="/var/log/mqtt-home-automation"
DATA_DIR="/var/lib/mqtt-home-automation"

case "$1" in
    configure)
        echo "Configuring MQTT Home Automation..."
        
        # Create system user and group
        if ! getent group "$GROUP_NAME" > /dev/null 2>&1; then
            echo "Creating group: $GROUP_NAME"
            addgroup --system "$GROUP_NAME"
        fi
        
        if ! getent passwd "$USER_NAME" > /dev/null 2>&1; then
            echo "Creating user: $USER_NAME"
            adduser --system --ingroup "$GROUP_NAME" --home "$DATA_DIR" \
                    --no-create-home --shell /bin/false "$USER_NAME"
        fi
        
        # Create necessary directories
        mkdir -p "$LOG_DIR"
        mkdir -p "$DATA_DIR"
        
        # Set permissions
        chown -R "$USER_NAME:$GROUP_NAME" "$CONFIG_DIR"
        chown -R "$USER_NAME:$GROUP_NAME" "$LOG_DIR"
        chown -R "$USER_NAME:$GROUP_NAME" "$DATA_DIR"
        chown -R "$USER_NAME:$GROUP_NAME" "$LIB_DIR"
        
        # Set proper permissions on config files
        chmod 640 "$CONFIG_DIR"/*.xml
        
        # Set executable permissions on binaries
        chmod +x /usr/local/bin/home-automation-server
        chmod +x /usr/local/bin/command_runner_server
        chmod +x /usr/local/bin/mqtt_listener
        chmod +x /usr/local/bin/serial_expect
        
        # Reload systemd
        systemctl daemon-reload
        
        # Enable services (but don't start them automatically)
        systemctl enable mqtt-home-automation.service
        systemctl enable command-runner.service
        
        echo ""
        echo "MQTT Home Automation has been installed successfully!"
        echo ""
        echo "Configuration files are located in: $CONFIG_DIR"
        echo "Web files are located in: $LIB_DIR"
        echo "Logs will be stored in: $LOG_DIR"
        echo ""
        echo "Before starting the services, please:"
        echo "1. Review and update the configuration files in $CONFIG_DIR"
        echo "2. Ensure your MQTT broker is accessible"
        echo "3. Configure any required GPIO or serial permissions"
        echo ""
        echo "To start the services:"
        echo "  sudo systemctl start mqtt-home-automation"
        echo "  sudo systemctl start command-runner"
        echo ""
        echo "To view service status:"
        echo "  sudo systemctl status mqtt-home-automation"
        echo "  sudo systemctl status command-runner"
        echo ""
        echo "Web interfaces will be available at:"
        echo "  Home Automation: http://localhost:8080"
        echo "  Command Runner:  http://localhost:8000"
        ;;
        
    abort-upgrade|abort-remove|abort-deconfigure)
        ;;
        
    *)
        echo "postinst called with unknown argument \`$1'" >&2
        exit 1
        ;;
esac

exit 0
EOF

# Generate prerm script
echo "Generating prerm script..."
cat > "$DEBIAN_DIR/prerm" << 'EOF'
#!/bin/bash
# Pre-removal script for mqtt-home-automation

set -e

case "$1" in
    remove|upgrade|deconfigure)
        echo "Stopping MQTT Home Automation services..."
        
        # Stop services if they're running
        if systemctl is-active --quiet mqtt-home-automation.service; then
            systemctl stop mqtt-home-automation.service
        fi
        
        if systemctl is-active --quiet command-runner.service; then
            systemctl stop command-runner.service
        fi
        
        # Disable services
        if systemctl is-enabled --quiet mqtt-home-automation.service; then
            systemctl disable mqtt-home-automation.service
        fi
        
        if systemctl is-enabled --quiet command-runner.service; then
            systemctl disable command-runner.service
        fi
        ;;
        
    failed-upgrade)
        ;;
        
    *)
        echo "prerm called with unknown argument \`$1'" >&2
        exit 1
        ;;
esac

exit 0
EOF

# Generate postrm script
echo "Generating postrm script..."
cat > "$DEBIAN_DIR/postrm" << 'EOF'
#!/bin/bash
# Post-removal script for mqtt-home-automation

set -e

USER_NAME="mqtt-automation"
GROUP_NAME="mqtt-automation"
LOG_DIR="/var/log/mqtt-home-automation"
DATA_DIR="/var/lib/mqtt-home-automation"

case "$1" in
    purge)
        echo "Purging MQTT Home Automation..."
        
        # Remove user and group
        if getent passwd "$USER_NAME" > /dev/null 2>&1; then
            echo "Removing user: $USER_NAME"
            deluser --quiet "$USER_NAME" || true
        fi
        
        if getent group "$GROUP_NAME" > /dev/null 2>&1; then
            echo "Removing group: $GROUP_NAME"
            delgroup --quiet "$GROUP_NAME" || true
        fi
        
        # Remove data directories (but preserve logs and config)
        if [ -d "$DATA_DIR" ]; then
            echo "Removing data directory: $DATA_DIR"
            rm -rf "$DATA_DIR"
        fi
        
        # Ask about removing logs and configuration
        echo ""
        echo "The following directories contain user data and logs:"
        echo "  Configuration: /etc/mqtt-home-automation"
        echo "  Logs: $LOG_DIR"
        echo ""
        echo "These have been preserved. Remove manually if desired:"
        echo "  sudo rm -rf /etc/mqtt-home-automation"
        echo "  sudo rm -rf $LOG_DIR"
        ;;
        
    remove|upgrade|failed-upgrade|abort-install|abort-upgrade|disappear)
        # Reload systemd after removing service files
        systemctl daemon-reload || true
        ;;
        
    *)
        echo "postrm called with unknown argument \`$1'" >&2
        exit 1
        ;;
esac

exit 0
EOF

# Set executable permissions on scripts
chmod 755 "$DEBIAN_DIR/postinst"
chmod 755 "$DEBIAN_DIR/prerm" 
chmod 755 "$DEBIAN_DIR/postrm"

echo "Debian package files generated successfully in $DEBIAN_DIR"
