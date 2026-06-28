//go:build !windows

package terminal

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

// OnResize registers a callback that is invoked whenever the terminal window
// is resized (SIGWINCH). The callback receives the new column and row counts.
// Call the returned stop function to unregister the handler and free resources.
func OnResize(callback func(cols, rows int)) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ch:
				cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
				if err != nil {
					continue
				}
				callback(cols, rows)
			}
		}
	}()

	return func() {
		close(done)
		signal.Stop(ch)
	}
}
