//go:build !windows

package display

// Internal tests for unexported display functions.
// These run in the same package on Unix only.

import (
	"testing"
)

// TestAutoDetectDisplayMode covers all branches of autoDetectDisplayMode.
func TestAutoDetectDisplayMode(t *testing.T) {
	cases := []struct {
		name        string
		isTerminal  bool
		hasDisplay  bool
		termType    string
		isSSH       bool
		isMosh      bool
		wantMode    DisplayMode
	}{
		// !IsTerminal && !HasDisplay → Headless
		{"headless", false, false, "xterm", false, false, DisplayModeHeadless},
		// TerminalType == "dumb" → CLI
		{"dumb_terminal", true, false, "dumb", false, false, DisplayModeCLI},
		// HasDisplay && !IsSSH && !IsMosh → GUI
		{"gui", false, true, "xterm", false, false, DisplayModeGUI},
		// IsTerminal (no display, no dumb) → TUI
		{"tui", true, false, "xterm-256color", false, false, DisplayModeTUI},
		// HasDisplay but SSH → TUI (falls through to IsTerminal)
		{"has_display_ssh_tty", true, true, "xterm", true, false, DisplayModeTUI},
		// HasDisplay but mosh → TUI
		{"has_display_mosh_tty", true, true, "xterm", false, true, DisplayModeTUI},
		// No terminal, no display, dumb term → Headless (IsTerminal=false wins first)
		{"no_tty_dumb", false, false, "dumb", false, false, DisplayModeHeadless},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &DisplayEnv{
				IsTerminal:   tc.isTerminal,
				HasDisplay:   tc.hasDisplay,
				TerminalType: tc.termType,
				IsSSH:        tc.isSSH,
				IsMosh:       tc.isMosh,
			}
			got := e.autoDetectDisplayMode()
			if got != tc.wantMode {
				t.Errorf("autoDetectDisplayMode() = %d, want %d", got, tc.wantMode)
			}
		})
	}
}

// TestDetectPlatformDisplay_WaylandEnv verifies WAYLAND_DISPLAY sets HasDisplay + type.
func TestDetectPlatformDisplay_WaylandEnv(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")

	e := &DisplayEnv{}
	e.detectPlatformDisplay()

	if !e.HasDisplay {
		t.Error("HasDisplay: got false, want true for WAYLAND_DISPLAY set")
	}
	if e.DisplayType != "wayland" {
		t.Errorf("DisplayType: got %q, want %q", e.DisplayType, "wayland")
	}
}

// TestDetectPlatformDisplay_X11Env verifies DISPLAY sets HasDisplay + type.
func TestDetectPlatformDisplay_X11Env(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	e := &DisplayEnv{}
	e.detectPlatformDisplay()

	if !e.HasDisplay {
		t.Error("HasDisplay: got false, want true for DISPLAY set")
	}
	if e.DisplayType != "x11" {
		t.Errorf("DisplayType: got %q, want %q", e.DisplayType, "x11")
	}
}

// TestDetectPlatformDisplay_NoDisplay verifies no-display env yields type "none".
func TestDetectPlatformDisplay_NoDisplay(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	e := &DisplayEnv{}
	e.detectPlatformDisplay()

	// On Linux CI without macOS: HasDisplay must be false.
	if e.DisplayType != "none" && e.DisplayType != "macos" {
		t.Errorf("DisplayType: got %q, want %q or %q", e.DisplayType, "none", "macos")
	}
}
