package terminal

// SymbolSet holds Unicode box-drawing characters and their ASCII fallbacks.
// The appropriate set is selected by NewSymbolSet based on terminal capabilities.
type SymbolSet struct {
	// Box-drawing corners
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	// Box-drawing edges
	Horizontal string
	Vertical   string
	// Box-drawing T-junctions
	TLeft   string
	TRight  string
	TTop    string
	TBottom string
	// Box-drawing cross
	Cross string
	// Arrows
	ArrowRight string
	ArrowLeft  string
	ArrowUp    string
	ArrowDown  string
	// Status indicators
	CheckMark string
	Cross2    string
	Bullet    string
	Ellipsis  string
	// Tree drawing
	TreeBranch string
	TreeLast   string
	TreePipe   string
}

// unicodeSymbols is the full Unicode box-drawing symbol set.
var unicodeSymbols = SymbolSet{
	TopLeft:     "┌",
	TopRight:    "┐",
	BottomLeft:  "└",
	BottomRight: "┘",
	Horizontal:  "─",
	Vertical:    "│",
	TLeft:       "├",
	TRight:      "┤",
	TTop:        "┬",
	TBottom:     "┴",
	Cross:       "┼",
	ArrowRight:  "→",
	ArrowLeft:   "←",
	ArrowUp:     "↑",
	ArrowDown:   "↓",
	CheckMark:   "✓",
	Cross2:      "✗",
	Bullet:      "•",
	Ellipsis:    "…",
	TreeBranch:  "├─",
	TreeLast:    "└─",
	TreePipe:    "│ ",
}

// asciiSymbols is the ASCII-only fallback symbol set for TERM=dumb and limited terminals.
var asciiSymbols = SymbolSet{
	TopLeft:     "+",
	TopRight:    "+",
	BottomLeft:  "+",
	BottomRight: "+",
	Horizontal:  "-",
	Vertical:    "|",
	TLeft:       "+",
	TRight:      "+",
	TTop:        "+",
	TBottom:     "+",
	Cross:       "+",
	ArrowRight:  ">",
	ArrowLeft:   "<",
	ArrowUp:     "^",
	ArrowDown:   "v",
	CheckMark:   "[ok]",
	Cross2:      "[x]",
	Bullet:      "*",
	Ellipsis:    "...",
	TreeBranch:  "+-",
	TreeLast:    "\\-",
	TreePipe:    "| ",
}

// NewSymbolSet returns the Unicode symbol set when the terminal supports it,
// or the ASCII fallback set for TERM=dumb and non-Unicode terminals.
func NewSymbolSet(dumb bool) SymbolSet {
	if dumb {
		return asciiSymbols
	}
	return unicodeSymbols
}
