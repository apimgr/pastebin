package display

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// DisplayMode - UI display mode (NOT app mode)
type DisplayMode int

const (
	// No display, no TTY
	DisplayModeHeadless DisplayMode = iota
	// Command-line only (piped or command provided)
	DisplayModeCLI
	// Terminal UI (interactive terminal)
	DisplayModeTUI
	// Native graphical UI
	DisplayModeGUI
)

// DisplayEnv - detected display environment
type DisplayEnv struct {
	Mode DisplayMode
	// X11, Wayland, Windows, macOS display
	HasDisplay bool
	// "x11", "wayland", "windows", "macos", "none"
	DisplayType string
	// stdout is a TTY
	IsTerminal bool
	// Running over SSH
	IsSSH bool
	// Running over mosh
	IsMosh bool
	// Running in screen/tmux
	IsScreen bool
	// TERM value
	TerminalType string
	// Terminal columns (0 if no terminal)
	Cols int
	// Terminal rows (0 if no terminal)
	Rows int
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
