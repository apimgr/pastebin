package i2p

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// osProcess wraps a running i2pd child process, tracking liveness and
// providing a graceful-then-forced shutdown, mirroring how bine manages
// the Tor child process in src/tor/tor.go.
type osProcess struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

// startI2Pd launches i2pd as a child process pointed at the given
// tunnels.conf, with its data/config/log directories under the server's
// own directory tree (never a system-wide i2pd install).
func startI2Pd(bin, configDir, dataDir, logDir, tunnelsPath string) (*osProcess, error) {
	i2pDataDir := filepath.Join(dataDir, "i2p")
	i2pConfigDir := filepath.Join(configDir, "i2p")
	logPath := filepath.Join(logDir, "i2pd.log")

	args := []string{
		"--tunconf=" + tunnelsPath,
		"--datadir=" + i2pDataDir,
		"--conf=" + filepath.Join(i2pConfigDir, "i2pd.conf"),
		"--log=file",
		"--logfile=" + logPath,
		"--daemon=false",
		"--notransit=true",
		"--ifname4=127.0.0.1",
	}

	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec i2pd: %w", err)
	}

	p := &osProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		waitErr := cmd.Wait()
		p.mu.Lock()
		p.err = waitErr
		p.mu.Unlock()
		close(p.done)
	}()
	return p, nil
}

// Alive reports whether the i2pd process is still running.
func (p *osProcess) Alive() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// shutdownGrace is how long Close waits for i2pd to exit after SIGTERM
// before escalating to SIGKILL.
const shutdownGrace = 10 * time.Second

// Close terminates the i2pd process: SIGTERM first, escalating to SIGKILL
// if it has not exited within shutdownGrace.
func (p *osProcess) Close() error {
	p.mu.Lock()
	cmd := p.cmd
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil || !p.Alive() {
		return nil
	}
	_ = terminateGracefully(cmd)
	select {
	case <-p.done:
		return nil
	case <-time.After(shutdownGrace):
		_ = cmd.Process.Kill()
		<-p.done
		return nil
	}
}
