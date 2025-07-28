// Global variables
let autoRefreshEnabled = true;
let refreshIntervals = [];
let commandExecuted = false;

// Auto-refresh output tab if it's active
function refreshOutput() {
    if (!autoRefreshEnabled) return;
    
    if (document.getElementById('output-tab').classList.contains('active')) {
        fetch('/output')
            .then(response => response.text())
            .then(data => {
                document.getElementById('output-content').textContent = data;
                // Auto-scroll to bottom
                const outputElement = document.getElementById('output-content');
                outputElement.scrollTop = outputElement.scrollHeight;
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
        // Force refresh output after switching
        setTimeout(refreshOutput, 100);
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

// Check if we should auto-switch to output tab on page load
function checkForNewOutput() {
    // Check if there's new output (not the default message)
    fetch('/output')
        .then(response => response.text())
        .then(data => {
            if (data && data.trim() !== 'No commands executed yet.' && commandExecuted) {
                switchToOutputTab();
                commandExecuted = false;
            }
        })
        .catch(error => {
            console.error('Error checking output:', error);
        });
}

// Add event listeners to command forms
document.addEventListener('DOMContentLoaded', function() {
    const forms = document.querySelectorAll('form[action="/run"]');
    forms.forEach(form => {
        form.addEventListener('submit', function(e) {
            commandExecuted = true;
            const button = form.querySelector('button[type="submit"]');
            if (button) {
                button.disabled = true;
                button.innerHTML = '<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> Running...';
                
                // Re-enable button after form submission
                setTimeout(() => {
                    button.disabled = false;
                    button.innerHTML = button.getAttribute('data-original-text') || 'Run Command';
                }, 2000);
            }
        });
        
        // Store original button text
        const button = form.querySelector('button[type="submit"]');
        if (button) {
            button.setAttribute('data-original-text', button.innerHTML);
        }
    });
    
    // Start auto-refresh
    startAutoRefresh();
    
    // Check for output periodically after page load
    setTimeout(checkForNewOutput, 1000);
    
    // Add Bootstrap tab event listeners
    const tabTriggers = document.querySelectorAll('[data-bs-toggle="tab"]');
    tabTriggers.forEach(trigger => {
        trigger.addEventListener('shown.bs.tab', function (event) {
            const targetId = event.target.getAttribute('data-bs-target');
            if (targetId === '#output') {
                // Force refresh when output tab is shown
                setTimeout(refreshOutput, 100);
            } else if (targetId === '#about') {
                // Force refresh when about tab is shown
                setTimeout(updateStats, 100);
            }
        });
    });
});
