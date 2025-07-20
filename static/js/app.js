class HomeAutomation {
    constructor() {
        this.ws = null;
        this.init();
    }

    init() {
        this.connectWebSocket();
        this.setupToasts();
    }

    connectWebSocket() {
        this.ws = new WebSocket('ws://' + window.location.host + '/ws');
        
        this.ws.onopen = () => {
            this.showToast('Connected to server', 'success');
        };

        this.ws.onmessage = (event) => {
            const message = JSON.parse(event.data);
            if (message.type === 'status_update') {
                this.updateDeviceStatus(message.deviceId, message.data);
            }
        };

        this.ws.onclose = () => {
            this.showToast('Connection lost. Reconnecting...', 'warning');
            setTimeout(() => this.connectWebSocket(), 3000);
        };

        this.ws.onerror = (error) => {
            this.showToast('Connection error', 'danger');
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

// Global functions for template compatibility
function sendCommand(deviceId, topic, payload) {
    app.sendCommand(deviceId, topic, payload);
}

function toggleCommand(deviceId, topic, payload, button) {
    app.toggleCommand(deviceId, topic, payload, button);
}

function filterCategory(category) {
    app.filterCategory(category);
}

// Initialize app
const app = {
    homeAutomation: null,

    init() {
        this.homeAutomation = new HomeAutomation();
    },

    sendCommand(deviceId, topic, payload) {
        fetch('/api/control', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({device: deviceId, topic: topic, payload: payload})
        })
        .then(response => {
            if (response.ok) {
                this.homeAutomation.showToast(`Command sent: ${payload}`, 'success');
            } else {
                this.homeAutomation.showToast('Command failed', 'danger');
            }
        })
        .catch(error => {
            this.homeAutomation.showToast('Network error', 'danger');
        });
    },

    toggleCommand(deviceId, topic, payload, button) {
        const isActive = button.classList.contains('active');
        const newPayload = isActive ? 'OFF' : payload;
        const icon = button.querySelector('i');
        
        button.classList.toggle('active');
        if (button.classList.contains('active')) {
            button.classList.remove('btn-outline-primary');
            button.classList.add('btn-success');
            icon.className = 'bi bi-toggle-on';
        } else {
            button.classList.remove('btn-success');
            button.classList.add('btn-outline-primary');
            icon.className = 'bi bi-toggle-off';
        }
        
        this.sendCommand(deviceId, topic, newPayload);
    },

    filterCategory(category) {
        const devices = document.querySelectorAll('.device-card');
        const buttons = document.querySelectorAll('[data-category]');
        
        // Update button states
        buttons.forEach(btn => {
            btn.classList.remove('active');
            btn.classList.add('btn-outline-primary');
            btn.classList.remove('btn-primary');
        });
        
        const activeButton = document.querySelector(`[data-category="${category}"]`);
        if (activeButton) {
            activeButton.classList.add('active');
            activeButton.classList.remove('btn-outline-primary');
            activeButton.classList.add('btn-primary');
        }
        
        // Filter devices with animation
        devices.forEach(device => {
            if (category === 'all' || device.dataset.category === category) {
                device.style.display = 'block';
                device.style.animation = 'fadeIn 0.3s ease-in';
            } else {
                device.style.display = 'none';
            }
        });
    }
};

// Initialize when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    app.init();
});

// Add CSS animation
const style = document.createElement('style');
style.textContent = `
    @keyframes fadeIn {
        from { opacity: 0; transform: translateY(10px); }
        to { opacity: 1; transform: translateY(0); }
    }
`;
document.head.appendChild(style);
