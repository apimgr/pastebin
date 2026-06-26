package tui

import "github.com/charmbracelet/lipgloss"

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

// darkTheme is the default dark color palette.
var darkTheme = TUITheme{
	Background: "#282a36",
	Foreground: "#f8f8f2",
	Primary:    "#bd93f9",
	Secondary:  "#6272a4",
	Accent:     "#8be9fd",
	Error:      "#ff5555",
	Success:    "#50fa7b",
	Warning:    "#f1fa8c",
	Muted:      "#44475a",
}

// lightTheme is the light color palette.
var lightTheme = TUITheme{
	Background: "#ffffff",
	Foreground: "#282a36",
	Primary:    "#6c5ce7",
	Secondary:  "#636e72",
	Accent:     "#0984e3",
	Error:      "#d63031",
	Success:    "#00b894",
	Warning:    "#fdcb6e",
	Muted:      "#dfe6e9",
}

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
