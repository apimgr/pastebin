//go:build !windows

package i2p

import (
	"os/exec"
	"syscall"
)

// terminateGracefully sends SIGTERM so i2pd can shut down cleanly and
// flush its destination keys, mirroring how the Tor child process is
// stopped elsewhere in this project.
func terminateGracefully(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}
