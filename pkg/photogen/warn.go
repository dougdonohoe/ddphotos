package photogen

import (
	"fmt"
	"sync"
)

// WarnCollector accumulates warning messages as they are printed so they can
// be re-displayed as a summary at the end of the run, making them easy to spot.
type WarnCollector struct {
	mu       sync.Mutex
	warnings []string
}

// Warn prints a warning immediately and stores it for the end-of-run summary.
// Safe to call on a nil receiver — prints but does not store.
func (wc *WarnCollector) Warn(msg string) {
	fmt.Print(msg)
	if wc == nil {
		return
	}
	wc.mu.Lock()
	wc.warnings = append(wc.warnings, msg)
	wc.mu.Unlock()
}

// Warnf is a convenience wrapper around Warn that accepts a format string.
func (wc *WarnCollector) Warnf(format string, args ...any) {
	wc.Warn(fmt.Sprintf(format, args...))
}

// PrintSummary re-prints all collected warnings under a header.
// No-op if no warnings were recorded or the receiver is nil.
func (wc *WarnCollector) PrintSummary() {
	if wc == nil {
		return
	}
	wc.mu.Lock()
	defer wc.mu.Unlock()
	if len(wc.warnings) == 0 {
		return
	}
	fmt.Printf("\n=== %d Warning(s) ===\n", len(wc.warnings))
	for _, w := range wc.warnings {
		fmt.Print(w)
	}
}
