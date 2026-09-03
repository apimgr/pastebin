package display

// tuiSymbolSet holds the status/decoration glyphs used by CLI/TUI output
// (PART 32, AI.md "Consistent Icons and Symbols", line 44564).
type tuiSymbolSet struct {
	Success string
	Error   string
	Warning string
	Info    string
	Arrow   string
	Check   string
	Cross   string
	Bullet  string
	Spinner []string
}

// TUISymbols - Unicode symbols that work across terminals.
var TUISymbols = tuiSymbolSet{
	Success: "✓",
	Error:   "✗",
	Warning: "⚠",
	Info:    "ℹ",
	Arrow:   "→",
	Check:   "☑",
	Cross:   "☒",
	Bullet:  "•",
	Spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

// TUISymbolsASCII - fallback for non-Unicode terminals.
var TUISymbolsASCII = tuiSymbolSet{
	Success: "[OK]",
	Error:   "[ERR]",
	Warning: "[WARN]",
	Info:    "[INFO]",
	Arrow:   "->",
	Check:   "[x]",
	Cross:   "[ ]",
	Bullet:  "*",
	Spinner: []string{"|", "/", "-", "\\"},
}

// GetTUISymbols returns the Unicode symbol set when the environment supports
// it, otherwise the ASCII fallback set.
func GetTUISymbols(env DisplayEnv) interface{} {
	if env.SupportsUnicode() {
		return TUISymbols
	}
	return TUISymbolsASCII
}
