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
        this.init();
    }

    init() {
        this.connectWebSocket();
        this.setupToasts();
        this.setupLoadChart();
        this.startSystemStatsPolling();
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
        setInterval(() => this.updateSystemStats(), 30000); // Update every 30 seconds
    }

    async updateSystemStats() {
        try {
            const response = await fetch('/api/system-stats');
            const stats = await response.json();
            
            // Update uptime
            document.getElementById('uptime-text').textContent = stats.uptime || 'Unknown';
            
            // Update memory
            const memoryPercent = stats.memoryTotal > 0 ? 
                ((stats.memoryUsed / stats.memoryTotal) * 100).toFixed(1) : 0;
            document.getElementById('memory-text').textContent = `${memoryPercent}%`;
            document.getElementById('memory-bar').style.width = `${memoryPercent}%`;
            
            // Update load chart
            const now = new Date().toLocaleTimeString();
            this.loadData.labels.push(now);
            this.loadData.load1.push(stats.loadAvg1 || 0);
            this.loadData.load5.push(stats.loadAvg5 || 0);
            this.loadData.load15.push(stats.loadAvg15 || 0);
            
            // Keep only last maxDataPoints
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
        this.ws = new WebSocket('ws://' + window.location.host + '/ws');
        
        this.ws.onopen = () => {
            this.showToast('Connected to server', 'success');
            document.getElementById('system-status').innerHTML = 
                '<i class="bi bi-circle-fill text-success"></i> System Online';
        };

        this.ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            if (message.type === 'status_update') {
                this.updateDeviceStatus(message.deviceId, message.data);
            }
        };

        this.ws.onclose = () => {
            this.showToast('Connection lost. Reconnecting...', 'warning');
            document.getElementById('system-status').innerHTML = 
                '<i class="bi bi-circle-fill text-warning"></i> Reconnecting...';
            setTimeout(() => this.connectWebSocket(), 3000);
        };

        this.ws.onerror = (error) => {
            this.showToast('Connection error', 'danger');
            document.getElementById('system-status').innerHTML = 
                '<i class="bi bi-circle-fill text-danger"></i> Connection Error';
        };
    }

    updateDeviceStatus(deviceId, status) {
        const statusElement = document.getElementById('status-' + deviceId);
        if (!statusElement) return;

        let statusText = 'Online';
        let badgeClass = 'bg-success';
        let iconClass = 'bi-circle-fill text-success';

        if (status.value !== undefined) {
            statusText += ' - ' + status.value;
        }

        if (status.lastUpdate) {
            const time = new Date(status.lastUpdate).toLocaleTimeString();
            statusText += ' (' + time + ')';
        }

        statusElement.innerHTML = `<span class="badge ${badgeClass}"><i class="bi ${iconClass}"></i> ${statusText}</span>`;
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
}

// Global functions for button clicks (called from HTML)
async function sendCommand(deviceId, topic, payload, localCommand) {
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
                localCommand: localCommand
            })
        });

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
            throw new Error(`HTTP ${response.status}`);
        }
    } catch (error) {
        console.error('Failed to send command:', error);
        app.showToast('Failed to send command', 'danger');
    }
}

async function sendSliderCommand(deviceId, topic, label, value) {
    const payload = JSON.stringify({
        [label.toLowerCase()]: parseInt(value)
    });
    
    await sendCommand(deviceId, topic, payload, '');
}

function updateSliderValue(deviceId, label, value) {
    document.getElementById(`slider-${deviceId}-${label}`).textContent = value;
}

async function toggleCommand(deviceId, topic, payload, localCommand, button) {
    const isActive = button.classList.contains('active');
    
    if (isActive) {
        button.classList.remove('active');
        button.innerHTML = '<i class="bi bi-toggle-off"></i> ' + button.textContent.trim();
    } else {
        button.classList.add('active');
        button.innerHTML = '<i class="bi bi-toggle-on"></i> ' + button.textContent.trim();
    }
    
    await sendCommand(deviceId, topic, payload, localCommand);
}

function filterCategory(categoryId) {
    const devices = document.querySelectorAll('.device-card');
    const buttons = document.querySelectorAll('[data-category]');
    
    // Update button states
    buttons.forEach(btn => {
        btn.classList.remove('active');
        if (btn.dataset.category === categoryId) {
            btn.classList.add('active');
        }
    });
    
    // Show/hide devices
    devices.forEach(device => {
        if (categoryId === 'all' || device.dataset.category === categoryId) {
            device.style.display = 'block';
        } else {
            device.style.display = 'none';
        }
    });
}

// Initialize the application
let app;
document.addEventListener('DOMContentLoaded', function() {
    app = new HomeAutomation();
});
