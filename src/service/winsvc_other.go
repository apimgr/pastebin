//go:build !windows

package service

// IsWindowsService always returns false on non-Windows platforms.
func IsWindowsService() bool { return false }

// isPrivileged is an alias for isElevated for backward compatibility.
func isPrivileged() bool { return isElevated() }

// RunAsWindowsService is a no-op on non-Windows platforms.
// It exists so callers can reference it unconditionally.
func RunAsWindowsService(_ string, _ func()) error { return nil }
