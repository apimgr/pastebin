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
func allHelpEntries() []helpEntry {
	return []helpEntry{
		{"j / ↓", t("help_next_item")},
		{"k / ↑", t("help_previous_item")},
		{"g / G", t("help_top_bottom")},
		{"Enter", t("help_open_paste")},
		{"/", t("help_search")},
		{"r", t("help_refresh")},
		{"n", t("help_new_paste")},
		{"d", t("help_delete_paste")},
		{"?", t("help_help")},
		{"q", t("help_quit")},
		{"Esc", t("help_back")},
	}
}

// viewHelp renders the help overlay modal.
func viewHelp(styles TUIStyles, width, height int) string {
	title := styles.Title.Render(t("help_title"))
	divider := styles.Muted.Render(strings.Repeat("─", 28))

	var rows []string
	rows = append(rows, title)
	rows = append(rows, divider)
	for _, e := range allHelpEntries() {
		key := lipgloss.NewStyle().
			Foreground(lipgloss.Color(CurrentTheme.Primary)).
			Width(12).
			Render(e.key)
		desc := styles.Normal.Render(e.desc)
		rows = append(rows, key+desc)
	}
	rows = append(rows, "")
	rows = append(rows, styles.Muted.Render(t("help_footer")))

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
