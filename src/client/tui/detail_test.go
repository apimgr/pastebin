package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Tests for detail.go: detailModel, newDetailModel, update, view, resize

func TestNewDetailModelInitializes(t *testing.T) {
	m, cmd := newDetailModel("https://example.com", "en", "abc123", 80, 24)

	if m.pasteID != "abc123" {
		t.Errorf("pasteID = %q, want %q", m.pasteID, "abc123")
	}
	if m.server != "https://example.com" {
		t.Errorf("server = %q, want %q", m.server, "https://example.com")
	}
	if m.lang != "en" {
		t.Errorf("lang = %q, want %q", m.lang, "en")
	}
	if !m.loading {
		t.Error("loading should be true")
	}
	if m.deleting {
		t.Error("deleting should be false")
	}
	if cmd == nil {
		t.Error("cmd should not be nil")
	}
}

func TestDetailModelResize(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.resize(120, 40)

	if m.vp.Width != 120 {
		t.Errorf("vp.Width = %d, want 120", m.vp.Width)
	}
	// Height is reduced by 4 for header
	if m.vp.Height != 36 {
		t.Errorf("vp.Height = %d, want 36", m.vp.Height)
	}
}

func TestDetailModelUpdatePasteRawMsgSuccess(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)

	updated, cmd := m.update(pasteRawMsg{content: "Hello World", err: nil})
	if updated.loading {
		t.Error("loading should be false")
	}
	if updated.err != nil {
		t.Errorf("err = %v, want nil", updated.err)
	}
	if updated.content != "Hello World" {
		t.Errorf("content = %q, want %q", updated.content, "Hello World")
	}
	if !updated.vpReady {
		t.Error("vpReady should be true")
	}
	if cmd != nil {
		t.Error("cmd should be nil")
	}
}

func TestDetailModelUpdatePasteRawMsgError(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)

	testErr := errors.New("not found")
	updated, _ := m.update(pasteRawMsg{content: "", err: testErr})
	if updated.loading {
		t.Error("loading should be false")
	}
	if updated.err == nil {
		t.Error("err should not be nil")
	}
	if updated.vpReady {
		t.Error("vpReady should be false on error")
	}
}

func TestDetailModelUpdatePasteDeletedMsg(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true

	updated, cmd := m.update(pasteDeletedMsg{id: "abc123"})
	if updated.deleting {
		t.Error("deleting should be false")
	}
	if updated.deleteSuccess == "" {
		t.Error("deleteSuccess should be set")
	}
	if !strings.Contains(updated.deleteSuccess, "abc123") {
		t.Error("deleteSuccess should mention the paste ID")
	}
	if cmd != nil {
		t.Error("cmd should be nil")
	}
}

func TestDetailModelUpdatePasteDeleteErrMsg(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true

	testErr := errors.New("invalid token")
	updated, cmd := m.update(pasteDeleteErrMsg{err: testErr})
	if updated.deleting {
		t.Error("deleting should be false")
	}
	if updated.deleteErr == "" {
		t.Error("deleteErr should be set")
	}
	if !strings.Contains(updated.deleteErr, "invalid token") {
		t.Errorf("deleteErr = %q, should contain error message", updated.deleteErr)
	}
	if cmd != nil {
		t.Error("cmd should be nil")
	}
}

