//go:build !windows

package display

// Internal tests for spinner functions that require same-package access.

import (
	"testing"
	"time"
)

// TestANSISpinner_StartStopCycle exercises the goroutine start and stop path.
func TestANSISpinner_StartStopCycle(t *testing.T) {
	s := &ANSISpinner{
		message: "testing",
		frames:  ansiFrames,
	}
	s.Start()
	// Give the goroutine time to tick at least once
	time.Sleep(150 * time.Millisecond)
	s.Stop()
}

// TestANSISpinner_StopWithoutStart verifies Stop on unstarted spinner is safe.
func TestANSISpinner_StopWithoutStart(t *testing.T) {
	s := &ANSISpinner{
		message: "testing",
		frames:  ansiFrames,
	}
	// done channel is nil; Stop should handle this gracefully
	s.Stop()
}

// TestANSISpinner_MultipleStops verifies calling Stop twice is safe.
func TestANSISpinner_MultipleStops(t *testing.T) {
	s := &ANSISpinner{
		message: "testing",
		frames:  ansiFrames,
	}
	s.Start()
	time.Sleep(50 * time.Millisecond)
	s.Stop()
	s.Stop()
}

// TestANSISpinner_SetMessage verifies message can be updated while running.
func TestANSISpinner_SetMessage(t *testing.T) {
	s := &ANSISpinner{
		message: "initial",
		frames:  textFrames,
	}
	s.Start()
	time.Sleep(50 * time.Millisecond)
	s.SetMessage("updated")
	if s.message != "updated" {
		t.Errorf("message: got %q, want %q", s.message, "updated")
	}
	s.Stop()
}
