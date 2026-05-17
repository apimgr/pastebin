package banner

import "strings"

// asciiArt returns a simple ASCII art representation of the app name.
// For small names only; returns empty string if name is too long to fit in maxWidth.
func asciiArt(name string, maxWidth int) string {
	if len(name) == 0 || maxWidth < 20 {
		return ""
	}

	// Simple block-letter renderer using standard ASCII box chars
	upper := strings.ToUpper(name)
	line := strings.Repeat("#", len(upper)*2+1)
	if len(line) > maxWidth {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("  " + line + "\n")
	sb.WriteString("  # " + strings.Join(strings.Split(upper, ""), " ") + " #\n")
	sb.WriteString("  " + line + "\n")
	return sb.String()
}
