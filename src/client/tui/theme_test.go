package tui

import (
	"testing"
)

// Tests for theme.go: TUIStyles, StylesFromTerminalPalette, DarkTheme, LightTheme

func TestDarkThemeReturnsExpectedPalette(t *testing.T) {
	palette := DarkTheme()
	if palette.Foreground != "15" {
		t.Errorf("DarkTheme().Foreground = %q, want %q", palette.Foreground, "15")
	}
	if palette.Primary == "" {
		t.Error("DarkTheme().Primary is empty")
	}
	if palette.Error == "" {
		t.Error("DarkTheme().Error is empty")
	}
}

func TestLightThemeReturnsExpectedPalette(t *testing.T) {
	palette := LightTheme()
	if palette.Foreground != "0" {
		t.Errorf("LightTheme().Foreground = %q, want %q", palette.Foreground, "0")
	}
	if palette.Primary == "" {
		t.Error("LightTheme().Primary is empty")
	}
}

func TestStylesFromTerminalPaletteProducesNonZeroStyles(t *testing.T) {
	palette := DarkTheme()
	styles := StylesFromTerminalPalette(palette)

	// Check that all style fields are initialized (non-zero render output)
	testCases := []struct {
		name  string
		style func() string
	}{
		{"Base", func() string { return styles.Base.Render("x") }},
		{"Title", func() string { return styles.Title.Render("x") }},
		{"Header", func() string { return styles.Header.Render("x") }},
		{"Selected", func() string { return styles.Selected.Render("x") }},
		{"Normal", func() string { return styles.Normal.Render("x") }},
		{"Muted", func() string { return styles.Muted.Render("x") }},
		{"Error", func() string { return styles.Error.Render("x") }},
		{"Success", func() string { return styles.Success.Render("x") }},
		{"Warning", func() string { return styles.Warning.Render("x") }},
		{"Border", func() string { return styles.Border.Render("x") }},
		{"Help", func() string { return styles.Help.Render("x") }},
		{"Input", func() string { return styles.Input.Render("x") }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			out := tc.style()
			if out == "" {
				t.Errorf("%s.Render returned empty string", tc.name)
			}
		})
	}
}

func TestStylesFromTerminalPaletteLightTheme(t *testing.T) {
	palette := LightTheme()
	styles := StylesFromTerminalPalette(palette)
	out := styles.Title.Render("test")
	if out == "" {
		t.Error("StylesFromTerminalPalette(LightTheme()) produced empty Title style")
	}
}

func TestCurrentThemeDefaultsToDark(t *testing.T) {
	// CurrentTheme should be initialized to darkTheme
	if CurrentTheme.Foreground != darkTheme.Foreground {
		t.Errorf("CurrentTheme.Foreground = %q, want %q", CurrentTheme.Foreground, darkTheme.Foreground)
	}
}
