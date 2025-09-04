#!/bin/bash
# Setup script to create the complete Debian packaging structure

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$SCRIPT_DIR"

echo "Setting up Debian packaging structure in $ROOT_DIR"

# Create directory structure
mkdir -p "$ROOT_DIR/scripts"
mkdir -p "$ROOT_DIR/debian"

echo "Creating build script..."

# Create the build script
cat > "$ROOT_DIR/scripts/build-deb.sh" << 'BUILD_SCRIPT_EOF'
#!/bin/bash
# Complete build script for MQTT Home Automation Debian package

set -e

# Configuration
PROJECT_NAME="mqtt-home-automation"
VERSION="1.0.0"
ARCH="amd64"
MAINTAINER="John-MustangGT <maintainer@example.com>"

# Directories
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$ROOT_DIR/build"
PACKAGE_NAME="${PROJECT_NAME}_${VERSION}_${ARCH}"
PACKAGE_DIR="$BUILD_DIR/$PACKAGE_NAME"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.18 or later."
    fi
    
    if ! command -v dpkg-deb &> /dev/null; then
        log_error "dpkg-deb is not installed. Please install dpkg-dev package."
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    log_info "Go version: $GO_VERSION"
    
    log_success "Prerequisites check passed"
}

# Clean previous builds
clean_build() {
    log_info "Cleaning previous build artifacts..."
    
    cd "$ROOT_DIR"
    
    if [ -d "$BUILD_DIR" ]; then
        rm -rf "$BUILD_DIR"
        log_info "Removed $BUILD_DIR"
    fi
    
    find . -name "home-automation-server" -delete 2>/dev/null || true
    find . -name "command_runner_server" -delete 2>/dev/null || true
    find . -name "mqtt_listener" -delete 2>/dev/null || true
    find . -name "serial_expect" -delete 2>/dev/null || true
    
    log_success "Build cleanup completed"
}

# Build Go binaries
build_binaries() {
    log_info "Building Go binaries..."
    
    cd "$ROOT_DIR"
    
    BUILD_FLAGS="-ldflags=-w -s -trimpath"
    
    log_info "Building home-automation-server..."
    if [ -d "cmd/home-automation-server" ]; then
        cd cmd/home-automation-server
        go build $BUILD_FLAGS -o home-automation-server .
        cd "$ROOT_DIR"
    else
        log_warning "cmd/home-automation-server directory not found"
    fi
    
    log_info "Building command_runner_server..."
    if [ -d "cmd/command_runner_server" ]; then
        cd cmd/command_runner_server
        go build $BUILD_FLAGS -o command_runner_server main.go
        cd "$ROOT_DIR"
    else
        log_warning "cmd/command_runner_server directory not found"
    fi
    
    log_info "Building mqtt_listener..."
    if [ -d "cmd/mqtt_listener" ]; then
        cd cmd/mqtt_listener
        go build $BUILD_FLAGS -o mqtt_listener main.go
        cd "$ROOT_DIR"
    else
        log_warning "cmd/mqtt_listener directory not found"
    fi
    
    log_info "Building serial_expect..."
    if [ -d "cmd/serial_expect" ]; then
        cd cmd/serial_expect
        go build $BUILD_FLAGS -o serial_expect main.go
        cd "$ROOT_DIR"
    else
        log_warning "cmd/serial_expect directory not found"
    fi
    
    log_success "Binary build completed"
}

# Create package directory structure
create_package_structure() {
    log_info "Creating package directory structure..."
    
    mkdir -p "$PACKAGE_DIR/DEBIAN"
    mkdir -p "$PACKAGE_DIR/usr/local/bin"
    mkdir -p "$PACKAGE_DIR/usr/local/lib/$PROJECT_NAME"
    mkdir -p "$PACKAGE_DIR/etc/$PROJECT_NAME"
    mkdir -p "$PACKAGE_DIR/etc/systemd/system"
    mkdir -p "$PACKAGE_DIR/usr/local/lib/$PROJECT_NAME/web"
    mkdir -p "$PACKAGE_DIR/usr/local/lib/$PROJECT_NAME/command_runner_web"
    
    log_success "Package directory structure created"
}

