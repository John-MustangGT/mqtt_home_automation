// Global variables
let autoRefreshEnabled = true;
let refreshIntervals = [];
let lastOutputLength = 0;

// Auto-refresh output tab if it's active
function refreshOutput() {
    console.log('Refreshing output...');
    
    fetch('/output')
        .then(response => {
            console.log('Output response status:', response.status);
            return response.text();
        })
        .then(data => {
            console.log('Output data length:', data.length);
            console.log('Output preview:', data.substring(0, 100));
            
            const outputElement = document.getElementById('output-content');
            if (outputElement) {
                outputElement.textContent = data;
                // Auto-scroll to bottom
                outputElement.scrollTop = outputElement.scrollHeight;
                
                // Update last known length
                lastOutputLength = data.length;
            } else {
                console.error('Output element not found');
            }
        })
        .catch(error => {
            console.error('Error fetching output:', error);
        });
}

// Force refresh output (called manually)
function forceRefreshOutput() {
    console.log('Force refreshing output...');
    refreshOutput();
}

// Update current time
function updateTime() {
    if (!autoRefreshEnabled) return;
    
    fetch('/api/time')
        .then(response => response.text())
        .then(time => {
            const timeElement = document.getElementById('current-time');
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
    const textElement = document.getElementById('auto-refresh-text');
    
    if (autoRefreshEnabled) {
        if (textElement) {
            textElement.textContent = 'Pause Auto-refresh';
            textElement.parentElement.innerHTML = '⏸️ <span id="auto-refresh-text">Pause Auto-refresh</span>';
        }
        startAutoRefresh();
        console.log('Auto-refresh enabled');
    } else {
        if (textElement) {
            textElement.textContent = 'Resume Auto-refresh';
            textElement.parentElement.innerHTML = '▶️ <span id="auto-refresh-text">Resume Auto-refresh</span>';
        }
        stopAutoRefresh();
        console.log('Auto-refresh disabled');
    }
}

// Start auto-refresh intervals
function startAutoRefresh() {
    stopAutoRefresh(); // Clear existing intervals
    
    // More frequent output refresh
    refreshIntervals.push(setInterval(() => {
        if (document.getElementById('output-tab').classList.contains('active') || 
            document.visibilityState === 'visible') {
            refreshOutput();
        }
    }, 1000));
    
    refreshIntervals.push(setInterval(updateTime, 1000));
    refreshIntervals.push(setInterval(updateStats, 5000));
    
    console.log('Auto-refresh intervals started');
}

// Stop auto-refresh intervals
function stopAutoRefresh() {
    refreshIntervals.forEach(interval => clearInterval(interval));
    refreshIntervals = [];
    console.log('Auto-refresh intervals stopped');
}

// Clear output function
function clearOutput() {
    document.getElementById('output-content').textContent = 'Output cleared.';
    lastOutputLength = 0;
}

// Auto-switch to output tab when command is executed
function switchToOutputTab() {
    console.log('Switching to output tab...');
    const outputTab = document.getElementById('output-tab');
    if (outputTab) {
        outputTab.click();
        // Force refresh output after switching
        setTimeout(() => {
            forceRefreshOutput();
        }, 200);
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
    console.log('DOM loaded, initializing app...');
    
    const forms = document.querySelectorAll('form[action="/run"]');
    console.log('Found', forms.length, 'command forms');
    
    forms.forEach((form, index) => {
        const button = form.querySelector('button[type="submit"]');
        if (button) {
            // Store original button text
            button.setAttribute('data-original-text', button.innerHTML);
            
            form.addEventListener('submit', function(e) {
                console.log('Command form', index, 'submitted');
                
                button.disabled = true;
                button.innerHTML = '<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> Running...';
                
                // Switch to output tab after a short delay to allow form submission
                setTimeout(() => {
                    switchToOutputTab();
                }, 100);
                
                // Re-enable button after submission
                setTimeout(() => {
                    button.disabled = false;
                    button.innerHTML = button.getAttribute('data-original-text');
                }, 3000);
            });
        }
    });
    
    // Add Bootstrap tab event listeners
    const tabTriggers = document.querySelectorAll('[data-bs-toggle="tab"]');
    console.log('Found', tabTriggers.length, 'tab triggers');
    
    tabTriggers.forEach(trigger => {
        trigger.addEventListener('shown.bs.tab', function (event) {
            const targetId = event.target.getAttribute('data-bs-target');
            console.log('Tab switched to:', targetId);
            
            if (targetId === '#output') {
                // Force refresh when output tab is shown
                setTimeout(forceRefreshOutput, 100);
            } else if (targetId === '#about') {
                // Force refresh when about tab is shown
                setTimeout(updateStats, 100);
            }
        });
    });
    
    // Initial output refresh
    setTimeout(() => {
        console.log('Initial output refresh...');
        forceRefreshOutput();
    }, 500);
    
    // Start auto-refresh
    startAutoRefresh();
    
    // Add a manual refresh button to the output tab for debugging
    const outputCard = document.querySelector('#output .card-header');
    if (outputCard) {
        const refreshButton = document.createElement('button');
        refreshButton.className = 'btn btn-sm btn-outline-primary ms-2';
        refreshButton.innerHTML = '🔄 Refresh';
        refreshButton.onclick = forceRefreshOutput;
        outputCard.appendChild(refreshButton);
    }
    
    console.log('App initialization complete');
});
