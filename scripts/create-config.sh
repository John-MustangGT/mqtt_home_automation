#!/bin/bash
# Create command_runner.xml configuration file

if [ $# -ne 1 ]; then
    echo "Usage: $0 OUTPUT_FILE"
    exit 1
fi

OUTPUT_FILE="$1"

cat > "$OUTPUT_FILE" << 'XMLEOF'
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
            <n>system_info</n>
            <display_name>📊 System Info</display_name>
            <command>uname -a</command>
            <size>md</size>
            <color>primary</color>
        </button>
        
        <button>
            <n>disk_usage</n>
            <display_name>💾 Disk Usage</display_name>
            <command>df -h</command>
            <size>md</size>
            <color>info</color>
        </button>
        
        <button>
            <n>memory_usage</n>
            <display_name>🧠 Memory Usage</display_name>
            <command>free -h</command>
            <size>md</size>
            <color>success</color>
        </button>
        
        <button>
            <n>uptime</n>
            <display_name>⏰ Server Uptime</display_name>
            <command>uptime</command>
            <size>sm</size>
            <color>dark</color>
        </button>
    </buttons>
</config>
XMLEOF

echo "Configuration file created: $OUTPUT_FILE"
