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
