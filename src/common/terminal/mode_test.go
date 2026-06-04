package terminal

// Internal tests for the unexported calculateMode function.
// These run in the same package and can access unexported symbols.

import "testing"

// TestCalculateMode covers all 7 branches of the calculateMode switch.
func TestCalculateMode(t *testing.T) {
	cases := []struct {
		name string
		cols int
		rows int
		want SizeMode
	}{
		// micro: cols < 40 OR rows < 10
		{"cols_too_small", 30, 24, SizeModeMicro},
		{"rows_too_small", 80, 5, SizeModeMicro},
		// minimal: cols < 60 OR rows < 16
		{"cols_minimal", 50, 24, SizeModeMinimal},
		{"rows_minimal", 80, 12, SizeModeMinimal},
		// compact: cols < 80 OR rows < 24
		{"cols_compact", 70, 30, SizeModeCompact},
		{"rows_compact", 100, 20, SizeModeCompact},
		// standard: cols < 120 OR rows < 40 (both must be met for standard)
		{"standard", 80, 24, SizeModeStandard},
		{"standard_wide_cols", 110, 35, SizeModeStandard},
		// wide: cols < 200 OR rows < 60
		{"wide", 150, 45, SizeModeWide},
		// ultrawide: cols < 400 OR rows < 80
		{"ultrawide", 250, 65, SizeModeUltrawide},
		// massive: 400+ cols AND 80+ rows
		{"massive", 400, 80, SizeModeMassive},
		{"massive_large", 500, 100, SizeModeMassive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateMode(tc.cols, tc.rows)
			if got != tc.want {
				t.Errorf("calculateMode(%d, %d) = %d, want %d", tc.cols, tc.rows, got, tc.want)
			}
		})
	}
}
