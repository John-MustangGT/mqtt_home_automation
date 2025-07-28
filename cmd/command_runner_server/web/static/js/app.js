// Change framework function (optional for Bootstrap, but good for consistency)
function changeFramework(framework) {
    debugLog('Changing framework to:', framework);

    const form = document.createElement('form');
    form.method = 'POST';
    form.action = '/set-framework';

    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = 'framework';
    input.value = framework;

    form.appendChild(input);
    document.body.appendChild(form);
    form.submit();

    // Force hard reload after short delay
    setTimeout(() => {
        window.location.reload(true);
    }, 500);
}

// Global variables
let autoRefreshEnabled = true;
let refreshIntervals = [];
let debugMode = false; // Can be enabled via URL parameter or localStorage

// Debug logging function
function debugLog(message, ...args) {
    if (debugMode) {
        console.log('[DEBUG]', message, ...args);
    }
}

// Check for debug mode on load
function initDebugMode() {
    // Check URL parameter
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('debug') === 'true') {
        debugMode = true;
        debugLog('Debug mode enabled via URL parameter');
    }
    
    // Check localStorage
    if (localStorage.getItem('commandRunnerDebug') === 'true') {
        debugMode = true;
        debugLog('Debug mode enabled via localStorage');
    }
    
    // Add debug toggle to page if debug mode is enabled
    if (debugMode) {
        addDebugControls();
    }
}

// Add debug controls to the page
function addDebugControls() {
    const debugPanel = document.createElement('div');
    debugPanel.id = 'debug-panel';
    debugPanel.className = 'position-fixed bottom-0 end-0 p-2 bg-dark text-light rounded-top-start';
    debugPanel.style.zIndex = '9999';
    debugPanel.innerHTML = `
        <div class="mb-2"><strong>Debug Panel</strong></div>
        <button class="btn btn-sm btn-outline-light me-1" onclick="forceRefreshOutput()">Refresh Output</button>
        <button class="btn btn-sm btn-outline-light me-1" onclick="clearDebugLog()">Clear Console</button>
        <button class="btn btn-sm btn-outline-light" onclick="toggleDebugMode()">Disable Debug</button>
        <div class="mt-2 small">
            <div>Auto-refresh: <span id="debug-auto-refresh">enabled</span></div>
            <div>Output length: <span id="debug-output-length">0</span></div>
        </div>
    `;
    document.body.appendChild(debugPanel);
    
    // Update debug info periodically
    setInterval(updateDebugInfo, 1000);
}

// Update debug information
function updateDebugInfo() {
    if (!debugMode) return;
    
    const autoRefreshElement = document.getElementById('debug-auto-refresh');
    const outputLengthElement = document.getElementById('debug-output-length');
    
    if (autoRefreshElement) {
        autoRefreshElement.textContent = autoRefreshEnabled ? 'enabled' : 'disabled';
    }
    
    if (outputLengthElement) {
        const outputContent = document.getElementById('output-content');
        if (outputContent) {
            outputLengthElement.textContent = outputContent.textContent.length;
        }
    }
}

// Toggle debug mode
function toggleDebugMode() {
    debugMode = !debugMode;
    if (debugMode) {
        localStorage.setItem('commandRunnerDebug', 'true');
        location.reload(); // Reload to add debug controls
    } else {
        localStorage.removeItem('commandRunnerDebug');
        const debugPanel = document.getElementById('debug-panel');
        if (debugPanel) {
            debugPanel.remove();
        }
    }
}

// Clear debug log
function clearDebugLog() {
    console.clear();
    debugLog('Console cleared');
}

// Auto-refresh output tab if it's active
function refreshOutput() {
    debugLog('Refreshing output...');
    
    fetch('/output')
        .then(response => {
            debugLog('Output response status:', response.status);
            return response.text();
        })
        .then(data => {
            debugLog('Output data length:', data.length);
            if (debugMode) {
                debugLog('Output preview:', data.substring(0, 100));
            }
            
            const outputElement = document.getElementById('output-content');
            if (outputElement) {
                outputElement.textContent = data;
                // Auto-scroll to bottom
                outputElement.scrollTop = outputElement.scrollHeight;
            } else {
                debugLog('Output element not found');
            }
        })
        .catch(error => {
            debugLog('Error fetching output:', error);
        });
}

// Force refresh output (called manually)
function forceRefreshOutput() {
    debugLog('Force refreshing output...');
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
            debugLog('Error fetching time:', error);
        });
}

// Update system stats in about tab
function updateStats() {
    if (!autoRefreshEnabled) return;
    
    if (document.getElementById('about-tab').classList.contains('active')) {
        debugLog('Updating stats...');
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
                debugLog('Stats updated');
            })
            .catch(error => {
                debugLog('Error fetching stats:', error);
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
        debugLog('Auto-refresh enabled');
    } else {
        if (textElement) {
            textElement.textContent = 'Resume Auto-refresh';
            textElement.parentElement.innerHTML = '▶️ <span id="auto-refresh-text">Resume Auto-refresh</span>';
        }
        stopAutoRefresh();
        debugLog('Auto-refresh disabled');
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
    
    debugLog('Auto-refresh intervals started');
}

// Stop auto-refresh intervals
function stopAutoRefresh() {
    refreshIntervals.forEach(interval => clearInterval(interval));
    refreshIntervals = [];
    debugLog('Auto-refresh intervals stopped');
}

// Clear output function
function clearOutput() {
    document.getElementById('output-content').textContent = 'Output cleared.';
    debugLog('Output cleared');
}

// Auto-switch to output tab when command is executed
function switchToOutputTab() {
    debugLog('Switching to output tab...');
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
    debugLog('Loading XML config...');
    fetch('/config.xml')
        .then(response => response.text())
        .then(data => {
            document.getElementById('xml-content').textContent = data;
            const modal = new bootstrap.Modal(document.getElementById('xmlConfigModal'));
            modal.show();
            debugLog('XML config modal shown');
        })
        .catch(error => {
            debugLog('Error fetching XML config:', error);
            alert('Error loading XML configuration');
        });
}

// Refresh about tab information
function refreshAbout() {
    debugLog('Refreshing about tab...');
    updateStats();
}

// Add event listeners to command forms
document.addEventListener('DOMContentLoaded', function() {
    // Initialize debug mode
    initDebugMode();
    
    debugLog('DOM loaded, initializing app...');
    
    const forms = document.querySelectorAll('form[action="/run"]');
    debugLog('Found', forms.length, 'command forms');
    
    forms.forEach((form, index) => {
        const button = form.querySelector('button[type="submit"]');
        if (button) {
            // Store original button text
            button.setAttribute('data-original-text', button.innerHTML);
            
            form.addEventListener('submit', function(e) {
                debugLog('Command form', index, 'submitted');
                
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
    debugLog('Found', tabTriggers.length, 'tab triggers');
    
    tabTriggers.forEach(trigger => {
        trigger.addEventListener('shown.bs.tab', function (event) {
            const targetId = event.target.getAttribute('data-bs-target');
            debugLog('Tab switched to:', targetId);
            
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
        debugLog('Initial output refresh...');
        forceRefreshOutput();
    }, 500);
    
    // Start auto-refresh
    startAutoRefresh();
    
    debugLog('App initialization complete');
});
