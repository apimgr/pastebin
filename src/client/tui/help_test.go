package tui

import (
	"strings"
	"testing"
)

// Tests for help.go: viewHelp, allHelpEntries

func TestAllHelpEntriesNotEmpty(t *testing.T) {
	if len(allHelpEntries) == 0 {
		t.Fatal("allHelpEntries should not be empty")
	}
}

func TestAllHelpEntriesHaveKeysAndDescriptions(t *testing.T) {
	for i, entry := range allHelpEntries {
		if entry.key == "" {
			t.Errorf("allHelpEntries[%d].key is empty", i)
		}
		if entry.desc == "" {
			t.Errorf("allHelpEntries[%d].desc is empty", i)
		}
	}
}

func TestAllHelpEntriesContainsExpectedKeys(t *testing.T) {
	expectedKeys := []string{"j", "k", "Enter", "/", "r", "n", "d", "?", "q", "Esc"}

	for _, expected := range expectedKeys {
		found := false
		for _, entry := range allHelpEntries {
			if strings.Contains(entry.key, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("allHelpEntries missing key containing %q", expected)
		}
	}
}

func TestViewHelpRendersTitle(t *testing.T) {
	styles := StylesFromTheme(DarkTheme())
	view := viewHelp(styles, 80, 24)

	if !strings.Contains(view, "Keyboard shortcuts") {
		t.Error("viewHelp should contain title")
	}
}

func TestViewHelpRendersKeybindings(t *testing.T) {
	styles := StylesFromTheme(DarkTheme())
	view := viewHelp(styles, 80, 24)

	// Check a few expected keybindings are rendered
	if !strings.Contains(view, "Next item") {
		t.Error("viewHelp should contain 'Next item'")
	}
	if !strings.Contains(view, "Previous item") {
		t.Error("viewHelp should contain 'Previous item'")
	}
	if !strings.Contains(view, "Quit") {
		t.Error("viewHelp should contain 'Quit'")
	}
}

func TestViewHelpRendersCloseInstruction(t *testing.T) {
	styles := StylesFromTheme(DarkTheme())
	view := viewHelp(styles, 80, 24)

	if !strings.Contains(view, "close") {
		t.Error("viewHelp should contain close instruction")
	}
}

func TestViewHelpCentersContent(t *testing.T) {
	styles := StylesFromTheme(DarkTheme())
	view := viewHelp(styles, 120, 40)

	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatal("viewHelp should return multiple lines")
	}

	// On a wide terminal, lines should have leading spaces for centering
	// At least some lines should have padding
	hasLeadingSpaces := false
	for _, line := range lines {
		if len(line) > 0 && line[0] == ' ' {
			hasLeadingSpaces = true
			break
		}
	}
	if !hasLeadingSpaces {
		t.Error("viewHelp should center content with leading spaces on wide terminal")
	}
}

func TestViewHelpNarrowTerminal(t *testing.T) {
	styles := StylesFromTheme(DarkTheme())
	view := viewHelp(styles, 40, 20)

	// Should still render without crash
	if view == "" {
		t.Error("viewHelp should render content even on narrow terminal")
	}
}

func TestViewHelpWithLightTheme(t *testing.T) {
	styles := StylesFromTheme(LightTheme())
	view := viewHelp(styles, 80, 24)

	if !strings.Contains(view, "Keyboard shortcuts") {
		t.Error("viewHelp with light theme should contain title")
	}
}

func TestHelpEntryStruct(t *testing.T) {
	entry := helpEntry{key: "test", desc: "Test description"}
	if entry.key != "test" {
		t.Errorf("key = %q, want %q", entry.key, "test")
	}
	if entry.desc != "Test description" {
		t.Errorf("desc = %q, want %q", entry.desc, "Test description")
	}
}
