//go:build windows

package terminal

// OnResize registers a callback for terminal resize events.
// On Windows there is no SIGWINCH signal; this is a no-op stub that returns
// a no-op stop function so callers compile identically on all platforms.
func OnResize(callback func(cols, rows int)) (stop func()) {
	return func() {}
}
