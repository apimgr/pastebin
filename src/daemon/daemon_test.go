//go:build !windows

package daemon_test

import (
	"testing"

	"github.com/apimgr/pastebin/src/daemon"
)

// ─── Daemonize ────────────────────────────────────────────────────────────────

func TestDaemonize_AlreadyChild(t *testing.T) {
	// When _DAEMON_CHILD=1 is set, Daemonize must return nil immediately
	// without forking or exiting.
	t.Setenv("_DAEMON_CHILD", "1")
	if err := daemon.Daemonize(); err != nil {
		t.Errorf("Daemonize (child mode): expected nil, got %v", err)
	}
}
