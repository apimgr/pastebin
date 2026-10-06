//go:build windows

package updater

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"golang.org/x/sys/windows"
)

// replaceBinary replaces the running binary on Windows.
// Windows cannot rename over a running executable, so we rename the current
// binary to .old and move the new binary into its place.  The .old file is
// still locked by this process, so it is scheduled for deletion at the next
// reboot — otherwise every update would orphan a copy of the old binary.
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

	// Schedule the old binary for deletion on reboot (MOVEFILE_DELAY_UNTIL_REBOOT).
	// Best effort: the swap above has already succeeded, so a scheduling failure
	// must not abort the install. Failing here would report "update failed" and
	// skip RestartSelf, leaving the new binary on disk but the old image still
	// running. The stale .old copy is a harmless leftover, so log and continue.
	oldPathPtr, err := windows.UTF16PtrFromString(oldPath)
	if err == nil {
		err = windows.MoveFileEx(oldPathPtr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
	if err != nil {
		log.Printf("updater: could not schedule %s for deletion at reboot: %v", oldPath, err)
	}

	return nil
}

// RestartSelf spawns a new instance of the updated binary and exits the
// current process.  Windows does not support exec-over-self.  The update
// selector is dropped from argv so the new process starts the server instead
// of re-running the update.
func RestartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, restartArgs(os.Args)[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start new process: %w", err)
	}

	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
	// unreachable
	return nil
}
