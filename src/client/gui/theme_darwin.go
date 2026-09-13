//go:build gui && darwin

package gui

import (
	"os/exec"
	"strings"
)

// systemPrefersDark detects macOS dark mode via `defaults read -g
// AppleInterfaceStyle` (AI.md "System Theme Detection"). The key is unset
// entirely in light mode, which `defaults` reports as a non-zero exit —
// treated as "light", not an error.
func systemPrefersDark() bool {
	out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), "dark")
}
