//go:build gui && !freebsd && !netbsd && !openbsd

package gui

import (
	"github.com/gogpu/ui/theme"
	"github.com/gogpu/ui/theme/material3"
	"github.com/gogpu/ui/widget"
)

// seedColor is the brand accent used to derive the Material 3 color scheme,
// matching the accent previously hardcoded in the old Gio dark palette
// (ContrastBg 0x4a4ad6).
const seedColor = 0x4a4ad6

// newTheme resolves the "dark"/"light"/"auto" cli.yml gui theme setting to a
// gogpu/ui Material 3 theme. Per AI.md's "GUI Theming" rule, native GUI does
// not consume the literal hex ThemePalette used by Web/Swagger/GraphiQL — it
// only detects light/dark and lets the toolkit derive its own color scheme
// from a single seed color.
func newTheme(name string) *theme.Theme {
	seed := widget.Hex(seedColor)
	if resolveDarkMode(name) {
		return material3.NewDark(seed).AsTheme()
	}
	return material3.New(seed).AsTheme()
}

// resolveDarkMode decides whether the dark color scheme should be used,
// honoring an explicit "dark"/"light" override and otherwise falling back
// to the OS-reported preference (systemPrefersDark, per-OS in theme_*.go).
// Defaults to dark if detection fails, matching the CLI/TUI/Web default.
func resolveDarkMode(name string) bool {
	switch name {
	case "light":
		return false
	case "dark":
		return true
	default:
		return systemPrefersDark()
	}
}
