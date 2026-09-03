package theme

// ThemePalette holds all color values for a UI theme
type ThemePalette struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Surface    string `json:"surface"`
	SurfaceAlt string `json:"surface_alt"`
	Border     string `json:"border"`
	Muted      string `json:"muted"`
}

// ThemePaletteDark is the default dark theme palette
var ThemePaletteDark = ThemePalette{
	Background: "#1a1b26", Foreground: "#c0caf5",
	Primary: "#7aa2f7", Secondary: "#9ece6a", Accent: "#bb9af7",
	Success: "#9ece6a", Warning: "#e0af68", Error: "#f7768e", Info: "#7dcfff",
	Surface: "#24283b", SurfaceAlt: "#1f2335", Border: "#414868", Muted: "#565f89",
}

// ThemePaletteLight is the light theme palette
var ThemePaletteLight = ThemePalette{
	Background: "#ffffff", Foreground: "#1a1b26",
	Primary: "#2e7de9", Secondary: "#587539", Accent: "#7847bd",
	Success: "#587539", Warning: "#8c6c3e", Error: "#c64343", Info: "#007197",
	Surface: "#f5f5f5", SurfaceAlt: "#e9e9ec", Border: "#c0caf5", Muted: "#6172b0",
}

// GetThemePalette returns a palette for the given theme mode string.
// Valid values: "dark", "light", "auto". Defaults to dark.
func GetThemePalette(themeMode string) ThemePalette {
	switch themeMode {
	case "light":
		return ThemePaletteLight
	case "auto":
		if IsSystemDarkTheme() {
			return ThemePaletteDark
		}
		return ThemePaletteLight
	default:
		return ThemePaletteDark
	}
}

// TerminalPalette holds ANSI 16-color indices (0-15) for CLI/TUI — never
// the literal hex ThemePalette. lipgloss.Color() and the ESC[38;5;{n}m
// escape both accept these indices directly (AI.md PART 16 "CLI/TUI Color
// Mapping", PART 32 "CLI/TUI/GUI Theming").
type TerminalPalette struct {
	Foreground string `json:"foreground"`
	Muted      string `json:"muted"`
	Primary    string `json:"primary"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Border     string `json:"border"`
}

// TerminalPaletteDark is the ANSI-mapped palette for dark-themed terminals.
var TerminalPaletteDark = TerminalPalette{
	Foreground: "15", Muted: "7", Primary: "13",
	Success: "10", Warning: "11", Error: "9", Info: "12", Border: "13",
}

// TerminalPaletteLight is the ANSI-mapped palette for light-themed terminals.
var TerminalPaletteLight = TerminalPalette{
	Foreground: "0", Muted: "8", Primary: "4",
	Success: "2", Warning: "3", Error: "1", Info: "4", Border: "4",
}

// GetTerminalPalette returns the ANSI-mapped palette for the given theme
// mode string. Valid values: "dark", "light", "auto". Defaults to dark.
func GetTerminalPalette(themeMode string) TerminalPalette {
	switch themeMode {
	case "light":
		return TerminalPaletteLight
	case "auto":
		if IsSystemDarkTheme() {
			return TerminalPaletteDark
		}
		return TerminalPaletteLight
	default:
		return TerminalPaletteDark
	}
}
