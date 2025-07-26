class HomeAutomation {
    constructor() {
        this.ws = null;
        this.loadChart = null;
        this.loadData = {
            labels: [],
            load1: [],
            load5: [],
            load15: []
        };
        this.maxDataPoints = 20;
        this.automationStatus = {};
        this.deviceHealth = {};
        this.init();
    }

    init() {
        this.connectWebSocket();
        this.setupToasts();
        this.setupLoadChart();
        this.startSystemStatsPolling();
        this.loadInitialMqttLog();
        this.loadAutomationStatus();
        this.loadDeviceHealth();
    }

    async loadInitialMqttLog() {
        try {
            const response = await fetch('/api/mqtt-log');
            const logEntries = await response.json();
            
            const logContainer = document.getElementById('mqtt-log');
            if (logContainer) {
                logContainer.innerHTML = '';
                
                logEntries.forEach(entry => {
                    this.addMqttLogEntry(entry, false);
                });
                
                if (logEntries.length === 0) {
                    logContainer.innerHTML = '<div class="text-muted">MQTT messages will appear here...</div>';
                }
            }
        } catch (error) {
            console.error('Failed to load initial MQTT log:', error);
        }
    }

    async loadAutomationStatus() {
        try {
            const response = await fetch('/api/automations');
            this.automationStatus = await response.json();
            this.updateAutomationDisplay();
        } catch (error) {
            console.error('Failed to load automation status:', error);
        }
    }

    async loadDeviceHealth() {
        try {
            const response = await fetch('/api/device-health');
            this.deviceHealth = await response.json();
            this.updateHealthDisplay();
        } catch (error) {
            console.error('Failed to load device health:', error);
        }
    }

    setupLoadChart() {
        const ctx = document.getElementById('loadChart').getContext('2d');
        this.loadChart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: this.loadData.labels,
                datasets: [{
                    label: '1 min',
                    data: this.loadData.load1,
                    borderColor: 'rgb(255, 99, 132)',
                    backgroundColor: 'rgba(255, 99, 132, 0.1)',
                    tension: 0.4
                }, {
                    label: '5 min',
                    data: this.loadData.load5,
                    borderColor: 'rgb(54, 162, 235)',
                    backgroundColor: 'rgba(54, 162, 235, 0.1)',
                    tension: 0.4
                }, {
                    label: '15 min',
                    data: this.loadData.load15,
                    borderColor: 'rgb(75, 192, 192)',
                    backgroundColor: 'rgba(75, 192, 192, 0.1)',
                    tension: 0.4
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    y: {
                        beginAtZero: true,
                        title: {
                            display: true,
                            text: 'Load Average'
                        }
                    },
                    x: {
                        title: {
                            display: true,
                            text: 'Time'
                        }
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: 'System Load Average'
                    },
                    legend: {
                        position: 'top'
                    }
                }
            }
        });
    }

    startSystemStatsPolling() {
        this.updateSystemStats();
        setInterval(() => this.updateSystemStats(), 30000);
    }

    async updateSystemStats() {
        try {
            const response = await fetch('/api/system-stats');
            const stats = await response.json();
            
            document.getElementById('uptime-text').textContent = stats.uptime || 'Unknown';
            
            const memoryPercent = stats.memoryTotal > 0 ? 
                ((stats.memoryUsed / stats.memoryTotal) * 100).toFixed(1) : 0;
            document.getElementById('memory-text').textContent = `${memoryPercent}%`;
            document.getElementById('memory-bar').style.width = `${memoryPercent}%`;
            
            const now = new Date().toLocaleTimeString();
            this.loadData.labels.push(now);
            this.loadData.load1.push(stats.loadAvg1 || 0);
            this.loadData.load5.push(stats.loadAvg5 || 0);
            this.loadData.load15.push(stats.loadAvg15 || 0);
            
            if (this.loadData.labels.length > this.maxDataPoints) {
                this.loadData.labels.shift();
                this.loadData.load1.shift();
                this.loadData.load5.shift();
                this.loadData.load15.shift();
            }
            
            this.loadChart.update();
            
        } catch (error) {
            console.error('Failed to fetch system stats:', error);
            this.showToast('Failed to update system stats', 'warning');
        }
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.ws = new WebSocket(protocol + '//' + window.location.host + '/ws');
        
        this.ws.onopen = () => {
            this.showToast('Connected to server', 'success');
            this.updateConnectionStatus('online');
        };

        this.ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            switch (message.type) {
                case 'status_update':
                    this.updateDeviceStatus(message.deviceId, message.data);
                    break;
                case 'health_update':
                    this.updateDeviceHealth(message.deviceId, message.data);
                    break;
                case 'mqtt_log':
                    this.addMqttLogEntry(message.data);
                    break;
                case 'automation_update':
                    this.updateAutomationStatus(message.data);
                    break;
            }
        };

        this.ws.onclose = () => {
            this.showToast('Connection lost. Reconnecting...', 'warning');
            this.updateConnectionStatus('reconnecting');
            setTimeout(() => this.connectWebSocket(), 3000);
        };

        this.ws.onerror = (error) => {
            this.showToast('Connection error', 'danger');
            this.updateConnectionStatus('error');
        };
    }

    updateConnectionStatus(status) {
        const statusElement = document.getElementById('system-status');
        if (!statusElement) return;

        let icon, text, colorClass;
        switch (status) {
            case 'online':
                icon = 'bi-circle-fill text-success';
                text = 'System Online';
                break;
            case 'reconnecting':
                icon = 'bi-circle-fill text-warning';
                text = 'Reconnecting...';
                break;
            case 'error':
                icon = 'bi-circle-fill text-danger';
                text = 'Connection Error';
                break;
        }

        statusElement.innerHTML = `<i class="bi ${icon}"></i> ${text}`;
    }

    updateDeviceStatus(deviceId, status) {
        const statusElements = document.querySelectorAll(`[id^="status-${deviceId}"]`);
        
        let statusText = 'Online';
        let badgeClass = 'bg-success';

        if (status.value !== undefined) {
            statusText += ' - ' + status.value;
        }

        if (status.lastUpdate) {
            const time = new Date(status.lastUpdate).toLocaleTimeString();
            statusText += ' (' + time + ')';
        }

        const statusHtml = `<span class="badge ${badgeClass}"><i class="bi bi-circle-fill text-success"></i> ${statusText}</span>`;
        
        statusElements.forEach(element => {
            element.innerHTML = statusHtml;
        });
    }

    updateDeviceHealth(deviceId, healthData) {
        const healthElements = document.querySelectorAll(`[id^="health-${deviceId}"]`);
        const status = healthData.status;
        
        let iconClass, colorClass;
        switch (status) {
            case 'online':
                iconClass = 'bi-heart-fill';
                colorClass = 'text-success';
                break;
            case 'offline':
                iconClass = 'bi-heart';
                colorClass = 'text-danger';
                break;
            default:
                iconClass = 'bi-question-circle';
                colorClass = 'text-warning';
        }

        healthElements.forEach(element => {
            element.innerHTML = `<i class="bi ${iconClass} ${colorClass}" title="Device ${status}"></i>`;
        });

        // Update global health status
        this.deviceHealth.devices = this.deviceHealth.devices || {};
        this.deviceHealth.devices[deviceId] = { healthStatus: status };
        this.updateHealthDisplay();
    }

    updateHealthDisplay() {
        const healthSummary = document.getElementById('health-summary');
        if (!healthSummary || !this.deviceHealth.devices) return;

        let online = 0, offline = 0, unknown = 0;
        
        Object.values(this.deviceHealth.devices).forEach(device => {
            switch (device.healthStatus) {
                case 'online': online++; break;
                case 'offline': offline++; break;
                default: unknown++; break;
            }
        });

        healthSummary.innerHTML = `
            <div class="row text-center">
                <div class="col">
                    <div class="text-success h4">${online}</div>
                    <div class="small">Online</div>
                </div>
                <div class="col">
                    <div class="text-danger h4">${offline}</div>
                    <div class="small">Offline</div>
                </div>
                <div class="col">
                    <div class="text-warning h4">${unknown}</div>
                    <div class="small">Unknown</div>
                </div>
            </div>
        `;
    }

    updateAutomationDisplay() {
        const automationList = document.getElementById('automation-list');
        if (!automationList) return;

        let html = '';
        Object.entries(this.automationStatus).forEach(([id, automation]) => {
            const nextRun = new Date(automation.nextRun).toLocaleString();
            const statusBadge = automation.enabled ? 
                '<span class="badge bg-success">Enabled</span>' : 
                '<span class="badge bg-secondary">Disabled</span>';
            
            const runningBadge = automation.running ? 
                '<span class="badge bg-info ms-1">Running</span>' : '';

            html += `
                <div class="card mb-2">
                    <div class="card-body py-2">
                        <div class="d-flex justify-content-between align-items-center">
                            <div>
                                <strong>${automation.name}</strong>
                                <div class="small text-muted">Next: ${nextRun}</div>
                            </div>
                            <div>
                                ${statusBadge}${runningBadge}
                                <div class="btn-group btn-group-sm ms-2">
                                    <button class="btn btn-outline-primary" onclick="toggleAutomation('${id}')">
                                        ${automation.enabled ? 'Disable' : 'Enable'}
                                    </button>
                                    <button class="btn btn-outline-success" onclick="triggerAutomation('${id}')">
                                        Trigger
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            `;
        });

        automationList.innerHTML = html || '<div class="text-muted">No automations configured</div>';
    }

    setupToasts() {
        if (!document.getElementById('toast-container')) {
            const container = document.createElement('div');
            container.id = 'toast-container';
            container.className = 'position-fixed bottom-0 end-0 p-3';
            container.style.zIndex = '1050';
            document.body.appendChild(container);
        }
    }

    showToast(message, type = 'info') {
        const toastContainer = document.getElementById('toast-container');
        const toastId = 'toast-' + Date.now();
        
        const bgClass = {
            'success': 'bg-success',
            'danger': 'bg-danger', 
            'warning': 'bg-warning',
            'info': 'bg-info'
        }[type] || 'bg-info';

        const toastHtml = `
            <div class="toast ${bgClass} text-white" id="${toastId}" role="alert">
                <div class="toast-body">
                    ${message}
                    <button type="button" class="btn-close btn-close-white float-end" data-bs-dismiss="toast"></button>
                </div>
            </div>
        `;

        toastContainer.insertAdjacentHTML('beforeend', toastHtml);
        
        const toastElement = document.getElementById(toastId);
        const toast = new bootstrap.Toast(toastElement, { delay: 3000 });
        toast.show();

        toastElement.addEventListener('hidden.bs.toast', () => {
            toastElement.remove();
        });
    }

    addMqttLogEntry(logEntry, scrollToTop = true) {
        const logContainer = document.getElementById('mqtt-log');
        if (!logContainer) return;

        const placeholder = logContainer.querySelector('.text-muted');
        if (placeholder) {
            placeholder.remove();
        }

        const logLine = document.createElement('div');
        logLine.className = 'mqtt-log-entry mb-1';
        
        const isOutgoing = logEntry.topic.includes('(OUT)');
        const isAutomation = logEntry.topic.includes('(AUTO)');
        const isHealth = logEntry.topic.includes('(HEALTH)');
        
        let topicClass = 'text-warning';
        let direction = '←';
        let topicText = logEntry.topic;
        
        if (isOutgoing) {
            topicClass = 'text-info';
            direction = '→';
            topicText = logEntry.topic.replace(' (OUT)', '');
        } else if (isAutomation) {
            topicClass = 'text-success';
            direction = '⚡';
            topicText = logEntry.topic.replace(' (AUTO)', '');
        } else if (isHealth) {
            topicClass = 'text-primary';
            direction = '♥';
            topicText = logEntry.topic.replace(' (HEALTH)', '');
        }
        
        logLine.innerHTML = `
            <span class="text-secondary">[${logEntry.timestamp}]</span>
            <span class="text-muted">${direction}</span>
            <span class="${topicClass}">${topicText}</span>
            <span class="text-light">: ${this.formatPayload(logEntry.payload)}</span>
        `;

        if (scrollToTop) {
            logContainer.insertBefore(logLine, logContainer.firstChild);
        } else {
            logContainer.appendChild(logLine);
        }

        const entries = logContainer.querySelectorAll('.mqtt-log-entry');
        if (entries.length > 50) {
            for (let i = 50; i < entries.length; i++) {
                entries[i].remove();
            }
        }

        if (scrollToTop) {
            logContainer.scrollTop = 0;
        }
    }

    formatPayload(payload) {
        try {
            const parsed = JSON.parse(payload);
            return JSON.stringify(parsed, null, 0);
        } catch (e) {
            return payload;
        }
    }
}

