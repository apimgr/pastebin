package tui

import "github.com/apimgr/pastebin/src/common/terminal"

// LayoutConfig describes how the list view should be rendered for a given SizeMode.
type LayoutConfig struct {
	MaxTableCols int
	ShowBorder   bool
	ShowSidebar  bool
	TruncateAt   int
}

// GetLayoutConfig returns the LayoutConfig for the given SizeMode.
// Implements the PART 32 GetLayoutConfig table.
func GetLayoutConfig(mode terminal.SizeMode) LayoutConfig {
	switch mode {
	case terminal.SizeModeMicro:
		return LayoutConfig{MaxTableCols: 2, ShowBorder: false, ShowSidebar: false, TruncateAt: 20}
	case terminal.SizeModeMinimal:
		return LayoutConfig{MaxTableCols: 3, ShowBorder: false, ShowSidebar: false, TruncateAt: 30}
	case terminal.SizeModeCompact:
		return LayoutConfig{MaxTableCols: 4, ShowBorder: true, ShowSidebar: false, TruncateAt: 40}
	case terminal.SizeModeStandard:
		return LayoutConfig{MaxTableCols: 6, ShowBorder: true, ShowSidebar: false, TruncateAt: 60}
	default:
		// Wide, Ultrawide, Massive — all get full columns.
		return LayoutConfig{MaxTableCols: 10, ShowBorder: true, ShowSidebar: true, TruncateAt: 100}
	}
}

// sizeMode derives a SizeMode from raw terminal dimensions, mirroring the
// logic in the terminal package without depending on the unexported calculateMode.
func sizeMode(cols, rows int) terminal.SizeMode {
	switch {
	case cols < 40 || rows < 10:
		return terminal.SizeModeMicro
	case cols < 60 || rows < 16:
		return terminal.SizeModeMinimal
	case cols < 80 || rows < 24:
		return terminal.SizeModeCompact
	case cols < 120 || rows < 40:
		return terminal.SizeModeStandard
	case cols < 200 || rows < 60:
		return terminal.SizeModeWide
	case cols < 400 || rows < 80:
		return terminal.SizeModeUltrawide
	default:
		return terminal.SizeModeMassive
	}
}

// helpLineForMode returns the condensed help line appropriate for a given SizeMode.
func helpLineForMode(mode terminal.SizeMode) string {
	switch {
	case mode <= terminal.SizeModeMinimal:
		return "?:help q:quit"
	case mode == terminal.SizeModeCompact:
		return "↑↓:nav │ enter:open │ /:search │ ?:help │ q:quit"
	default:
		return "↑↓/jk:nav │ enter:open │ /:search │ r:refresh │ n:new │ d:delete │ ?:help │ q:quit"
	}
}
