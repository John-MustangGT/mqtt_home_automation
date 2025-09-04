# MQTT Home Automation - Root Makefile
PROJECT_NAME = mqtt-home-automation
VERSION = 1.0.0
ARCH = amd64
PACKAGE_NAME = $(PROJECT_NAME)_$(VERSION)_$(ARCH)

# Build directories
BUILD_DIR = build
PACKAGE_DIR = $(BUILD_DIR)/$(PACKAGE_NAME)
DEB_DIR = $(PACKAGE_DIR)/DEBIAN

# Installation paths
INSTALL_PREFIX = /usr/local
BIN_DIR = $(INSTALL_PREFIX)/bin
LIB_DIR = $(INSTALL_PREFIX)/lib/$(PROJECT_NAME)
CONFIG_DIR = /etc/$(PROJECT_NAME)
SERVICE_DIR = /etc/systemd/system
WEB_DIR = $(LIB_DIR)/web

# Go build flags
GO_BUILD_FLAGS = -ldflags="-w -s" -trimpath

# Binary targets
BINARIES = cmd/home-automation-server/home-automation-server \
           cmd/command_runner_server/command_runner_server \
           cmd/mqtt_listener/mqtt_listener \
           cmd/serial_expect/serial_expect

.PHONY: all build clean test install deb help deps

all: build

# Build all binaries
build: $(BINARIES)

cmd/home-automation-server/home-automation-server:
	@echo "Building home-automation-server..."
	cd cmd/home-automation-server && go build $(GO_BUILD_FLAGS) -o home-automation-server .

cmd/command_runner_server/command_runner_server:
	@echo "Building command_runner_server..."
	cd cmd/command_runner_server && go build $(GO_BUILD_FLAGS) -o command_runner_server main.go

cmd/mqtt_listener/mqtt_listener:
	@echo "Building mqtt_listener..."
	cd cmd/mqtt_listener && go build $(GO_BUILD_FLAGS) -o mqtt_listener main.go

cmd/serial_expect/serial_expect:
	@echo "Building serial_expect..."
	cd cmd/serial_expect && go build $(GO_BUILD_FLAGS) -o serial_expect main.go

# Install dependencies
deps:
	@echo "Installing Go dependencies..."
	go mod download
	go mod verify

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BINARIES)
	rm -rf $(BUILD_DIR)
	find . -name "*.test" -delete
	find . -name "*.out" -delete

# Create Debian package
deb: build
	@echo "Creating Debian package..."
	@mkdir -p $(DEB_DIR)
	@mkdir -p $(PACKAGE_DIR)$(BIN_DIR)
	@mkdir -p $(PACKAGE_DIR)$(LIB_DIR)
	@mkdir -p $(PACKAGE_DIR)$(CONFIG_DIR)
	@mkdir -p $(PACKAGE_DIR)$(SERVICE_DIR)
	@mkdir -p $(PACKAGE_DIR)$(WEB_DIR)
	
	# Copy binaries
	@echo "Copying binaries..."
	cp cmd/home-automation-server/home-automation-server $(PACKAGE_DIR)$(BIN_DIR)/
	cp cmd/command_runner_server/command_runner_server $(PACKAGE_DIR)$(BIN_DIR)/
	cp cmd/mqtt_listener/mqtt_listener $(PACKAGE_DIR)$(BIN_DIR)/
	cp cmd/serial_expect/serial_expect $(PACKAGE_DIR)$(BIN_DIR)/
	
	# Copy web files
	@echo "Copying web files..."
	cp -r web/* $(PACKAGE_DIR)$(WEB_DIR)/
	cp -r cmd/command_runner_server/web/* $(PACKAGE_DIR)$(LIB_DIR)/command_runner_web/
	
	# Copy configuration files
	@echo "Copying configuration files..."
	cp configs/*.xml $(PACKAGE_DIR)$(CONFIG_DIR)/
	
	# Copy systemd service files
	@echo "Copying service files..."
	cp debian/mqtt-home-automation.service $(PACKAGE_DIR)$(SERVICE_DIR)/
	cp debian/command-runner.service $(PACKAGE_DIR)$(SERVICE_DIR)/
	
	# Generate DEBIAN control files
	@echo "Generating Debian control files..."
	./scripts/generate-debian-files.sh $(VERSION) $(ARCH) $(PACKAGE_DIR)
	
	# Build the package
	@echo "Building .deb package..."
	dpkg-deb --build $(PACKAGE_DIR)
	@echo "Package created: $(BUILD_DIR)/$(PACKAGE_NAME).deb"

# Install locally (for development)
install: build
	@echo "Installing binaries to $(DESTDIR)$(BIN_DIR)..."
	@mkdir -p $(DESTDIR)$(BIN_DIR)
	@mkdir -p $(DESTDIR)$(LIB_DIR)
	@mkdir -p $(DESTDIR)$(CONFIG_DIR)
	
	cp cmd/home-automation-server/home-automation-server $(DESTDIR)$(BIN_DIR)/
	cp cmd/command_runner_server/command_runner_server $(DESTDIR)$(BIN_DIR)/
	cp cmd/mqtt_listener/mqtt_listener $(DESTDIR)$(BIN_DIR)/
	cp cmd/serial_expect/serial_expect $(DESTDIR)$(BIN_DIR)/
	
	cp -r web $(DESTDIR)$(LIB_DIR)/
	cp configs/*.xml $(DESTDIR)$(CONFIG_DIR)/
	
	@echo "Installation complete!"

# Development server (runs home automation server)
dev-server: build
	cd cmd/home-automation-server && ./home-automation-server -config ../../configs/config.xml -webdir ../../web

# Development command runner
dev-command-runner: build
	cd cmd/command_runner_server && ./command_runner_server -config config.xml

# Show help
help:
	@echo "MQTT Home Automation Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  all              - Build all binaries (default)"
	@echo "  build            - Build all binaries"
	@echo "  deps             - Install Go dependencies"
	@echo "  test             - Run tests"
	@echo "  clean            - Remove build artifacts"
	@echo "  deb              - Create Debian package"
	@echo "  install          - Install locally (use DESTDIR= for custom prefix)"
	@echo "  dev-server       - Run development home automation server"
	@echo "  dev-command-runner - Run development command runner server"
	@echo "  help             - Show this help message"
	@echo ""
	@echo "Package info:"
	@echo "  Name:    $(PROJECT_NAME)"
	@echo "  Version: $(VERSION)"
	@echo "  Arch:    $(ARCH)"
	@echo "  Package: $(PACKAGE_NAME).deb"
