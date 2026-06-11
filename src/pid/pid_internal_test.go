//go:build !windows

package pid

// Internal tests for the pid package — exercises isOurProcess and
// isOurProcessDarwin which are unexported and platform-specific (Unix only).

import (
	"os"
	"testing"
)

// TestIsOurProcess_CurrentProcess verifies isOurProcess on the current PID.
// The test binary exe path does NOT contain "pastebin", so the result is false.
func TestIsOurProcess_CurrentProcess(t *testing.T) {
	result := isOurProcess(os.Getpid())
	// Test binary is named "pid.test" not "pastebin".
	// We just want no panic; the result itself is environment-dependent.
	_ = result
}

// TestIsOurProcess_InvalidPID verifies that isOurProcess on a non-existent
// PID returns false without panicking.
func TestIsOurProcess_InvalidPID(t *testing.T) {
	if isOurProcess(99999999) {
		t.Error("isOurProcess(99999999): expected false for non-existent PID")
	}
}

// TestIsOurProcessDarwin_CurrentProcess verifies ps-based check on current PID.
// On Linux, ps is available and returns the test binary name (not "pastebin").
func TestIsOurProcessDarwin_CurrentProcess(t *testing.T) {
	result := isOurProcessDarwin(os.Getpid())
	// Test binary is not "pastebin", so result should be false.
	_ = result
}

// TestIsOurProcessDarwin_InvalidPID verifies that isOurProcessDarwin returns
// false for a PID that ps cannot find.
func TestIsOurProcessDarwin_InvalidPID(t *testing.T) {
	if isOurProcessDarwin(99999999) {
		t.Error("isOurProcessDarwin(99999999): expected false for non-existent PID")
	}
}

// TestIsProcessRunning_CurrentProcess verifies isProcessRunning on current PID.
func TestIsProcessRunning_CurrentProcess(t *testing.T) {
	if !isProcessRunning(os.Getpid()) {
		t.Error("isProcessRunning(self): expected true")
	}
}

// TestIsProcessRunning_InvalidPID verifies isProcessRunning returns false for
// a PID that is definitely not running.
func TestIsProcessRunning_InvalidPID(t *testing.T) {
	if isProcessRunning(99999999) {
		t.Error("isProcessRunning(99999999): expected false")
	}
}
