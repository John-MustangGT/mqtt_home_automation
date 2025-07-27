// Global variables
let autoRefreshEnabled = true;
let refreshIntervals = [];

// Auto-refresh output tab if it's active
function refreshOutput() {
    if (!autoRefreshEnabled) return;
    
    if (document.getElementById('output-tab').classList.contains('active')) {
        fetch('/output')
            .then(response => response.text())
            .then(data => {
                document.getElementById('output-content').textContent = data;
            })
            .catch(error => {
                console.error('Error fetching output:', error);
            });
    }
}

// Update current time
function updateTime() {
    if (!autoRefreshEnabled) return;
    
    fetch('/api/time')
        .then(response => response.text())
        .then(time => {
            document.getElementById('current-time').textContent = time;
        })
        .catch(error => {
            console.error('Error fetching time:', error);
        });
}

// Update system stats in about tab
function updateStats() {
    if (!autoRefreshEnabled) return;
    
    if (document.getElementById('about-tab').classList.contains('active')) {
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
    const button = document.getElementById('auto-refresh-text');
    
    if (autoRefreshEnabled) {
        button.textContent = 'Pause Auto-refresh';
        button.parentElement.innerHTML = '⏸️ <span id="auto-refresh-text">Pause Auto-refresh</span>';
        startAutoRefresh();
    } else {
        button.textContent = 'Resume Auto-refresh';
        button.parentElement.innerHTML = '▶️ <span id="auto-refresh-text">Resume Auto-refresh</span>';
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
    document.getElementById('output-content').textContent = 'Output cleared.';
}

// Auto-switch to output tab when command is executed
function switchToOutputTab() {
    const outputTab = document.getElementById('output-tab');
    if (outputTab) {
        outputTab.click();
    }
}

// View XML configuration in modal
function viewXmlConfig() {
    fetch('/config.xml')
        .then(response => response.text())
        .then(data => {
            document.getElementById('xml-content').textContent = data;
            const modal = new bootstrap.Modal(document.getElementById('xmlConfigModal'));
            modal.show();
        })
        .catch(error => {
            console.error('Error fetching XML config:', error);
            alert('Error loading XML configuration');
        });
}

// Refresh about tab information
function refreshAbout() {
    updateStats();
}

// Add event listeners to command forms
document.addEventListener('DOMContentLoaded', function() {
    const forms = document.querySelectorAll('form[action="/run"]');
    forms.forEach(form => {
        form.addEventListener('submit', function() {
            setTimeout(switchToOutputTab, 100);
        });
    });
    
    // Start auto-refresh
    startAutoRefresh();
    
    // Add visual feedback for button clicks
    const buttons = document.querySelectorAll('button[type="submit"]');
    buttons.forEach(button => {
        button.addEventListener('click', function() {
            this.disabled = true;
            setTimeout(() => {
                this.disabled = false;
            }, 1000);
        });
    });
});
