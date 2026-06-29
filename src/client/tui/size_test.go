package tui

import (
	"testing"

	"github.com/apimgr/pastebin/src/common/terminal"
)

// Tests for size.go: GetLayoutConfig, sizeMode, helpLineForMode

func TestSizeModeFromDimensions(t *testing.T) {
	tests := []struct {
		cols int
		rows int
		want terminal.SizeMode
	}{
		{30, 8, terminal.SizeModeMicro},
		{39, 15, terminal.SizeModeMicro},
		{40, 9, terminal.SizeModeMicro},
		{50, 12, terminal.SizeModeMinimal},
		{59, 15, terminal.SizeModeMinimal},
		{60, 20, terminal.SizeModeCompact},
		{79, 23, terminal.SizeModeCompact},
		{80, 24, terminal.SizeModeStandard},
		{100, 30, terminal.SizeModeStandard},
		{119, 39, terminal.SizeModeStandard},
		{120, 40, terminal.SizeModeWide},
		{150, 50, terminal.SizeModeWide},
		{200, 60, terminal.SizeModeUltrawide},
		{300, 70, terminal.SizeModeUltrawide},
		{400, 80, terminal.SizeModeMassive},
		{500, 100, terminal.SizeModeMassive},
	}

	for _, tc := range tests {
		got := sizeMode(tc.cols, tc.rows)
		if got != tc.want {
			t.Errorf("sizeMode(%d, %d) = %v, want %v", tc.cols, tc.rows, got, tc.want)
		}
	}
}

func TestGetLayoutConfigMicro(t *testing.T) {
	cfg := GetLayoutConfig(terminal.SizeModeMicro)
	if cfg.MaxTableCols != 2 {
		t.Errorf("Micro MaxTableCols = %d, want 2", cfg.MaxTableCols)
	}
	if cfg.ShowBorder {
		t.Error("Micro ShowBorder should be false")
	}
	if cfg.ShowSidebar {
		t.Error("Micro ShowSidebar should be false")
	}
	if cfg.TruncateAt != 20 {
		t.Errorf("Micro TruncateAt = %d, want 20", cfg.TruncateAt)
	}
}

func TestGetLayoutConfigMinimal(t *testing.T) {
	cfg := GetLayoutConfig(terminal.SizeModeMinimal)
	if cfg.MaxTableCols != 3 {
		t.Errorf("Minimal MaxTableCols = %d, want 3", cfg.MaxTableCols)
	}
	if cfg.ShowBorder {
		t.Error("Minimal ShowBorder should be false")
	}
	if cfg.TruncateAt != 30 {
		t.Errorf("Minimal TruncateAt = %d, want 30", cfg.TruncateAt)
	}
}

func TestGetLayoutConfigCompact(t *testing.T) {
	cfg := GetLayoutConfig(terminal.SizeModeCompact)
	if cfg.MaxTableCols != 4 {
		t.Errorf("Compact MaxTableCols = %d, want 4", cfg.MaxTableCols)
	}
	if !cfg.ShowBorder {
		t.Error("Compact ShowBorder should be true")
	}
	if cfg.TruncateAt != 40 {
		t.Errorf("Compact TruncateAt = %d, want 40", cfg.TruncateAt)
	}
}

func TestGetLayoutConfigStandard(t *testing.T) {
	cfg := GetLayoutConfig(terminal.SizeModeStandard)
	if cfg.MaxTableCols != 6 {
		t.Errorf("Standard MaxTableCols = %d, want 6", cfg.MaxTableCols)
	}
	if !cfg.ShowBorder {
		t.Error("Standard ShowBorder should be true")
	}
	if cfg.ShowSidebar {
		t.Error("Standard ShowSidebar should be false")
	}
	if cfg.TruncateAt != 60 {
		t.Errorf("Standard TruncateAt = %d, want 60", cfg.TruncateAt)
	}
}

func TestGetLayoutConfigWideAndAbove(t *testing.T) {
	modes := []terminal.SizeMode{
		terminal.SizeModeWide,
		terminal.SizeModeUltrawide,
		terminal.SizeModeMassive,
	}
	for _, mode := range modes {
		cfg := GetLayoutConfig(mode)
		if cfg.MaxTableCols != 10 {
			t.Errorf("GetLayoutConfig(%v) MaxTableCols = %d, want 10", mode, cfg.MaxTableCols)
		}
		if !cfg.ShowBorder {
			t.Errorf("GetLayoutConfig(%v) ShowBorder should be true", mode)
		}
		if !cfg.ShowSidebar {
			t.Errorf("GetLayoutConfig(%v) ShowSidebar should be true", mode)
		}
		if cfg.TruncateAt != 100 {
			t.Errorf("GetLayoutConfig(%v) TruncateAt = %d, want 100", mode, cfg.TruncateAt)
		}
	}
}

func TestHelpLineForModeMicro(t *testing.T) {
	line := helpLineForMode(terminal.SizeModeMicro)
	if line != "?:help q:quit" {
		t.Errorf("helpLineForMode(Micro) = %q, want %q", line, "?:help q:quit")
	}
}

func TestHelpLineForModeMinimal(t *testing.T) {
	line := helpLineForMode(terminal.SizeModeMinimal)
	if line != "?:help q:quit" {
		t.Errorf("helpLineForMode(Minimal) = %q, want %q", line, "?:help q:quit")
	}
}

func TestHelpLineForModeCompact(t *testing.T) {
	line := helpLineForMode(terminal.SizeModeCompact)
	expected := "↑↓:nav │ enter:open │ /:search │ ?:help │ q:quit"
	if line != expected {
		t.Errorf("helpLineForMode(Compact) = %q, want %q", line, expected)
	}
}

func TestHelpLineForModeStandard(t *testing.T) {
	line := helpLineForMode(terminal.SizeModeStandard)
	expected := "↑↓/jk:nav │ enter:open │ /:search │ r:refresh │ n:new │ d:delete │ ?:help │ q:quit"
	if line != expected {
		t.Errorf("helpLineForMode(Standard) = %q, want %q", line, expected)
	}
}
