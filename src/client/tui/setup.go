package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// setupModel is the sub-model for the server URL setup wizard.
type setupModel struct {
	input   textinput.Model
	err     string
	saveURL func(string) error
}

// serverURLSavedMsg is sent when the server URL has been persisted.
type serverURLSavedMsg struct{ url string }

// serverURLErrorMsg is sent when saving the server URL fails.
type serverURLErrorMsg struct{ err error }

// newSetupModel creates an initialised setup sub-model.
func newSetupModel(saveURL func(string) error) setupModel {
	ti := textinput.New()
	ti.Placeholder = "https://paste.example.com"
	ti.Focus()
	ti.CharLimit = 512
	ti.Width = 44
	return setupModel{input: ti, saveURL: saveURL}
}

// update processes key events for the setup wizard.
func (s setupModel) update(msg tea.Msg) (setupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			rawURL := strings.TrimSpace(s.input.Value())
			if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
				s.err = t("setup_url_invalid")
				return s, nil
			}
			s.err = ""
			saveFn := s.saveURL
			return s, func() tea.Msg {
				if err := saveFn(rawURL); err != nil {
					return serverURLErrorMsg{err: err}
				}
				return serverURLSavedMsg{url: rawURL}
			}
		}
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, cmd
}

// view renders the setup wizard.
func (s setupModel) view(styles TUIStyles) string {
	title := styles.Title.Render(t("setup_title"))
	prompt := styles.Normal.Render(t("setup_server_url_label"))
	inputView := s.input.View()

	errLine := ""
	if s.err != "" {
		errLine = "\n" + styles.Error.Render(s.err)
	}

	hint := styles.Muted.Render(t("setup_hint"))

	inner := fmt.Sprintf("%s\n\n%s\n> %s%s\n\n%s",
		title, prompt, inputView, errLine, hint)

	return styles.Border.
		Width(47).
		Padding(1, 2).
		Render(inner)
}