func TestDetailModelNavKeyJ(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.vpReady = true
	m.vp.SetContent(strings.Repeat("line\n", 100))

	// j should scroll down
	updated, _ := m.updateNav(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	// Viewport position changed - hard to test exact value, just ensure no crash
	_ = updated
}

func TestDetailModelNavKeyK(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.vpReady = true
	m.vp.SetContent(strings.Repeat("line\n", 100))

	updated, _ := m.updateNav(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	_ = updated
}

func TestDetailModelNavKeyD(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false

	updated, _ := m.updateNav(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !updated.deleting {
		t.Error("deleting should be true after 'd' key")
	}
	if updated.deleteErr != "" {
		t.Error("deleteErr should be empty")
	}
	if updated.deleteSuccess != "" {
		t.Error("deleteSuccess should be empty")
	}
}

func TestDetailModelDeleteModeEscape(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true
	m.deleteInput.Focus()

	updated, _ := m.updateDelete(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.deleting {
		t.Error("deleting should be false after Esc")
	}
}

func TestDetailModelDeleteModeEnterEmptyToken(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true
	m.deleteInput.SetValue("")

	updated, cmd := m.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.deleteErr == "" {
		t.Error("deleteErr should be set for empty token")
	}
	if !strings.Contains(updated.deleteErr, "token") {
		t.Errorf("deleteErr = %q, should mention token", updated.deleteErr)
	}
	if cmd != nil {
		t.Error("cmd should be nil for empty token")
	}
}

func TestDetailModelDeleteModeEnterWithToken(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true
	m.deleteInput.SetValue("tok_abc123xyz")

	updated, cmd := m.updateDelete(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated
	if cmd == nil {
		t.Error("cmd should not be nil when token provided")
	}
}

func TestDetailModelDeleteModeTyping(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true
	m.deleteInput.Focus()

	updated, _ := m.updateDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !strings.Contains(updated.deleteInput.Value(), "x") {
		t.Error("typing should add character to input")
	}
}

func TestDetailModelUpdateKeyMsgDelegatesDelete(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = true
	m.deleteInput.Focus()

	// Should delegate to updateDelete
	updated, _ := m.update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.deleting {
		t.Error("Esc in delete mode should cancel delete")
	}
}

func TestDetailModelUpdateKeyMsgDelegatesNav(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleting = false
	m.loading = false

	// Should delegate to updateNav
	updated, _ := m.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if !updated.deleting {
		t.Error("d key should enter delete mode")
	}
}

func TestDetailModelViewDeleteSuccess(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.deleteSuccess = "Paste abc123 deleted."
	styles := StylesFromTheme(DarkTheme())

	view := m.view(styles, 80)
	if !strings.Contains(view, "deleted") {
		t.Error("view should show delete success message")
	}
	if !strings.Contains(view, "Esc") || !strings.Contains(view, "back") {
		t.Error("view should show back instructions")
	}
}

func TestDetailModelViewLoading(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = true
	styles := StylesFromTheme(DarkTheme())

	view := m.view(styles, 80)
	if !strings.Contains(view, "Loading") {
		t.Error("view should show loading message")
	}
}

func TestDetailModelViewError(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.err = errors.New("paste not found")
	styles := StylesFromTheme(DarkTheme())

	view := m.view(styles, 80)
	if !strings.Contains(view, "Error") {
		t.Error("view should show error")
	}
	if !strings.Contains(view, "paste not found") {
		t.Error("view should show error message")
	}
}

func TestDetailModelViewContent(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.vpReady = true
	m.content = "Hello World"
	m.vp.SetContent("Hello World")
	styles := StylesFromTheme(DarkTheme())

	view := m.view(styles, 80)
	if !strings.Contains(view, "Paste: abc123") {
		t.Error("view should show paste ID in header")
	}
}

func TestDetailModelViewDeleteMode(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.vpReady = true
	m.deleting = true
	styles := StylesFromTheme(DarkTheme())

	view := m.view(styles, 80)
	if !strings.Contains(view, "delete token") {
		t.Error("view should show delete token prompt")
	}
}

func TestDetailModelViewDeleteError(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.vpReady = true
	m.deleting = true
	m.deleteErr = "invalid token"
	styles := StylesFromTheme(DarkTheme())

	view := m.view(styles, 80)
	if !strings.Contains(view, "invalid token") {
		t.Error("view should show delete error")
	}
}

func TestDetailModelLoadCmd(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	cmd := m.loadCmd()
	if cmd == nil {
		t.Error("loadCmd should return a non-nil command")
	}
}

func TestPasteRawMsgFields(t *testing.T) {
	msg := pasteRawMsg{content: "test content", err: nil}
	if msg.content != "test content" {
		t.Errorf("content = %q, want %q", msg.content, "test content")
	}
	if msg.err != nil {
		t.Errorf("err = %v, want nil", msg.err)
	}
}

func TestPasteDeletedMsgFields(t *testing.T) {
	msg := pasteDeletedMsg{id: "xyz789"}
	if msg.id != "xyz789" {
		t.Errorf("id = %q, want %q", msg.id, "xyz789")
	}
}

func TestPasteDeleteErrMsgFields(t *testing.T) {
	testErr := errors.New("forbidden")
	msg := pasteDeleteErrMsg{err: testErr}
	if msg.err != testErr {
		t.Errorf("err = %v, want %v", msg.err, testErr)
	}
}

func TestDetailModelUpdateForwardsToViewport(t *testing.T) {
	m, _ := newDetailModel("https://example.com", "en", "abc123", 80, 24)
	m.loading = false
	m.vpReady = true

	// Mouse wheel or other messages should be forwarded to viewport
	// Just ensure it doesn't panic
	updated, _ := m.update(tea.MouseMsg{})
	_ = updated
}
