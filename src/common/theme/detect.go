package theme

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// runtimeGOOS holds the current OS identifier — overridable in tests to exercise
// OS-specific branches without running on the target OS.
var runtimeGOOS = runtime.GOOS

// IsSystemDarkTheme detects whether the system is using a dark theme.
// Returns true for dark, false for light. Defaults to dark on failure.
func IsSystemDarkTheme() bool {
	switch runtimeGOOS {
	case "linux":
		return isLinuxDarkTheme()
	case "darwin":
		return isMacOSDarkTheme()
	case "windows":
		return isWindowsDarkTheme()
	default:
		// Check COLORFGBG env var (fg;bg where low bg = dark)
		return isTerminalDarkTheme()
	}
}

// isLinuxDarkTheme checks GNOME color-scheme setting
func isLinuxDarkTheme() bool {
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
	out, err := cmd.Output()
	if err != nil {
		return isTerminalDarkTheme()
	}
	return strings.Contains(strings.ToLower(string(out)), "dark")
}

// isMacOSDarkTheme checks macOS appearance setting
func isMacOSDarkTheme() bool {
	cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
	out, err := cmd.Output()
	if err != nil {
		// Command fails when light mode is active (no value set)
		return false
	}
	return strings.TrimSpace(string(out)) == "Dark"
}

// isWindowsDarkTheme checks the Windows registry for dark mode
func isWindowsDarkTheme() bool {
	cmd := exec.Command("reg", "query",
		`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		"/v", "AppsUseLightTheme")
	out, err := cmd.Output()
	if err != nil {
		// default dark
		return true
	}
	// AppsUseLightTheme = 0 means dark mode
	return strings.Contains(string(out), "0x0")
}

// isTerminalDarkTheme uses COLORFGBG to guess theme
func isTerminalDarkTheme() bool {
	colorfgbg := os.Getenv("COLORFGBG")
	if colorfgbg == "" {
		// default to dark
		return true
	}
	// Format: "fg;bg" where bg < 8 typically means dark background
	parts := strings.Split(colorfgbg, ";")
	if len(parts) < 2 {
		return true
	}
	bg := strings.TrimSpace(parts[len(parts)-1])
	// Common dark background colors: 0-7
	switch bg {
	case "0", "1", "2", "3", "4", "5", "6", "7":
		return true
	default:
		return false
	}
}
