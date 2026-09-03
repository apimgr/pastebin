//go:build windows

package i2p

import (
	"os/exec"
)

// terminateGracefully has no SIGTERM equivalent on Windows, so it kills
// the process directly; the caller's shutdownGrace/Kill fallback still
// applies for any process that doesn't exit promptly.
func terminateGracefully(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
