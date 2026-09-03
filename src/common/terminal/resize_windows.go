//go:build windows

package terminal

import (
	"os"
	"time"

	"golang.org/x/term"
)

// OnResize invokes callback whenever the terminal window is resized.
// Windows has no SIGWINCH signal, so the size is polled every 500ms and
// the callback fires only when the dimensions actually change (AI.md
// "Window Resize Handling" — watch_windows.go). The returned stop value
// halts polling.
func OnResize(callback func(cols, rows int)) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		lastCols, lastRows, _ := term.GetSize(int(os.Stdout.Fd()))
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
				if err != nil {
					continue
				}
				if cols != lastCols || rows != lastRows {
					lastCols, lastRows = cols, rows
					callback(cols, rows)
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}
