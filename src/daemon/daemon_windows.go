//go:build windows

// Package daemon provides daemonization support.
// On Windows, traditional Unix daemonization is not supported.
// Use --service install && --service start for background execution.
package daemon

import (
	"fmt"
	"os"
)

// Daemonize is a no-op on Windows. It prints a warning directing the user
// to use Windows Service management instead.
func Daemonize() error {
	fmt.Fprintln(os.Stderr, "Warning: --daemon is not supported on Windows.")
	fmt.Fprintln(os.Stderr, "Use '--service --install' and '--service start' to run as a Windows Service.")
	return nil
}
