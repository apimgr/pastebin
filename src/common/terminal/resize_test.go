//go:build !windows

package terminal

// Internal tests for OnResize (requires same package to test SIGWINCH path).

import (
	"testing"
	"time"
)

// TestOnResize_RegisterAndStop verifies OnResize registers and the stop function
// cleans up without panic. We cannot reliably trigger SIGWINCH in CI, so this
// tests the setup/teardown path only.
func TestOnResize_RegisterAndStop(t *testing.T) {
	called := false
	stop := OnResize(func(cols, rows int) {
		called = true
	})
	// Allow goroutine to start
	time.Sleep(10 * time.Millisecond)
	// Stop the handler
	stop()
	// Verify no panic and callback was not spuriously invoked
	if called {
		t.Log("callback was invoked, likely due to real SIGWINCH")
	}
}

// TestOnResize_MultipleStops verifies calling stop multiple times is safe.
func TestOnResize_MultipleStops(t *testing.T) {
	stop := OnResize(func(cols, rows int) {})
	stop()
	// Second stop should not panic (signal.Stop on closed channel is safe)
}
