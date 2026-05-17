package terminal

import (
	"os"

	"golang.org/x/term"
)

// SizeMode represents the terminal size category
type SizeMode int

const (
	SizeModeMicro     SizeMode = iota // <40 cols or <10 rows
	SizeModeMinimal                    // 40-59 cols or 10-15 rows
	SizeModeCompact                    // 60-79 cols or 16-23 rows
	SizeModeStandard                   // 80-119 cols and 24-39 rows
	SizeModeWide                       // 120-199 cols and 40-59 rows
	SizeModeUltrawide                  // 200-399 cols and 60-79 rows
	SizeModeMassive                    // 400+ cols and 80+ rows
)

// TerminalSize holds the detected terminal dimensions and mode
type TerminalSize struct {
	Cols int
	Rows int
	Mode SizeMode
}

// GetTerminalSize detects the current terminal size and categorizes it
func GetTerminalSize() TerminalSize {
	cols, rows, _ := term.GetSize(int(os.Stdout.Fd()))
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	return TerminalSize{
		Cols: cols,
		Rows: rows,
		Mode: calculateMode(cols, rows),
	}
}

func calculateMode(cols, rows int) SizeMode {
	switch {
	case cols < 40 || rows < 10:
		return SizeModeMicro
	case cols < 60 || rows < 16:
		return SizeModeMinimal
	case cols < 80 || rows < 24:
		return SizeModeCompact
	case cols < 120 || rows < 40:
		return SizeModeStandard
	case cols < 200 || rows < 60:
		return SizeModeWide
	case cols < 400 || rows < 80:
		return SizeModeUltrawide
	default:
		return SizeModeMassive
	}
}

// ShowASCIIArt returns true if terminal is wide enough for ASCII art
func (s SizeMode) ShowASCIIArt() bool { return s >= SizeModeStandard }

// ShowBorders returns true if terminal is wide enough for bordered output
func (s SizeMode) ShowBorders() bool { return s >= SizeModeCompact }

// ShowSidebar returns true if terminal is wide enough for sidebars
func (s SizeMode) ShowSidebar() bool { return s >= SizeModeWide }

// ShowIcons returns true if terminal is wide enough for icons
func (s SizeMode) ShowIcons() bool { return s >= SizeModeMinimal }
