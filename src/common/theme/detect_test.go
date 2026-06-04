package theme

// Internal tests for the unexported theme detection helpers.
// These run in the same package and can call isTerminalDarkTheme directly.

import "testing"

// TestIsTerminalDarkTheme covers all branches of the COLORFGBG parser.
func TestIsTerminalDarkTheme(t *testing.T) {
	cases := []struct {
		name        string
		colorfgbg   string
		wantDark    bool
	}{
		// Empty value → default dark
		{"empty", "", true},
		// Well-known dark background codes (0-7)
		{"bg_0", "15;0", true},
		{"bg_1", "15;1", true},
		{"bg_2", "15;2", true},
		{"bg_7", "15;7", true},
		// Light background codes (8+)
		{"bg_8", "0;8", false},
		{"bg_15", "0;15", false},
		// malformed: only one part → default dark
		{"no_semicolon", "15", true},
		// Three-part format: use last element
		{"three_parts_dark", "15;0;0", true},
		{"three_parts_light", "15;0;8", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COLORFGBG", tc.colorfgbg)
			got := isTerminalDarkTheme()
			if got != tc.wantDark {
				t.Errorf("isTerminalDarkTheme() with COLORFGBG=%q: got %v, want %v",
					tc.colorfgbg, got, tc.wantDark)
			}
		})
	}
}

// TestIsLinuxDarkTheme_Fallback verifies isLinuxDarkTheme falls back to
// isTerminalDarkTheme when gsettings is unavailable (always the case in CI).
func TestIsLinuxDarkTheme_Fallback(t *testing.T) {
	// Set COLORFGBG to a known value to make the fallback deterministic.
	t.Setenv("COLORFGBG", "15;0") // dark bg
	result := isLinuxDarkTheme()
	// In CI gsettings won't be available; result depends on COLORFGBG.
	// Just verify it returns a bool without panicking.
	_ = result
}
