//go:build gui && linux

package gui

import (
	"os/exec"
	"strings"
)

// systemPrefersDark detects GNOME's color-scheme preference via gsettings
// (AI.md "System Theme Detection"). Any failure (gsettings missing, no
// desktop session, non-GNOME DE) is treated as "no preference detected"
// and falls back to the dark default, never an error.
func systemPrefersDark() bool {
	out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
	if err != nil {
		return true
	}
	lower := strings.ToLower(string(out))
	if strings.Contains(lower, "light") {
		return false
	}
	return true
}
