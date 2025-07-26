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
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const logEntries = await response.json();
            
            const logContainer = document.getElementById('mqtt-log');
            if (logContainer) {
                logContainer.innerHTML = '';
                
                if (Array.isArray(logEntries)) {
                    logEntries.forEach(entry => {
                        this.addMqttLogEntry(entry, false);
                    });
                    
                    if (logEntries.length === 0) {
                        logContainer.innerHTML = '<div class="text-muted">MQTT messages will appear here...</div>';
                    }
                } else {
                    console.warn('Expected array for MQTT log entries, got:', typeof logEntries);
                    logContainer.innerHTML = '<div class="text-muted">MQTT messages will appear here...</div>';
                }
            }
        } catch (error) {
            console.error('Failed to load initial MQTT log:', error);
            this.showToast(`Failed to load MQTT log: ${error.message}`, 'warning');
        }
    }

    async loadAutomationStatus() {
        try {
            const response = await fetch('/api/automations');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            this.automationStatus = await response.json();
            this.updateAutomationDisplay();
        } catch (error) {
            console.error('Failed to load automation status:', error);
            this.showToast(`Failed to load automation status: ${error.message}`, 'warning');
        }
    }

    async loadDeviceHealth() {
        try {
            const response = await fetch('/api/device-health');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            this.deviceHealth = await response.json();
            this.updateHealthDisplay();
        } catch (error) {
            console.error('Failed to load device health:', error);
            this.showToast(`Failed to load device health: ${error.message}`, 'warning');
        }
    }

    setupLoadChart() {
        const chartElement = document.getElementById('loadChart');
        if (!chartElement) {
            console.warn('Load chart element not found');
            return;
        }
        
        const ctx = chartElement.getContext('2d');
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
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const stats = await response.json();
            
            // Update uptime
            const uptimeElement = document.getElementById('uptime-text');
            if (uptimeElement) {
                uptimeElement.textContent = stats.uptime || 'Unknown';
            }
            
            // Update memory usage
            const memoryPercent = stats.memoryTotal > 0 ? 
                ((stats.memoryUsed / stats.memoryTotal) * 100).toFixed(1) : 0;
            
            const memoryTextElement = document.getElementById('memory-text');
            const memoryBarElement = document.getElementById('memory-bar');
            
            if (memoryTextElement) {
                memoryTextElement.textContent = `${memoryPercent}%`;
            }
            if (memoryBarElement) {
                memoryBarElement.style.width = `${memoryPercent}%`;
            }
            
            // Update load chart
            if (this.loadChart) {
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
                
                this.loadChart.update('none');
            }
            
        } catch (error) {
            console.error('Failed to fetch system stats:', error);
            this.showToast(`Failed to update system stats: ${error.message}`, 'warning');
        }
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        console.log('Connecting to WebSocket:', wsUrl);
        this.ws = new WebSocket(wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected successfully');
            this.showToast('Connected to server', 'success');
            this.updateConnectionStatus('online');
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.handleWebSocketMessage(message);
            } catch (error) {
                console.error('Failed to parse WebSocket message:', error, 'Raw data:', event.data);
            }
        };

        this.ws.onclose = (event) => {
            console.log('WebSocket closed:', event.code, event.reason);
            this.showToast('Connection lost. Reconnecting...', 'warning');
            this.updateConnectionStatus('reconnecting');
            
            // Reconnect after delay
            setTimeout(() => {
                if (this.ws.readyState === WebSocket.CLOSED) {
                    this.connectWebSocket();
                }
            }, 3000);
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.showToast('Connection error', 'danger');
            this.updateConnectionStatus('error');
        };
    }

    handleWebSocketMessage(message) {
        if (!message || typeof message !== 'object') {
            console.warn('Invalid WebSocket message format:', message);
            return;
        }

        switch (message.type) {
            case 'status_update':
                if (message.deviceId && message.data) {
                    this.updateDeviceStatus(message.deviceId, message.data);
                } else {
                    console.warn('Invalid status_update message:', message);
                }
                break;
            case 'health_update':
                if (message.deviceId && message.data) {
                    this.updateDeviceHealth(message.deviceId, message.data);
                } else {
                    console.warn('Invalid health_update message:', message);
                }
                break;
            case 'mqtt_log':
                if (message.data) {
                    this.addMqttLogEntry(message.data);
                } else {
                    console.warn('Invalid mqtt_log message:', message);
                }
                break;
            case 'automation_update':
                if (message.data) {
                    this.updateAutomationStatus(message.data);
                } else {
                    console.warn('Invalid automation_update message:', message);
                }
                break;
            default:
                console.warn('Unknown WebSocket message type:', message.type);
        }
    }

    updateConnectionStatus(status) {
        const statusElement = document.getElementById('system-status');
        if (!statusElement) return;

        let icon, text;
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
            default:
                icon = 'bi-circle-fill text-secondary';
                text = 'Unknown Status';
        }

        statusElement.innerHTML = `<i class="bi ${icon}"></i> ${text}`;
    }

    updateDeviceStatus(deviceId, status) {
        if (!deviceId || !status) {
            console.warn('Invalid device status update:', deviceId, status);
            return;
        }

        const statusElements = document.querySelectorAll(`[id^="status-${deviceId}"]`);
        
        let statusText = 'Online';
        let badgeClass = 'bg-success';

        if (status.value !== undefined) {
            statusText += ' - ' + String(status.value);
        }

        if (status.lastUpdate) {
            try {
                const time = new Date(status.lastUpdate).toLocaleTimeString();
                statusText += ' (' + time + ')';
            } catch (error) {
                console.warn('Invalid date format in status update:', status.lastUpdate);
            }
        }

        const statusHtml = `<span class="badge ${badgeClass}"><i class="bi bi-circle-fill text-success"></i> ${statusText}</span>`;
        
        statusElements.forEach(element => {
            element.innerHTML = statusHtml;
        });
    }

    updateDeviceHealth(deviceId, healthData) {
        if (!deviceId || !healthData) {
            console.warn('Invalid device health update:', deviceId, healthData);
            return;
        }

        const healthElements = document.querySelectorAll(`[id^="health-${deviceId}"]`);
        const status = healthData.status;
        
        let iconClass, colorClass, title;
        switch (status) {
            case 'online':
                iconClass = 'bi-heart-fill';
                colorClass = 'text-success';
                title = 'Device online';
                break;
            case 'offline':
                iconClass = 'bi-heart';
                colorClass = 'text-danger';
                title = 'Device offline';
                break;
            default:
                iconClass = 'bi-question-circle';
                colorClass = 'text-warning';
                title = 'Device status unknown';
        }

        healthElements.forEach(element => {
            element.innerHTML = `<i class="bi ${iconClass} ${colorClass}" title="${title}"></i>`;
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

        if (!this.automationStatus || Object.keys(this.automationStatus).length === 0) {
            automationList.innerHTML = '<div class="text-muted">No automations configured</div>';
            return;
        }

        let html = '';
        Object.entries(this.automationStatus).forEach(([id, automation]) => {
            try {
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
                                    <strong>${this.escapeHtml(automation.name || 'Unnamed')}</strong>
                                    <div class="small text-muted">Next: ${nextRun}</div>
                                </div>
                                <div>
                                    ${statusBadge}${runningBadge}
                                    <div class="btn-group btn-group-sm ms-2">
                                        <button class="btn btn-outline-primary" onclick="toggleAutomation('${this.escapeHtml(id)}')">
                                            ${automation.enabled ? 'Disable' : 'Enable'}
                                        </button>
                                        <button class="btn btn-outline-success" onclick="triggerAutomation('${this.escapeHtml(id)}')">
                                            Trigger
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                `;
            } catch (error) {
                console.error('Error processing automation:', id, error);
                html += `
                    <div class="card mb-2 border-danger">
                        <div class="card-body py-2">
                            <div class="text-danger">Error displaying automation: ${this.escapeHtml(id)}</div>
                        </div>
                    </div>
                `;
            }
        });

        automationList.innerHTML = html;
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
        if (!toastContainer) {
            console.error('Toast container not found');
            return;
        }
        
        const toastId = 'toast-' + Date.now();
        
        const bgClass = {
            'success': 'bg-success',
            'danger': 'bg-danger', 
            'warning': 'bg-warning',
            'info': 'bg-info'
        }[type] || 'bg-info';

        const safeMessage = this.escapeHtml(String(message));

        const toastHtml = `
            <div class="toast ${bgClass} text-white" id="${toastId}" role="alert">
                <div class="toast-body">
                    ${safeMessage}
                    <button type="button" class="btn-close btn-close-white float-end" data-bs-dismiss="toast"></button>
                </div>
            </div>
        `;

        toastContainer.insertAdjacentHTML('beforeend', toastHtml);
        
        const toastElement = document.getElementById(toastId);
        if (toastElement && typeof bootstrap !== 'undefined') {
            const toast = new bootstrap.Toast(toastElement, { delay: 3000 });
            toast.show();

            toastElement.addEventListener('hidden.bs.toast', () => {
                toastElement.remove();
            });
        } else {
            console.warn('Bootstrap not available or toast element not found');
            setTimeout(() => {
                const element = document.getElementById(toastId);
                if (element) element.remove();
            }, 3000);
        }
    }

    addMqttLogEntry(logEntry, scrollToTop = true) {
        const logContainer = document.getElementById('mqtt-log');
        if (!logContainer) return;

        if (!logEntry || typeof logEntry !== 'object') {
            console.warn('Invalid MQTT log entry:', logEntry);
            return;
        }

        const placeholder = logContainer.querySelector('.text-muted');
        if (placeholder) {
            placeholder.remove();
        }

        const logLine = document.createElement('div');
        logLine.className = 'mqtt-log-entry mb-1';
        
        const topic = String(logEntry.topic || 'unknown');
        const payload = String(logEntry.payload || '');
        const timestamp = String(logEntry.timestamp || new Date().toLocaleTimeString());
        
        const isOutgoing = topic.includes('(OUT)');
        const isAutomation = topic.includes('(AUTO)');
        const isHealth = topic.includes('(HEALTH)');
        
        let topicClass = 'text-warning';
        let direction = '←';
        let topicText = topic;
        
        if (isOutgoing) {
            topicClass = 'text-info';
            direction = '→';
            topicText = topic.replace(' (OUT)', '');
        } else if (isAutomation) {
            topicClass = 'text-success';
            direction = '⚡';
            topicText = topic.replace(' (AUTO)', '');
        } else if (isHealth) {
            topicClass = 'text-primary';
            direction = '♥';
            topicText = topic.replace(' (HEALTH)', '');
        }
        
        logLine.innerHTML = `
            <span class="text-secondary">[${this.escapeHtml(timestamp)}]</span>
            <span class="text-muted">${direction}</span>
            <span class="${topicClass}">${this.escapeHtml(topicText)}</span>
            <span class="text-light">: ${this.escapeHtml(this.formatPayload(payload))}</span>
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

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}

// Functions that read from data attributes to avoid template variable issues
function sendCommandFromButton(button, controlType) {
    console.log('=== SEND COMMAND FROM BUTTON ===');
    console.log('Button element:', button);
    
    const deviceId = button.getAttribute('data-device-id');
    const topic = button.getAttribute('data-topic') || '';
    const payload = button.getAttribute('data-payload') || '';
    const localCommand = button.getAttribute('data-local-command') || '';
    
    console.log('Data from button attributes:', {
        deviceId, topic, payload, localCommand, controlType
    });
    
    if (!deviceId) {
        console.error('No device ID found in button attributes');
        app.showToast('Automation triggered successfully', 'success');
    } catch (error) {
        console.error('Failed to trigger automation:', error);
        app.showToast(`Failed to trigger automation: ${error.message}`, 'danger');
    }
}

function clearMqttLog() {
    const logContainer = document.getElementById('mqtt-log');
    if (logContainer) {
        logContainer.innerHTML = '<div class="text-muted">MQTT messages will appear here...</div>';
        app.showToast('MQTT log cleared', 'info');
    }
}

// Debug function to analyze all buttons on page load
function debugAllButtons() {
    console.log('=== BUTTON ANALYSIS ===');
    
    const allButtons = document.querySelectorAll('button[onclick*="sendCommand"], button[onclick*="toggleCommand"], button[onclick*="sendCommandFromButton"], button[onclick*="toggleCommandFromButton"]');
    console.log(`Found ${allButtons.length} command buttons`);
    
    allButtons.forEach((button, index) => {
        console.log(`Button ${index + 1}:`);
        console.log('  Element:', button);
        console.log('  onclick:', button.getAttribute('onclick'));
        console.log('  data-device-id:', button.getAttribute('data-device-id'));
        console.log('  data-topic:', button.getAttribute('data-topic'));
        console.log('  data-payload:', button.getAttribute('data-payload'));
        console.log('  data-local-command:', button.getAttribute('data-local-command'));
        console.log('---');
    });
}

// Initialize the application
let app;
document.addEventListener('DOMContentLoaded', function() {
    console.log('DOM loaded, initializing HomeAutomation...');
    
    try {
        app = new HomeAutomation();
        console.log('HomeAutomation initialized successfully');
        
        // Run debugging analysis
        setTimeout(() => {
            debugAllButtons();
        }, 1000);
        
        // Refresh automation status every 30 seconds
        setInterval(() => {
            if (app && typeof app.loadAutomationStatus === 'function') {
                app.loadAutomationStatus();
            }
        }, 30000);
        
        // Refresh device health every 60 seconds
        setInterval(() => {
            if (app && typeof app.loadDeviceHealth === 'function') {
                app.loadDeviceHealth();
            }
        }, 60000);
    } catch (error) {
        console.error('Failed to initialize HomeAutomation:', error);
    }
});Toast('No device ID found', 'danger');
        return;
    }
    
    return sendCommand(deviceId, topic, payload, localCommand, controlType);
}

function sendSliderCommandFromElement(slider) {
    console.log('=== SEND SLIDER COMMAND FROM ELEMENT ===');
    console.log('Slider element:', slider);
    
    const deviceId = slider.getAttribute('data-device-id');
    const topic = slider.getAttribute('data-topic') || '';
    const label = slider.getAttribute('data-label') || '';
    const value = slider.value;
    
    console.log('Data from slider attributes:', {
        deviceId, topic, label, value
    });
    
    if (!deviceId) {
        console.error('No device ID found in slider attributes');
        app.showToast('No device ID found', 'danger');
        return;
    }
    
    return sendSliderCommand(deviceId, topic, label, value);
}

function toggleCommandFromButton(button) {
    console.log('=== TOGGLE COMMAND FROM BUTTON ===');
    console.log('Button element:', button);
    
    const deviceId = button.getAttribute('data-device-id');
    const topic = button.getAttribute('data-topic') || '';
    const payload = button.getAttribute('data-payload') || '';
    const localCommand = button.getAttribute('data-local-command') || '';
    
    console.log('Data from button attributes:', {
        deviceId, topic, payload, localCommand
    });
    
    if (!deviceId) {
        console.error('No device ID found in button attributes');
        app.showToast('No device ID found', 'danger');
        return;
    }
    
    return toggleCommand(deviceId, topic, payload, localCommand, button);
}

// Core command functions
async function sendCommand(deviceId, topic, payload, localCommand, controlType = '') {
    console.log('=== SEND COMMAND ===');
    console.log('Parameters:', { deviceId, topic, payload, localCommand, controlType });
    
    if (!deviceId) {
        console.error('Device ID is required');
        app.showToast('Device ID is required', 'danger');
        return;
    }

    try {
        const requestBody = {
            device: String(deviceId),
            topic: String(topic || ''),
            payload: String(payload || ''),
            localCommand: String(localCommand || ''),
            controlType: String(controlType || '')
        };

        console.log('Sending request:', requestBody);

        const response = await fetch('/api/control', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(requestBody)
        });

        let result;
        const contentType = response.headers.get('content-type');
        
        if (contentType && contentType.includes('application/json')) {
            result = await response.json();
        } else {
            const text = await response.text();
            console.error('Non-JSON response:', text);
            throw new Error(`Server returned non-JSON response: ${response.status} ${response.statusText}`);
        }

        if (response.ok) {
            console.log('✅ Command sent successfully:', result);
            
            if (topic) {
                app.showToast(`MQTT command sent to ${topic}`, 'success');
            }
            if (localCommand) {
                app.showToast(`Local command executed: ${localCommand}`, 'success');
            }
            if (!topic && !localCommand) {
                app.showToast('Command sent successfully', 'success');
            }
        } else {
            const errorMessage = result.error || result.message || `HTTP ${response.status}: ${response.statusText}`;
            throw new Error(errorMessage);
        }
    } catch (error) {
        console.error('❌ Failed to send command:', error);
        app.showToast(`Failed to send command: ${error.message}`, 'danger');
    }
}

async function sendSliderCommand(deviceId, topic, label, value) {
    if (!deviceId || !topic || !label) {
        console.error('Missing required parameters for slider command');
        app.showToast('Missing required parameters for slider command', 'danger');
        return;
    }

    const payload = JSON.stringify({
        [label.toLowerCase()]: parseInt(value, 10)
    });
    
    await sendCommand(deviceId, topic, payload, '', 'slider');
}

function updateSliderValue(deviceId, label, value, context = '') {
    if (!deviceId || !label) {
        console.warn('Missing parameters for updateSliderValue');
        return;
    }

    const suffix = context ? `-${context}` : '';
    const element = document.getElementById(`slider-${deviceId}-${label}${suffix}`);
    if (element) {
        element.textContent = String(value);
    }
    
    if (!context) {
        updateSliderValue(deviceId, label, value, 'all');
        
        const allElements = document.querySelectorAll(`[id^="slider-${deviceId}-${label}-"]`);
        allElements.forEach(el => {
            el.textContent = String(value);
        });
    }
}

async function toggleCommand(deviceId, topic, payload, localCommand, button) {
    if (!button) {
        console.error('Button element is required for toggle command');
        return;
    }

    const isActive = button.classList.contains('active');
    const buttonId = button.id || '';
    const labelParts = buttonId.split('-');
    
    if (labelParts.length < 3) {
        console.error('Invalid button ID format:', buttonId);
        return;
    }

    const label = labelParts[2];
    const allToggleButtons = document.querySelectorAll(`[id^="toggle-${deviceId}-${label}"]`);
    
    const originalText = button.textContent.replace(/.*?(\w+.*)/g, '$1').trim();
    
    allToggleButtons.forEach(btn => {
        if (isActive) {
            btn.classList.remove('active');
            btn.innerHTML = `<i class="bi bi-toggle-off"></i> ${originalText}`;
        } else {
            btn.classList.add('active');
            btn.innerHTML = `<i class="bi bi-toggle-on"></i> ${originalText}`;
        }
    });
    
    await sendCommand(deviceId, topic, payload, localCommand, 'toggle');
}

async function toggleAutomation(automationId) {
    if (!automationId) {
        console.error('Automation ID is required');
        app.showToast('Automation ID is required', 'danger');
        return;
    }

    try {
        const automation = app.automationStatus[automationId];
        if (!automation) {
            throw new Error('Automation not found in status');
        }
        
        const action = automation.enabled ? 'disable' : 'enable';
        
        const response = await fetch('/api/automations', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                automationId: String(automationId),
                action: action
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        const result = await response.json();
        console.log('Automation toggle result:', result);
        
        app.showToast(`Automation ${action}d successfully`, 'success');
        app.loadAutomationStatus();
    } catch (error) {
        console.error('Failed to toggle automation:', error);
        app.showToast(`Failed to toggle automation: ${error.message}`, 'danger');
    }
}

async function triggerAutomation(automationId) {
    if (!automationId) {
        console.error('Automation ID is required');
        app.showToast('Automation ID is required', 'danger');
        return;
    }

    try {
        const response = await fetch('/api/automations', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                automationId: String(automationId),
                action: 'trigger'
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }

        const result = await response.json();
        console.log('Automation trigger result:', result);
        
        app.show