//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// replaceBinary replaces the running binary on Windows.
// Windows cannot rename over a running executable, so we rename the current
// binary to .old and move the new binary into its place.  The .old file is
// left on disk; next startup can clean it up.
func replaceBinary(currentPath, newBinaryPath string) error {
	oldPath := currentPath + ".old"

	// Remove any leftover .old file from a previous update.
	os.Remove(oldPath)

	// Rename the running binary out of the way.
	if err := os.Rename(currentPath, oldPath); err != nil {
		return fmt.Errorf("rename current binary: %w", err)
	}

	// Move the new binary into place.
	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		// Attempt to restore the original binary.
		os.Rename(oldPath, currentPath)
		return fmt.Errorf("move new binary: %w", err)
	}

	return nil
}

// RestartSelf spawns a new instance of the updated binary and exits the
// current process.  Windows does not support exec-over-self.
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new process: %w", err)
	}

	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
	return nil // unreachable
}
