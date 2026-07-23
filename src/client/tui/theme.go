package tui

import (
	"github.com/apimgr/pastebin/src/common/theme"
	"github.com/charmbracelet/lipgloss"
)

// TUITheme holds the color palette for the TUI.
type TUITheme struct {
	Background string
	Foreground string
	Primary    string
	Secondary  string
	Accent     string
	Error      string
	Success    string
	Warning    string
	Muted      string
}

// TUIStyles holds the pre-built lipgloss styles for the TUI.
type TUIStyles struct {
	Base     lipgloss.Style
	Title    lipgloss.Style
	Header   lipgloss.Style
	Selected lipgloss.Style
	Normal   lipgloss.Style
	Muted    lipgloss.Style
	Error    lipgloss.Style
	Success  lipgloss.Style
	Warning  lipgloss.Style
	Border   lipgloss.Style
	Help     lipgloss.Style
	Input    lipgloss.Style
}

// tuiThemeFromPalette maps the canonical theme.ThemePalette (single source
// of truth, src/common/theme/colors.go, AI.md 24320) onto the TUI's
// lipgloss-facing field set.
func tuiThemeFromPalette(p theme.ThemePalette) TUITheme {
	return TUITheme{
		Background: p.Background,
		Foreground: p.Foreground,
		Primary:    p.Primary,
		Secondary:  p.Secondary,
		Accent:     p.Accent,
		Error:      p.Error,
		Success:    p.Success,
		Warning:    p.Warning,
		Muted:      p.Muted,
	}
}

// darkTheme is the default dark color palette.
var darkTheme = tuiThemeFromPalette(theme.ThemePaletteDark)

// lightTheme is the light color palette.
var lightTheme = tuiThemeFromPalette(theme.ThemePaletteLight)

// CurrentTheme is the active theme used by the TUI. Defaults to dark.
var CurrentTheme = darkTheme

// StylesFromTheme builds a TUIStyles set from the given TUITheme.
func StylesFromTheme(theme TUITheme) TUIStyles {
	bg := lipgloss.Color(theme.Background)
	fg := lipgloss.Color(theme.Foreground)
	primary := lipgloss.Color(theme.Primary)
	secondary := lipgloss.Color(theme.Secondary)
	accent := lipgloss.Color(theme.Accent)
	errColor := lipgloss.Color(theme.Error)
	success := lipgloss.Color(theme.Success)
	warning := lipgloss.Color(theme.Warning)
	muted := lipgloss.Color(theme.Muted)

	return TUIStyles{
		Base: lipgloss.NewStyle().
			Background(bg).
			Foreground(fg),

		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary).
			Background(bg),

		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(accent).
			Background(bg),

		Selected: lipgloss.NewStyle().
			Foreground(bg).
			Background(primary).
			Bold(true),

		Normal: lipgloss.NewStyle().
			Foreground(fg).
			Background(bg),

		Muted: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),

		Error: lipgloss.NewStyle().
			Foreground(errColor).
			Background(bg),

		Success: lipgloss.NewStyle().
			Foreground(success).
			Background(bg),

		Warning: lipgloss.NewStyle().
			Foreground(warning).
			Background(bg),

		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(secondary).
			Background(bg),

		Help: lipgloss.NewStyle().
			Foreground(muted).
			Background(bg),

		Input: lipgloss.NewStyle().
			Foreground(fg).
			Background(muted).
			Padding(0, 1),
	}
}

// DarkTheme returns the dark color palette.
func DarkTheme() TUITheme { return darkTheme }

// LightTheme returns the light color palette.
func LightTheme() TUITheme { return lightTheme }
