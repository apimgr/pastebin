package display

import (
	"fmt"
	"strings"
	"time"
)

// Spinner is the interface for progress spinners returned by NewSpinner.
type Spinner interface {
	// Start begins the spinner animation (no-op for TextSpinner).
	Start()
	// Stop ends the spinner animation and clears the line (no-op for TextSpinner).
	Stop()
	// SetMessage updates the spinner's displayed message.
	SetMessage(msg string)
}

// TextSpinner is a TERM=dumb-safe spinner: it prints plain text progress lines
// instead of ANSI escape sequences.
type TextSpinner struct {
	message string
}

// Start prints the initial message to stdout.
func (s *TextSpinner) Start() {
	fmt.Printf("... %s\n", s.message)
}

// Stop is a no-op for TextSpinner (no ANSI escape to clear).
func (s *TextSpinner) Stop() {}

// SetMessage updates the message and prints a new progress line.
func (s *TextSpinner) SetMessage(msg string) {
	s.message = msg
	fmt.Printf("... %s\n", msg)
}

// ANSISpinner is a spinner that uses ANSI escape sequences for animated output.
type ANSISpinner struct {
	message string
	frames  []string
	idx     int
	done    chan struct{}
}

// Start begins the ANSI spinner animation in a background goroutine.
func (s *ANSISpinner) Start() {
	s.done = make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				fmt.Printf("\r\033[K")
				return
			case <-ticker.C:
				fmt.Printf("\r%s %s ", s.frames[s.idx%len(s.frames)], s.message)
				s.idx++
			}
		}
	}()
}

// Stop halts the ANSI spinner and clears the line.
func (s *ANSISpinner) Stop() {
	if s.done != nil {
		close(s.done)
		s.done = nil
	}
}

// SetMessage updates the message displayed next to the spinner.
func (s *ANSISpinner) SetMessage(msg string) {
	s.message = msg
}

// ansiFrames are the default spinner animation frames.
var ansiFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// textFrames are ASCII fallback spinner frames for partial ANSI support.
var textFrames = []string{"|", "/", "-", "\\"}

// NewSpinner returns a TextSpinner for TERM=dumb environments and an
// ANSISpinner for terminals that support ANSI escape sequences (PART 7, AI.md line 9378).
func NewSpinner(env *DisplayEnv, message string) Spinner {
	if env == nil || env.IsDumbTerminal() || !CanUseANSI(env) {
		return &TextSpinner{message: message}
	}
	frames := ansiFrames
	if strings.Contains(env.TerminalType, "xterm") || strings.Contains(env.TerminalType, "256color") {
		frames = ansiFrames
	} else {
		frames = textFrames
	}
	return &ANSISpinner{
		message: message,
		frames:  frames,
	}
}

// ShowProgress prints a progress update. In dumb-terminal mode it prints
// "N% complete\n"; in ANSI mode it overwrites the current line with a progress
// bar (PART 7, AI.md line 9395).
func ShowProgress(env *DisplayEnv, percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	if env == nil || env.IsDumbTerminal() || !CanUseANSI(env) {
		fmt.Printf("%d%% complete\n", percent)
		return
	}

	const barWidth = 40
	filled := barWidth * percent / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	fmt.Printf("\r[%s] %3d%%", bar, percent)
	if percent == 100 {
		fmt.Println()
	}
}
