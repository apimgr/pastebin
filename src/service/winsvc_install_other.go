//go:build !windows

package service

import "fmt"

// installWindows is a stub on non-Windows platforms.
// The real implementation lives in winsvc_install_windows.go.
func installWindows() error {
	return fmt.Errorf("Windows service installation is only supported on Windows")
}

// uninstallWindows is a stub on non-Windows platforms.
func uninstallWindows() error {
	return fmt.Errorf("Windows service uninstallation is only supported on Windows")
}
