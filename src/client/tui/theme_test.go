package tui

import (
	"testing"
)

// Tests for theme.go: TUITheme, TUIStyles, StylesFromTheme, DarkTheme, LightTheme

func TestDarkThemeReturnsExpectedPalette(t *testing.T) {
	theme := DarkTheme()
	if theme.Background != "#282a36" {
		t.Errorf("DarkTheme().Background = %q, want %q", theme.Background, "#282a36")
	}
	if theme.Foreground != "#f8f8f2" {
		t.Errorf("DarkTheme().Foreground = %q, want %q", theme.Foreground, "#f8f8f2")
	}
	if theme.Primary == "" {
		t.Error("DarkTheme().Primary is empty")
	}
	if theme.Error == "" {
		t.Error("DarkTheme().Error is empty")
	}
}

func TestLightThemeReturnsExpectedPalette(t *testing.T) {
	theme := LightTheme()
	if theme.Background != "#ffffff" {
		t.Errorf("LightTheme().Background = %q, want %q", theme.Background, "#ffffff")
	}
	if theme.Foreground != "#282a36" {
		t.Errorf("LightTheme().Foreground = %q, want %q", theme.Foreground, "#282a36")
	}
	if theme.Primary == "" {
		t.Error("LightTheme().Primary is empty")
	}
}

func TestStylesFromThemeProducesNonZeroStyles(t *testing.T) {
	theme := DarkTheme()
	styles := StylesFromTheme(theme)

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

func TestStylesFromThemeLightTheme(t *testing.T) {
	theme := LightTheme()
	styles := StylesFromTheme(theme)
	out := styles.Title.Render("test")
	if out == "" {
		t.Error("StylesFromTheme(LightTheme()) produced empty Title style")
	}
}

func TestCurrentThemeDefaultsToDark(t *testing.T) {
	// CurrentTheme should be initialized to darkTheme
	if CurrentTheme.Background != darkTheme.Background {
		t.Errorf("CurrentTheme.Background = %q, want %q", CurrentTheme.Background, darkTheme.Background)
	}
}