# Copy files to package directory
copy_files() {
    log_info "Copying files to package directory..."
    
    # Copy binaries
    if [ -f "cmd/home-automation-server/home-automation-server" ]; then
        cp cmd/home-automation-server/home-automation-server "$PACKAGE_DIR/usr/local/bin/"
    fi
    if [ -f "cmd/command_runner_server/command_runner_server" ]; then
        cp cmd/command_runner_server/command_runner_server "$PACKAGE_DIR/usr/local/bin/"
    fi
    if [ -f "cmd/mqtt_listener/mqtt_listener" ]; then
        cp cmd/mqtt_listener/mqtt_listener "$PACKAGE_DIR/usr/local/bin/"
    fi
    if [ -f "cmd/serial_expect/serial_expect" ]; then
        cp cmd/serial_expect/serial_expect "$PACKAGE_DIR/usr/local/bin/"
    fi
    log_info "Copied binaries"
    
    # Copy web files
    if [ -d "web" ]; then
        cp -r web/* "$PACKAGE_DIR/usr/local/lib/$PROJECT_NAME/web/" 2>/dev/null || true
        log_info "Copied main web files"
    fi
    
    # Copy command runner web files
    if [ -d "cmd/command_runner_server/web" ]; then
        cp -r cmd/command_runner_server/web/* "$PACKAGE_DIR/usr/local/lib/$PROJECT_NAME/command_runner_web/" 2>/dev/null || true
        log_info "Copied command runner web files"
    fi
    
    # Copy configuration files
    if [ -d "configs" ]; then
        cp configs/config.xml "$PACKAGE_DIR/etc/$PROJECT_NAME/" 2>/dev/null || true
        cp configs/commands.xml "$PACKAGE_DIR/etc/$PROJECT_NAME/" 2>/dev/null || true
        cp configs/serial.xml "$PACKAGE_DIR/etc/$PROJECT_NAME/" 2>/dev/null || true
        log_info "Copied configuration files"
    fi
    
    # Create command_runner.xml
    cat > "$PACKAGE_DIR/etc/$PROJECT_NAME/command_runner.xml" << 'XML_CONFIG_EOF'
<?xml version="1.0" encoding="UTF-8"?>
<config>
    <server>
        <interface>0.0.0.0</interface>
        <port>8000</port>
        <webdir>/usr/local/lib/mqtt-home-automation/command_runner_web</webdir>
        <ui_framework>bootstrap</ui_framework>
    </server>
    
    <buttons>
        <button>
            <name>system_info</name>
            <display_name>📊 System Info</display_name>
            <command>uname -a</command>
            <size>md</size>
            <color>primary</color>
        </button>
        
        <button>
            <name>disk_usage</name>
            <display_name>💾 Disk Usage</display_name>
            <command>df -h</command>
            <size>md</size>
            <color>info</color>
        </button>
        
        <button>
            <name>memory_usage</name>
            <display_name>🧠 Memory Usage</display_name>
            <command>free -h</command>
            <size>md</size>
            <color>success</color>
        </button>
        
        <button>
            <name>list_processes</name>
            <display_name>⚙️ Running Processes</display_name>
            <command>ps aux | head -20</command>
            <size>lg</size>
            <color>warning</color>
        </button>
        
        <button>
            <name>network_info</name>
            <display_name>🌐 Network</display_name>
            <command>ip addr show</command>
            <size>sm</size>
            <color>secondary</color>
        </button>
        
        <button>
            <name>uptime</name>
            <display_name>⏰ Server Uptime</display_name>
            <command>uptime</command>
            <size>sm</size>
            <color>dark</color>
        </button>
        
        <button>
            <name>current_date</name>
            <display_name>📅 Date &amp; Time</display_name>
            <command>date</command>
            <size>sm</size>
            <color>light</color>
        </button>
    </buttons>
</config>
XML_CONFIG_EOF
    
    log_success "File copying completed"
}

# Create systemd service files
create_systemd_services() {
    log_info "Creating systemd service files..."
    
    # Create mqtt-home-automation.service
    cat > "$PACKAGE_DIR/etc/systemd/system/mqtt-home-automation.service" << 'SERVICE_EOF'
[Unit]
Description=MQTT Home Automation Server
Documentation=https://github.com/John-MustangGT/mqtt_home_automation
After=network.target network-online.target
Wants=network-online.target
ConditionPathExists=/etc/mqtt-home-automation/config.xml

[Service]
Type=simple
User=mqtt-automation
Group=mqtt-automation

WorkingDirectory=/usr/local/lib/mqtt-home-automation
ExecStart=/usr/local/bin/home-automation-server \
    -config /etc/mqtt-home-automation/config.xml \
    -webdir /usr/local/lib/mqtt-home-automation/web

Restart=always
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60

StandardOutput=journal
StandardError=journal
SyslogIdentifier=mqtt-home-automation

NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/log/mqtt-home-automation /var/lib/mqtt-home-automation

Environment=PATH=/usr/local/bin:/usr/bin:/bin
Environment=HOME=/var/lib/mqtt-home-automation

[Install]
WantedBy=multi-user.target
SERVICE_EOF

    # Create command-runner.service
    cat > "$PACKAGE_DIR/etc/systemd/system/command-runner.service" << 'SERVICE2_EOF'
[Unit]
Description=Command Runner HTTP Server
Documentation=https://github.com/John-MustangGT/mqtt_home_automation
After=network.target
ConditionPathExists=/etc/mqtt-home-automation/command_runner.xml

[Service]
Type=simple
User=mqtt-automation
Group=mqtt-automation

WorkingDirectory=/usr/local/lib/mqtt-home-automation
ExecStart=/usr/local/bin/command_runner_server \
    -config /etc/mqtt-home-automation/command_runner.xml

Restart=always
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60

StandardOutput=journal
StandardError=journal
SyslogIdentifier=command-runner

NoNewPrivileges=no
PrivateTmp=yes
ProtectSystem=false
ProtectHome=no
ReadWritePaths=/var/log/mqtt-home-automation /var/lib/mqtt-home-automation

Environment=PATH=/usr/local/bin:/usr/bin:/bin:/sbin:/usr/sbin
Environment=HOME=/var/lib/mqtt-home-automation

[Install]
WantedBy=multi-user.target
SERVICE2_EOF

    log_success "Systemd service files created"
}

# Generate Debian control files
generate_debian_files() {
    log_info "Generating Debian control files..."
    
    # Calculate installed size safely
    if [ -d "$PACKAGE_DIR" ]; then
        INSTALLED_SIZE=$(du -sk "$PACKAGE_DIR" 2>/dev/null | cut -f1 || echo "1000")
    else
        INSTALLED_SIZE="1000"
    fi
    
    # Generate control file
    cat > "$PACKAGE_DIR/DEBIAN/control" << CONTROL_EOF
Package: $PROJECT_NAME
Version: $VERSION
Section: net
Priority: optional
Architecture: $ARCH
Depends: systemd, adduser
Maintainer: $MAINTAINER
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
CONTROL_EOF

    # Generate postinst script
    cat > "$PACKAGE_DIR/DEBIAN/postinst" << 'POSTINST_EOF'
#!/bin/bash
set -e

USER_NAME="mqtt-automation"
GROUP_NAME="mqtt-automation"
CONFIG_DIR="/etc/mqtt-home-automation"
LIB_DIR="/usr/local/lib/mqtt-home-automation"
LOG_DIR="/var/log/mqtt-home-automation"
DATA_DIR="/var/lib/mqtt-home-automation"

case "$1" in
    configure)
        echo "Configuring MQTT Home Automation..."
        
        if ! getent group "$GROUP_NAME" > /dev/null 2>&1; then
            addgroup --system "$GROUP_NAME"
        fi
        
        if ! getent passwd "$USER_NAME" > /dev/null 2>&1; then
            adduser --system --ingroup "$GROUP_NAME" --home "$DATA_DIR" \
                    --no-create-home --shell /bin/false "$USER_NAME"
        fi
        
        mkdir -p "$LOG_DIR" "$DATA_DIR"
        
        chown -R "$USER_NAME:$GROUP_NAME" "$CONFIG_DIR" "$LOG_DIR" "$DATA_DIR" "$LIB_DIR"
        chmod 640 "$CONFIG_DIR"/*.xml
        chmod +x /usr/local/bin/home-automation-server
        chmod +x /usr/local/bin/command_runner_server
        chmod +x /usr/local/bin/mqtt_listener
        chmod +x /usr/local/bin/serial_expect
        
        systemctl daemon-reload
        systemctl enable mqtt-home-automation.service
        systemctl enable command-runner.service
        
        echo ""
        echo "MQTT Home Automation installed successfully!"
        echo "Configuration: $CONFIG_DIR"
        echo "Web interface: http://localhost:8080 (home automation)"
        echo "Command runner: http://localhost:8000"
        echo ""
        echo "Start services with:"
        echo "  sudo systemctl start mqtt-home-automation"
        echo "  sudo systemctl start command-runner"
        ;;
esac
exit 0
POSTINST_EOF

    # Generate prerm script
    cat > "$PACKAGE_DIR/DEBIAN/prerm" << 'PRERM_EOF'
#!/bin/bash
set -e

case "$1" in
    remove|upgrade|deconfigure)
        if systemctl is-active --quiet mqtt-home-automation.service; then
            systemctl stop mqtt-home-automation.service
        fi
        if systemctl is-active --quiet command-runner.service; then
            systemctl stop command-runner.service
        fi
        if systemctl is-enabled --quiet mqtt-home-automation.service; then
            systemctl disable mqtt-home-automation.service
        fi
        if systemctl is-enabled --quiet command-runner.service; then
            systemctl disable command-runner.service
        fi
        ;;
esac
exit 0
PRERM_EOF

    # Generate postrm script
    cat > "$PACKAGE_DIR/DEBIAN/postrm" << 'POSTRM_EOF'
#!/bin/bash
set -e

case "$1" in
    purge)
        if getent passwd "mqtt-automation" > /dev/null 2>&1; then
            deluser --quiet "mqtt-automation" || true
        fi
        if getent group "mqtt-automation" > /dev/null 2>&1; then
            delgroup --quiet "mqtt-automation" || true
        fi
        rm -rf "/var/lib/mqtt-home-automation" || true
        echo "Config and logs preserved in /etc/mqtt-home-automation and /var/log/mqtt-home-automation"
        ;;
    remove|upgrade|failed-upgrade|abort-install|abort-upgrade|disappear)
        systemctl daemon-reload || true
        ;;
esac
exit 0
POSTRM_EOF

    # Make scripts executable
    chmod 755 "$PACKAGE_DIR/DEBIAN/postinst"
    chmod 755 "$PACKAGE_DIR/DEBIAN/prerm"
    chmod 755 "$PACKAGE_DIR/DEBIAN/postrm"
    
    log_success "Debian control files generated"
}

# Build the final package
build_package() {
    log_info "Building .deb package..."
    
    cd "$BUILD_DIR"
    if dpkg-deb --build "$PACKAGE_NAME"; then
        if [ -f "${PACKAGE_NAME}.deb" ]; then
            log_success "Package built successfully: ${PACKAGE_NAME}.deb"
            
            log_info "Package information:"
            dpkg-deb --info "${PACKAGE_NAME}.deb" || true
            
            PACKAGE_SIZE=$(ls -lh "${PACKAGE_NAME}.deb" 2>/dev/null | awk '{print $5}' || echo "unknown")
            log_success "Package size: $PACKAGE_SIZE"
            
            echo ""
            log_info "To install the package:"
            echo "  sudo dpkg -i $BUILD_DIR/${PACKAGE_NAME}.deb"
            echo "  sudo apt-get install -f  # Fix dependencies if needed"
        else
            log_error "Package file was not created"
        fi
    else
        log_error "Failed to build package"
    fi
}

# Main execution
main() {
    log_info "Starting MQTT Home Automation Debian package build"
    log_info "Project: $PROJECT_NAME v$VERSION ($ARCH)"
    
    check_prerequisites
    clean_build
    build_binaries
    create_package_structure
    copy_files
    create_systemd_services
    generate_debian_files
    build_package
    
    log_success "Build completed successfully!"
}

# Run main function
main "$@"
BUILD_SCRIPT_EOF

chmod +x "$ROOT_DIR/scripts/build-deb.sh"

echo "Creating simple generate script..."

# Create generate-debian-files.sh script for the Makefile
cat > "$ROOT_DIR/scripts/generate-debian-files.sh" << 'GENERATE_SCRIPT_EOF'
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

mkdir -p "$DEBIAN_DIR"

echo "Generating Debian package files for version $VERSION ($ARCH)..."

# Calculate installed size safely
if [ -d "$PACKAGE_DIR" ]; then
    INSTALLED_SIZE=$(du -sk "$PACKAGE_DIR" 2>/dev/null | cut -f1 || echo "1000")
else
    INSTALLED_SIZE="1000"
fi

cat > "$DEBIAN_DIR/control" << CONTROL_EOF
Package: mqtt-home-automation
Version: $VERSION
Section: net
Priority: optional
Architecture: $ARCH
Depends: systemd, adduser
Maintainer: John-MustangGT <maintainer@example.com>
Description: MQTT Home Automation Server Suite
 A comprehensive MQTT-based home automation server with multiple components.
Homepage: https://github.com/John-MustangGT/mqtt_home_automation
Installed-Size: $INSTALLED_SIZE
CONTROL_EOF

echo "Debian package files generated successfully in $DEBIAN_DIR"
GENERATE_SCRIPT_EOF

chmod +x "$ROOT_DIR/scripts/generate-debian-files.sh"

echo "Creating systemd service files..."

# Create systemd service files in debian directory
cat > "$ROOT_DIR/debian/mqtt-home-automation.service" << 'MAIN_SERVICE_EOF'
[Unit]
Description=MQTT Home Automation Server
Documentation=https://github.com/John-MustangGT/mqtt_home_automation
After=network.target network-online.target
Wants=network-online.target
ConditionPathExists=/etc/mqtt-home-automation/config.xml

[Service]
Type=simple
User=mqtt-automation
Group=mqtt-automation

WorkingDirectory=/usr/local/lib/mqtt-home-automation
ExecStart=/usr/local/bin/home-automation-server \
    -config /etc/mqtt-home-automation/config.xml \
    -webdir /usr/local/lib/mqtt-home-automation/web

Restart=always
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60

StandardOutput=journal
StandardError=journal
SyslogIdentifier=mqtt-home-automation

NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/log/mqtt-home-automation /var/lib/mqtt-home-automation

Environment=PATH=/usr/local/bin:/usr/bin:/bin
Environment=HOME=/var/lib/mqtt-home-automation

[Install]
WantedBy=multi-user.target
MAIN_SERVICE_EOF

cat > "$ROOT_DIR/debian/command-runner.service" << 'CMD_SERVICE_EOF'
[Unit]
Description=Command Runner HTTP Server
Documentation=https://github.com/John-MustangGT/mqtt_home_automation
After=network.target
ConditionPathExists=/etc/mqtt-home-automation/command_runner.xml

[Service]
Type=simple
User=mqtt-automation
Group=mqtt-automation

WorkingDirectory=/usr/local/lib/mqtt-home-automation
ExecStart=/usr/local/bin/command_runner_server \
    -config /etc/mqtt-home-automation/command_runner.xml

Restart=always
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=60

StandardOutput=journal
StandardError=journal
SyslogIdentifier=command-runner

NoNewPrivileges=no
PrivateTmp=yes
ProtectSystem=false
ProtectHome=no
ReadWritePaths=/var/log/mqtt-home-automation /var/lib/mqtt-home-automation

Environment=PATH=/usr/local/bin:/usr/bin:/bin:/sbin:/usr/sbin
Environment=HOME=/var/lib/mqtt-home-automation

[Install]
WantedBy=multi-user.target
CMD_SERVICE_EOF

# Create README
cat > "$ROOT_DIR/DEBIAN_PACKAGING.md" << 'README_EOF'
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
README_EOF

echo ""
echo "✅ Debian packaging structure created successfully!"
echo ""
echo "Files created:"
echo "  📝 scripts/build-deb.sh - Main build script"
echo "  📝 scripts/generate-debian-files.sh - Debian control files generator"
echo "  📝 debian/mqtt-home-automation.service - Main service file"
echo "  📝 debian/command-runner.service - Command runner service file"
echo "  📝 DEBIAN_PACKAGING.md - Documentation"
echo ""
echo "To build the Debian package:"
echo "  ./scripts/build-deb.sh"
echo ""
echo "Note: Make sure you have Go installed and your project structure matches"
echo "      the expected layout with cmd/, configs/, and web/ directories."
