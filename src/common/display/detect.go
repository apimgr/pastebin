package display

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// DisplayMode - UI display mode (NOT app mode)
type DisplayMode int

const (
	DisplayModeHeadless DisplayMode = iota // No display, no TTY
	DisplayModeCLI                          // Command-line only (piped or command provided)
	DisplayModeTUI                          // Terminal UI (interactive terminal)
	DisplayModeGUI                          // Native graphical UI
)

// DisplayEnv - detected display environment
type DisplayEnv struct {
	Mode         DisplayMode
	HasDisplay   bool   // X11, Wayland, Windows, macOS display
	DisplayType  string // "x11", "wayland", "windows", "macos", "none"
	IsTerminal   bool   // stdout is a TTY
	IsSSH        bool   // Running over SSH
	IsMosh       bool   // Running over mosh
	IsScreen     bool   // Running in screen/tmux
	TerminalType string // TERM value
	Cols         int    // Terminal columns (0 if no terminal)
	Rows         int    // Terminal rows (0 if no terminal)
}

// DetectDisplayEnv - auto-detect display environment
func DetectDisplayEnv() DisplayEnv {
	env := DisplayEnv{}

	// Terminal detection
	env.IsTerminal = term.IsTerminal(int(os.Stdout.Fd()))
	if env.IsTerminal {
		env.Cols, env.Rows, _ = term.GetSize(int(os.Stdout.Fd()))
	}
	env.TerminalType = os.Getenv("TERM")

	// Remote session detection
	env.IsSSH = os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != ""
	env.IsMosh = os.Getenv("MOSH") != "" || strings.Contains(os.Getenv("TERM"), "mosh")
	env.IsScreen = os.Getenv("STY") != "" || os.Getenv("TMUX") != ""

	// Platform-specific display detection
	env.detectPlatformDisplay()

	// Auto-detect display mode
	env.Mode = env.autoDetectDisplayMode()

	return env
}

// autoDetectDisplayMode - determine display mode from environment
func (e *DisplayEnv) autoDetectDisplayMode() DisplayMode {
	if !e.IsTerminal && !e.HasDisplay {
		return DisplayModeHeadless
	}
	// TERM=dumb: force CLI mode (no TUI, no ANSI escapes)
	if e.TerminalType == "dumb" {
		return DisplayModeCLI
	}
	if e.HasDisplay && !e.IsSSH && !e.IsMosh {
		return DisplayModeGUI
	}
	if e.IsTerminal {
		return DisplayModeTUI
	}
	return DisplayModeCLI
}

// IsDumbTerminal - check if running in dumb terminal (no ANSI support)
func (e *DisplayEnv) IsDumbTerminal() bool {
	return e.TerminalType == "dumb"
}

// CanUseANSI checks if ANSI escape codes can be used
func CanUseANSI(env *DisplayEnv) bool {
	if env.IsDumbTerminal() {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return env.IsTerminal
}

// Helper methods with clear names
func (e DisplayEnv) IsAutoDetectDisplayModeGUI() bool      { return e.Mode == DisplayModeGUI }
func (e DisplayEnv) IsAutoDetectDisplayModeTUI() bool      { return e.Mode == DisplayModeTUI }
func (e DisplayEnv) IsAutoDetectDisplayModeCLI() bool      { return e.Mode == DisplayModeCLI }
func (e DisplayEnv) IsAutoDetectDisplayModeHeadless() bool { return e.Mode == DisplayModeHeadless }
