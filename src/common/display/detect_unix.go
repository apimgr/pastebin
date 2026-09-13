//go:build !windows
// +build !windows

package display

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// detectedOS holds the current OS — overridable in tests to exercise platform
// branches without running on the target OS.
var detectedOS = runtime.GOOS

// guiSupportedOS reports whether the current OS has a gogpu/gogpu windowing
// backend. freebsd/netbsd/openbsd have no backend yet (AI.md PART 32 "GUI
// Mode Requirements"), so GUI mode is never auto-detected there — the
// display falls through to TUI (or CLI with no TTY) instead.
func guiSupportedOS() bool {
	switch detectedOS {
	case "freebsd", "netbsd", "openbsd":
		return false
	default:
		return true
	}
}

// detectPlatformDisplay - Unix/macOS display detection
func (e *DisplayEnv) detectPlatformDisplay() {
	// Check for Wayland first (preferred on Linux)
	if waylandDisplay := os.Getenv("WAYLAND_DISPLAY"); waylandDisplay != "" {
		e.HasDisplay = true
		e.DisplayType = "wayland"
		return
	}

	// Check for X11
	if display := os.Getenv("DISPLAY"); display != "" {
		e.HasDisplay = true
		e.DisplayType = "x11"
		return
	}

	// macOS: check if we have access to WindowServer
	if detectedOS == "darwin" {
		// On macOS, display is always available unless:
		// - Running over SSH
		// - Running as a LaunchDaemon (no GUI session)
		if !e.IsSSH && os.Getenv("__CFBundleIdentifier") != "" {
			e.HasDisplay = true
			e.DisplayType = "macos"
			return
		}
		// Check if WindowServer is accessible
		cmd := exec.Command("launchctl", "managername")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), "Aqua") {
				e.HasDisplay = true
				e.DisplayType = "macos"
				return
			}
		}
	}

	e.HasDisplay = false
	e.DisplayType = "none"
}
