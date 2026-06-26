package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpEntry is a single keybinding row in the help overlay.
type helpEntry struct {
	key  string
	desc string
}

// allHelpEntries lists every keybinding shown in the help overlay.
var allHelpEntries = []helpEntry{
	{"j / ↓", "Next item"},
	{"k / ↑", "Previous item"},
	{"g / G", "Top / Bottom"},
	{"Enter", "Open paste"},
	{"/", "Search"},
	{"r", "Refresh"},
	{"n", "New paste"},
	{"d", "Delete paste"},
	{"?", "Help"},
	{"q", "Quit"},
	{"Esc", "Back"},
}

// viewHelp renders the help overlay modal.
func viewHelp(styles TUIStyles, width, height int) string {
	title := styles.Title.Render("Keyboard shortcuts")
	divider := styles.Muted.Render(strings.Repeat("─", 28))

	var rows []string
	rows = append(rows, title)
	rows = append(rows, divider)
	for _, e := range allHelpEntries {
		key := lipgloss.NewStyle().
			Foreground(lipgloss.Color(CurrentTheme.Accent)).
			Width(12).
			Render(e.key)
		desc := styles.Normal.Render(e.desc)
		rows = append(rows, key+desc)
	}
	rows = append(rows, "")
	rows = append(rows, styles.Muted.Render("Press ? or Esc to close"))

	content := strings.Join(rows, "\n")

	box := styles.Border.
		Padding(1, 2).
		Render(content)

	// Center the box horizontally.
	boxLines := strings.Split(box, "\n")
	boxWidth := 0
	for _, l := range boxLines {
		if len(l) > boxWidth {
			boxWidth = len(l)
		}
	}

	leftPad := (width - boxWidth) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	pad := strings.Repeat(" ", leftPad)

	var centered []string
	for _, l := range boxLines {
		centered = append(centered, pad+l)
	}
	return strings.Join(centered, "\n")
}
