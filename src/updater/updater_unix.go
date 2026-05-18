//go:build !windows

package updater

import (
	"fmt"
	"os"
	"syscall"
)

// replaceBinary atomically replaces the running binary with newBinaryPath.
// On Unix, renaming over a running executable is safe — the old inode stays
// alive in memory until the process exits.
func replaceBinary(currentPath, newBinaryPath string) error {
	info, err := os.Stat(currentPath)
	if err != nil {
		return fmt.Errorf("stat current binary: %w", err)
	}

	if err := os.Rename(newBinaryPath, currentPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	if err := os.Chmod(currentPath, info.Mode()); err != nil {
		return fmt.Errorf("restore permissions: %w", err)
	}

	return nil
}

// RestartSelf replaces the current process image with the updated binary via
// syscall.Exec (Unix exec-over-self).  This function does not return on
// success.
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
