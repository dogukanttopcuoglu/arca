package chunking

import "sync"

// WarningCollector provides thread-safe diagnostic warning collection for chunking pipeline execution.
type WarningCollector struct {
	mu       sync.RWMutex
	warnings []string
}

// NewWarningCollector creates a new WarningCollector.
func NewWarningCollector() *WarningCollector {
	return &WarningCollector{
		warnings: make([]string, 0),
	}
}

// AddWarning adds a diagnostic message to the warnings slice.
func (w *WarningCollector) AddWarning(msg string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = append(w.warnings, msg)
}

// Warnings returns a copy of all accumulated warnings.
func (w *WarningCollector) Warnings() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	cp := make([]string, len(w.warnings))
	copy(cp, w.warnings)
	return cp
}

// Clear resets accumulated warnings.
func (w *WarningCollector) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.warnings = make([]string, 0)
}
