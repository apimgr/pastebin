// Package tui provides the bubbletea-based interactive TUI for pastebin-cli.
//
// Entry point: Run(cfg) launches the program. The main Model dispatches to
// sub-models (setup, list, detail) based on the current state.
package tui

import (
	"fmt"
	"strings"

	"github.com/apimgr/pastebin/src/common/terminal"
	tea "github.com/charmbracelet/bubbletea"
)

// state represents which screen is currently active.
type state int

const (
	// server URL setup wizard
	stateSetup state = iota
	// paste list view
	stateListing
	// paste detail view
	stateDetail
	// help overlay (shown on top of the current view)
	stateHelp
)

// ClientConfig is passed from main.go to tui.Run.
type ClientConfig struct {
	// Server is the base URL of the pastebin server (may be empty on first run).
	Server string
	// Lang is the resolved Accept-Language locale string.
	Lang string
	// SaveURL persists a newly configured server URL to cli.yml.
	SaveURL func(string) error
	// CfgPath is the path to cli.yml (used for display in setup prompts).
	CfgPath string
}

// Model is the root bubbletea model for the TUI.
type Model struct {
	cfg       ClientConfig
	state     state
	prevState state

	width  int
	height int
	mode   terminal.SizeMode
	styles TUIStyles

	// Sub-models.
	setup  setupModel
	list   listModel
	detail detailModel
}

// checkServerURL returns true when a valid server URL is configured.
func checkServerURL(server string) bool {
	return strings.HasPrefix(server, "http://") || strings.HasPrefix(server, "https://")
}

// Run initialises and starts the bubbletea program.
func Run(cfg ClientConfig) error {
	m := newModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// newModel builds the initial Model and chooses the starting state.
func newModel(cfg ClientConfig) Model {
	styles := StylesFromTheme(CurrentTheme)
	ts := terminal.GetTerminalSize()

	m := Model{
		cfg:    cfg,
		width:  ts.Cols,
		height: ts.Rows,
		mode:   ts.Mode,
		styles: styles,
	}

	if !checkServerURL(cfg.Server) {
		m.state = stateSetup
		m.setup = newSetupModel(cfg.SaveURL)
	} else {
		m.state = stateListing
		var cmd tea.Cmd
		m.list, cmd = newListModel(cfg.Server, cfg.Lang)
		// cmd is consumed in Init.
		_ = cmd
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.state == stateListing {
		return m.list.loadCmd()
	}
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Recalculate size mode from new dimensions via GetTerminalSize override;
		// since we can't call the unexported calculateMode directly, we derive
		// the mode from the TerminalSize lookup table embedded in SizeMode methods.
		m.mode = sizeMode(msg.Width, msg.Height)
		m.detail.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		// Global keys handled first.
		switch msg.String() {
		case "ctrl+c", "q":
			// In search mode, q types a character instead of quitting.
			if m.state == stateListing && m.list.searching {
				break
			}
			return m, tea.Quit
		case "?":
			if m.state == stateHelp {
				m.state = m.prevState
			} else {
				m.prevState = m.state
				m.state = stateHelp
			}
			return m, nil
		case "esc":
			return m.handleEsc()
		}

	// Setup wizard: URL saved — move to listing.
	case serverURLSavedMsg:
		m.cfg.Server = msg.url
		m.state = stateListing
		var cmd tea.Cmd
		m.list, cmd = newListModel(m.cfg.Server, m.cfg.Lang)
		return m, cmd

	// Setup wizard: save error — display it in setup.
	case serverURLErrorMsg:
		m.setup.err = fmt.Sprintf("save failed: %v", msg.err)
		return m, nil

	// List view: pastes loaded.
	case pastesLoadedMsg:
		var cmd tea.Cmd
		m.list, cmd = m.list.update(msg)
		return m, cmd

	// Detail view: paste raw content loaded.
	case pasteRawMsg:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.update(msg)
		return m, cmd

	// Detail view: delete result.
	case pasteDeletedMsg:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.update(msg)
		return m, cmd

	case pasteDeleteErrMsg:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.update(msg)
		return m, cmd
	}

	// Delegate to the active sub-model.
	return m.delegateUpdate(msg)
}

// delegateUpdate routes unhandled messages to the active sub-model.
func (m Model) delegateUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateSetup:
		var cmd tea.Cmd
		m.setup, cmd = m.setup.update(msg)
		return m, cmd

	case stateListing:
		prevList := m.list
		var cmd tea.Cmd
		m.list, cmd = m.list.update(msg)

		// Check if Enter was pressed to open detail.
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyEnter && !prevList.searching {
			if item := prevList.selectedItem(); item != nil {
				var loadCmd tea.Cmd
				m.detail, loadCmd = newDetailModel(m.cfg.Server, m.cfg.Lang, item.ID, m.width, m.height)
				m.state = stateDetail
				return m, loadCmd
			}
		}
		return m, cmd

	case stateDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.update(msg)
		return m, cmd

	case stateHelp:
		// Help is an overlay; no sub-model update.
		return m, nil
	}
	return m, nil
}

// handleEsc navigates backward from the current state.
func (m Model) handleEsc() (tea.Model, tea.Cmd) {
	switch m.state {
	case stateHelp:
		m.state = m.prevState
	case stateDetail:
		m.state = stateListing
	case stateListing:
		if m.list.searching {
			m.list.searching = false
			m.list.searchQuery = ""
			m.list.applySearch()
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	switch m.state {
	case stateSetup:
		return m.viewSetup()
	case stateListing:
		return m.viewListing()
	case stateDetail:
		return m.viewDetail()
	case stateHelp:
		return m.viewHelpOverlay()
	}
	return ""
}

// viewSetup renders the setup wizard centred on screen.
func (m Model) viewSetup() string {
	content := m.setup.view(m.styles)
	lines := strings.Split(content, "\n")
	contentH := len(lines)
	top := (m.height - contentH) / 2
	if top < 0 {
		top = 0
	}
	return strings.Repeat("\n", top) + content
}

// viewListing renders the header + list view + help line.
func (m Model) viewListing() string {
	header := m.styles.Title.Render("pastebin  ") +
		m.styles.Muted.Render(m.cfg.Server)
	listContent := m.list.view(m.styles, m.width, m.height-3, m.mode)
	helpLine := m.styles.Help.Render(helpLineForMode(m.mode))
	return header + "\n" + listContent + "\n" + helpLine
}

// viewDetail renders the paste detail with header.
func (m Model) viewDetail() string {
	return m.detail.view(m.styles, m.width)
}

// viewHelpOverlay renders the help overlay on top of the current background view.
func (m Model) viewHelpOverlay() string {
	bg := ""
	switch m.prevState {
	case stateListing:
		bg = m.viewListing()
	case stateDetail:
		bg = m.viewDetail()
	}
	bgLines := strings.Split(bg, "\n")

	overlay := viewHelp(m.styles, m.width, m.height)
	overlayLines := strings.Split(overlay, "\n")

	// Place the overlay starting at row 2 of the background.
	startRow := 2
	for i, ol := range overlayLines {
		row := startRow + i
		if row < len(bgLines) {
			bgLines[row] = ol
		} else {
			bgLines = append(bgLines, ol)
		}
	}
	return strings.Join(bgLines, "\n")
}