// Global functions for button clicks
async function sendCommand(deviceId, topic, payload, localCommand, controlType = '') {
    try {
        const response = await fetch('/api/control', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                device: deviceId,
                topic: topic,
                payload: payload,
                localCommand: localCommand,
                controlType: controlType
            })
        });

        const result = await response.json();

        if (response.ok) {
            if (topic) {
                app.showToast(`MQTT command sent to ${topic}`, 'success');
                console.log(`MQTT command sent - Topic: ${topic}, Payload: ${payload}`);
            }
            if (localCommand) {
                app.showToast(`Local command executed: ${localCommand}`, 'success');
                console.log(`Local command executed: ${localCommand}`);
            }
        } else {
            throw new Error(result.error || `HTTP ${response.status}`);
        }
    } catch (error) {
        console.error('Failed to send command:', error);
        app.showToast(`Failed to send command: ${error.message}`, 'danger');
    }
}

async function sendSliderCommand(deviceId, topic, label, value) {
    const payload = JSON.stringify({
        [label.toLowerCase()]: parseInt(value)
    });
    
    await sendCommand(deviceId, topic, payload, '', 'slider');
}

function updateSliderValue(deviceId, label, value, context = '') {
    const suffix = context ? `-${context}` : '';
    const element = document.getElementById(`slider-${deviceId}-${label}${suffix}`);
    if (element) {
        element.textContent = value;
    }
    
    if (!context) {
        updateSliderValue(deviceId, label, value, 'all');
        
        const allElements = document.querySelectorAll(`[id^="slider-${deviceId}-${label}-"]`);
        allElements.forEach(el => {
            el.textContent = value;
        });
    }
}

