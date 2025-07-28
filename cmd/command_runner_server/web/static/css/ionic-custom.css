/* Custom styles for Ionic Command Runner */

:root {
  --ion-color-primary: #3880ff;
  --ion-color-primary-rgb: 56, 128, 255;
  --ion-color-primary-contrast: #ffffff;
  --ion-color-primary-contrast-rgb: 255, 255, 255;
  --ion-color-primary-shade: #3171e0;
  --ion-color-primary-tint: #4c8dff;

  --ion-color-secondary: #3dc2ff;
  --ion-color-secondary-rgb: 61, 194, 255;
  --ion-color-secondary-contrast: #ffffff;
  --ion-color-secondary-contrast-rgb: 255, 255, 255;
  --ion-color-secondary-shade: #36abe0;
  --ion-color-secondary-tint: #50c8ff;

  --ion-color-tertiary: #5260ff;
  --ion-color-tertiary-rgb: 82, 96, 255;
  --ion-color-tertiary-contrast: #ffffff;
  --ion-color-tertiary-contrast-rgb: 255, 255, 255;
  --ion-color-tertiary-shade: #4854e0;
  --ion-color-tertiary-tint: #6370ff;

  --ion-color-success: #2dd36f;
  --ion-color-success-rgb: 45, 211, 111;
  --ion-color-success-contrast: #ffffff;
  --ion-color-success-contrast-rgb: 255, 255, 255;
  --ion-color-success-shade: #28ba62;
  --ion-color-success-tint: #42d77d;

  --ion-color-warning: #ffc409;
  --ion-color-warning-rgb: 255, 196, 9;
  --ion-color-warning-contrast: #000000;
  --ion-color-warning-contrast-rgb: 0, 0, 0;
  --ion-color-warning-shade: #e0ac08;
  --ion-color-warning-tint: #ffca22;

  --ion-color-danger: #eb445a;
  --ion-color-danger-rgb: 235, 68, 90;
  --ion-color-danger-contrast: #ffffff;
  --ion-color-danger-contrast-rgb: 255, 255, 255;
  --ion-color-danger-shade: #cf3c4f;
  --ion-color-danger-tint: #ed576b;

  --ion-color-dark: #222428;
  --ion-color-dark-rgb: 34, 36, 40;
  --ion-color-dark-contrast: #ffffff;
  --ion-color-dark-contrast-rgb: 255, 255, 255;
  --ion-color-dark-shade: #1e2023;
  --ion-color-dark-tint: #383a3e;

  --ion-color-medium: #92949c;
  --ion-color-medium-rgb: 146, 148, 156;
  --ion-color-medium-contrast: #ffffff;
  --ion-color-medium-contrast-rgb: 255, 255, 255;
  --ion-color-medium-shade: #808289;
  --ion-color-medium-tint: #9d9fa6;

  --ion-color-light: #f4f5f8;
  --ion-color-light-rgb: 244, 245, 248;
  --ion-color-light-contrast: #000000;
  --ion-color-light-contrast-rgb: 0, 0, 0;
  --ion-color-light-shade: #d7d8da;
  --ion-color-light-tint: #f5f6f9;
}

/* Custom styles */
.tab-content {
  display: none;
  padding: 16px;
}

.tab-content.active {
  display: block;
}

.command-button {
  margin: 8px 0;
  font-weight: 500;
}

.command-button:hover {
  transform: translateY(-2px);
  transition: transform 0.2s ease-in-out;
}

.output-terminal {
  background-color: #1e1e1e;
  color: #d4d4d4;
  font-family: 'Courier New', Courier, monospace;
  font-size: 0.875rem;
  line-height: 1.4;
  padding: 16px;
  border-radius: 8px;
  white-space: pre-wrap;
  word-wrap: break-word;
  overflow-y: auto;
  max-height: 400px;
  border: 1px solid var(--ion-color-medium);
}

.xml-display {
  background-color: #f8f9fa;
  color: #212529;
  font-family: 'Courier New', Courier, monospace;
  font-size: 0.875rem;
  line-height: 1.4;
  padding: 16px;
  border-radius: 8px;
  border: 1px solid var(--ion-color-light-shade);
  overflow-x: auto;
  white-space: pre-wrap;
}

/* Loading animation for buttons */
.command-button:disabled {
  position: relative;
  opacity: 0.6;
}

.command-button:disabled::after {
  content: "";
  position: absolute;
  width: 16px;
  height: 16px;
  top: 50%;
  left: 50%;
  margin-left: -8px;
  margin-top: -8px;
  border: 2px solid var(--ion-color-primary-contrast);
  border-radius: 50%;
  border-top-color: transparent;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Segment styling */
ion-segment {
  margin: 16px;
}

/* Card enhancements */
ion-card {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
}

/* Button spacing */
ion-button {
  margin: 4px 0;
}

/* Time display in header */
#time-display {
  font-size: 0.875rem;
}

/* List item styling */
ion-item {
  --padding-start: 16px;
  --inner-padding-end: 16px;
}

ion-item h3 {
  margin: 0;
  font-weight: 600;
  color: var(--ion-color-dark);
}

ion-item p {
  margin: 4px 0 0 0;
  color: var(--ion-color-medium-shade);
}

/* Modal styling */
ion-modal {
  --height: 80vh;
  --border-radius: 16px;
}

/* Responsive adjustments */
@media (max-width: 768px) {
  .command-button {
    font-size: 0.875rem;
  }
  
  .output-terminal {
    font-size: 0.75rem;
    max-height: 300px;
  }
  
  ion-segment {
    margin: 8px;
  }
  
  .tab-content {
    padding: 8px;
  }
}

/* Dark mode support */
@media (prefers-color-scheme: dark) {
  .xml-display {
    background-color: #2d2d2d;
    color: #f8f8f2;
    border-color: var(--ion-color-dark-shade);
  }
}
