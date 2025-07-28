// Global variables for Ionic interface
let autoRefreshEnabled = true;
let refreshIntervals = [];
let currentTab = 'commands';

// Initialize Ionic app
document.addEventListener('DOMContentLoaded', function() {
    initializeIonicApp();
});

function initializeIonicApp() {
    // Set up segment change handler
    const segment = document.getElementById('main-segment');
    if (segment) {
        segment.addEventListener('ionChange', handleSegmentChange);
    }
    
    // Set up command form handlers
    const forms = document.querySelectorAll('form[action="/run"]');
    forms.forEach(form => {
        form.addEventListener('submit', function(e) {
            const button = form.querySelector('ion-button');
            if (button) {
                button.disabled = true;
                setTimeout(() => {
                    button.disabled = false;
                    switchToOutputTab();
                }, 1000);
            }
        });
    });
    
    // Start auto-refresh
    startAutoRefresh();
}

// Handle segment tab changes
function handleSegmentChange(event) {
    const selectedTab = event.detail.value;
    currentTab = selectedTab;
    
    // Hide all tab contents
    const tabContents = document.querySelectorAll('.tab-content');
    tabContents.forEach(content => {
        content.classList.remove('active');
    });
    
    // Show selected tab content
    const activeContent = document.getElementById(selectedTab + '-content');
    if (activeContent) {
        activeContent.classList.add('active');
    }
    
    // Update stats if about tab is selected
    if (selectedTab === 'about') {
        updateStats();
    }
}

// Auto-refresh output tab if it's active
function refreshOutput() {
    if (!autoRefreshEnabled) return;
    
    if (currentTab === 'output') {
        fetch('/output')
            .then(response => response.text())
            .then(data => {
                const outputElement = document.getElementById('output-display');
                if (outputElement) {
                    outputElement.textContent = data;
                }
            })
            .catch(error => {
                console.error('Error fetching output:', error);
            });
    }
}

// Update current time in header
function updateTime() {
    if (!autoRefreshEnabled) return;
    
    fetch('/api/time')
        .then(response => response.text())
        .then(time => {
            const timeElement = document.querySelector('#time-display ion-label');
            if (timeElement) {
                timeElement.textContent = time;
            }
        })
        .catch(error => {
            console.error('Error fetching time:', error);
        });
}

// Update system stats in about tab
function updateStats() {
    if (!autoRefreshEnabled) return;
    
    if (currentTab === 'about') {
        fetch('/api/stats')
            .then(response => response.json())
            .then(data => {
                const elements = {
                    'server-uptime': data.server_uptime,
                    'system-uptime': data.system_uptime,
                    'system-load': data.system_load,
                    'memory-info': data.memory_info,
                    'last-reload': data.last_reload,
                    'button-count': data.button_count
                };
                
                Object.entries(elements).forEach(([id, value]) => {
                    const element = document.getElementById(id);
                    if (element) {
                        element.textContent = value;
                    }
                });
            })
            .catch(error => {
                console.error('Error fetching stats:', error);
            });
    }
}

// Toggle auto-refresh functionality
function toggleAutoRefresh() {
    autoRefreshEnabled = !autoRefreshEnabled;
    const textElement = document.getElementById('auto-refresh-text');
    const iconElement = document.getElementById('refresh-icon');
    
    if (autoRefreshEnabled) {
        textElement.textContent = 'Pause Auto-refresh';
        if (iconElement) {
            iconElement.name = 'pause-outline';
        }
        startAutoRefresh();
    } else {
        textElement.textContent = 'Resume Auto-refresh';
        if (iconElement) {
            iconElement.name = 'play-outline';
        }
        stopAutoRefresh();
    }
}

// Start auto-refresh intervals
function startAutoRefresh() {
    stopAutoRefresh(); // Clear existing intervals
    refreshIntervals.push(setInterval(refreshOutput, 2000));
    refreshIntervals.push(setInterval(updateTime, 1000));
    refreshIntervals.push(setInterval(updateStats, 5000));
}

// Stop auto-refresh intervals
function stopAutoRefresh() {
    refreshIntervals.forEach(interval => clearInterval(interval));
    refreshIntervals = [];
}

// Clear output function
function clearOutput() {
    const outputElement = document.getElementById('output-display');
    if (outputElement) {
        outputElement.textContent = 'Output cleared.';
    }
}

// Switch to output tab when command is executed
function switchToOutputTab() {
    const segment = document.getElementById('main-segment');
    if (segment) {
        segment.value = 'output';
        currentTab = 'output';
        
        // Manually trigger tab change
        const tabContents = document.querySelectorAll('.tab-content');
        tabContents.forEach(content => {
            content.classList.remove('active');
        });
        
        const outputContent = document.getElementById('output-content');
        if (outputContent) {
            outputContent.classList.add('active');
        }
        
        // Refresh output immediately
        setTimeout(refreshOutput, 100);
    }
}

// View XML configuration in modal
function viewXmlConfig() {
    fetch('/config.xml')
        .then(response => response.text())
        .then(data => {
            const xmlContent = document.getElementById('xml-content');
            if (xmlContent) {
                xmlContent.textContent = data;
            }
            
            const modal = document.getElementById('xml-modal');
            if (modal) {
                modal.isOpen = true;
            }
        })
        .catch(error => {
            console.error('Error fetching XML config:', error);
            // Show Ionic toast or alert
            showIonicAlert('Error', 'Error loading XML configuration');
        });
}

// Close XML modal
function closeXmlModal() {
    const modal = document.getElementById('xml-modal');
    if (modal) {
        modal.isOpen = false;
    }
}

// Download XML config
function downloadXmlConfig() {
    window.open('/config.xml', '_blank');
}

// Refresh about tab information
function refreshAbout() {
    updateStats();
}

// Show Ionic alert (utility function)
function showIonicAlert(header, message) {
    // Create and present an Ionic alert
    const alert = document.createElement('ion-alert');
    alert.header = header;
    alert.message = message;
    alert.buttons = ['OK'];
    
    document.body.appendChild(alert);
    alert.present();
    
    // Remove alert after it's dismissed
    alert.addEventListener('didDismiss', () => {
        document.body.removeChild(alert);
    });
}

// Show Ionic toast (utility function)
function showIonicToast(message, color = 'primary') {
    const toast = document.createElement('ion-toast');
    toast.message = message;
    toast.duration = 2000;
    toast.color = color;
    toast.position = 'bottom';
    
    document.body.appendChild(toast);
    toast.present();
    
    // Remove toast after it's dismissed
    toast.addEventListener('didDismiss', () => {
        document.body.removeChild(toast);
    });
}
