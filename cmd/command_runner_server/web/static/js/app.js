// Auto-refresh output tab if it's active
function refreshOutput() {
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
    fetch('/api/time')
        .then(response => response.text())
        .then(time => {
            document.getElementById('current-time').textContent = time;
        })
        .catch(error => {
            console.error('Error fetching time:', error);
        });
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
    location.reload();
}

// Add event listeners to command forms
document.addEventListener('DOMContentLoaded', function() {
    const forms = document.querySelectorAll('form[action="/run"]');
    forms.forEach(form => {
        form.addEventListener('submit', function() {
            setTimeout(switchToOutputTab, 100);
        });
    });
    
    // Start auto-refresh for output and time
    setInterval(refreshOutput, 2000);
    setInterval(updateTime, 1000);
});
