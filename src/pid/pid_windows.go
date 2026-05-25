//go:build windows

package pid

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// isProcessRunning uses GetExitCodeProcess to determine whether the process
// is still alive (Windows).
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	handle := windows.Handle(uintptr(process.Pid))
	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	return err == nil && exitCode == windows.STILL_ACTIVE
}

// isOurProcess uses QueryFullProcessImageName to verify the running process
// is our binary, not a PID-reuse collision (Windows).
func isOurProcess(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle) //nolint:errcheck

	var buf [windows.MAX_PATH]uint16
	size := uint32(windows.MAX_PATH)
	err = windows.QueryFullProcessImageName(handle, 0, &buf[0], &size)
	if err != nil {
		return false
	}
	exePath := windows.UTF16ToString(buf[:size])
	return strings.Contains(strings.ToLower(filepath.Base(exePath)), "pastebin")
}
