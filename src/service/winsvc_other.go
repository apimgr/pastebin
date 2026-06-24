//go:build !windows

package service

import "os"

// IsWindowsService always returns false on non-Windows platforms.
func IsWindowsService() bool { return false }

// isPrivileged reports whether the process has the privileges required to
// install or remove a system service (root on Unix-like systems).
func isPrivileged() bool { return os.Geteuid() == 0 }

// RunAsWindowsService is a no-op on non-Windows platforms.
// It exists so callers can reference it unconditionally.
func RunAsWindowsService(_ string, _ func()) error { return nil }