async function toggleCommand(deviceId, topic, payload, localCommand, button) {
    const isActive = button.classList.contains('active');
    
    const allToggleButtons = document.querySelectorAll(`[id^="toggle-${deviceId}-${button.id.split('-')[2]}"]`);
    
    allToggleButtons.forEach(btn => {
        if (isActive) {
            btn.classList.remove('active');
            btn.innerHTML = '<i class="bi bi-toggle-off"></i> ' + btn.textContent.replace(/.*/, btn.textContent.trim());
        } else {
            btn.classList.add('active');
            btn.innerHTML = '<i class="bi bi-toggle-on"></i> ' + btn.textContent.replace(/.*/, btn.textContent.trim());
        }
    });
    
    await sendCommand(deviceId, topic, payload, localCommand, 'toggle');
}

async function toggleAutomation(automationId) {
    try {
        const automation = app.automationStatus[automationId];
        const action = automation.enabled ? 'disable' : 'enable';
        
        const response = await fetch('/api/automations', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                automationId: automationId,
                action: action
            })
        });

        if (response.ok) {
            app.showToast(`Automation ${action}d successfully`, 'success');
            app.loadAutomationStatus(); // Refresh status
        } else {
            throw new Error(`HTTP ${response.status}`);
        }
    } catch (error) {
        console.error('Failed to toggle automation:', error);
        app.showToast('Failed to toggle automation', 'danger');
    }
}

async function triggerAutomation(automationId) {
    try {
        const response = await fetch('/api/automations', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                automationId: automationId,
                action: 'trigger'
            })
        });

        if (response.ok) {
            app.showToast('Automation triggered successfully', 'success');
        } else {
            throw new Error(`HTTP ${response.status}`);
        }
    } catch (error) {
        console.error('Failed to trigger automation:', error);
        app.showToast('Failed to trigger automation', 'danger');
    }
}

function clearMqttLog() {
    const logContainer = document.getElementById('mqtt-log');
    if (logContainer) {
        logContainer.innerHTML = '<div class="text-muted">MQTT messages will appear here...</div>';
        app.showToast('MQTT log cleared', 'info');
    }
}

// Initialize the application
let app;
document.addEventListener('DOMContentLoaded', function() {
    app = new HomeAutomation();
    
    // Refresh automation status every 30 seconds
    setInterval(() => {
        if (app.loadAutomationStatus) {
            app.loadAutomationStatus();
        }
    }, 30000);
    
    // Refresh device health every 60 seconds
    setInterval(() => {
        if (app.loadDeviceHealth) {
            app.loadDeviceHealth();
        }
    }, 60000);
});