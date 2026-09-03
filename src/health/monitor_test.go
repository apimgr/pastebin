package health

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestMonitor builds a monitor with short intervals suitable for tests.
func newTestMonitor(selfHealing bool, maxAttempts int) *Monitor {
	return New(Config{
		SelfHealingEnabled: selfHealing,
		RetryInterval:      5 * time.Millisecond,
		MaxAttempts:        maxAttempts,
		NotifyOnEnter:      true,
		NotifyOnExit:       true,
	})
}

func TestNewDefaultsInterval(t *testing.T) {
	m := New(Config{})
	if m.retryInterval != 30*time.Second {
		t.Fatalf("expected default 30s interval, got %s", m.retryInterval)
	}
	if m.State() != StateStarting {
		t.Fatalf("expected starting state, got %s", m.State())
	}
}

func TestEnterExitTransitions(t *testing.T) {
	m := newTestMonitor(true, 0)
	var events []string
	var mu sync.Mutex
	m.SetNotifier(func(event string, _ Snapshot) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	m.Enter(ReasonDatabaseConnection, "db down")
	if !m.InMaintenance() {
		t.Fatal("expected maintenance state after Enter")
	}
	snap := m.Snapshot()
	if snap.Reason != ReasonDatabaseConnection || snap.Message != "db down" {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
	if snap.Since.IsZero() {
		t.Fatal("expected Since to be set")
	}

	// Re-entering is idempotent and must not change Since or re-fire notifier.
	since := snap.Since
	m.Enter(ReasonFileWrite, "disk full")
	if got := m.Snapshot(); got.Reason != ReasonDatabaseConnection || !got.Since.Equal(since) {
		t.Fatalf("re-enter should be idempotent, got %+v", got)
	}

	m.Exit()
	if m.InMaintenance() {
		t.Fatal("expected normal state after Exit")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 2 || events[0] != "enter" || events[1] != "exit" {
		t.Fatalf("expected [enter exit], got %v", events)
	}
}

func TestTickEntersOnFailure(t *testing.T) {
	m := newTestMonitor(true, 0)
	fail := true
	m.SetChecker(func() (bool, string, string) {
		if fail {
			return false, ReasonDatabaseConnection, "db down"
		}
		return true, "", ""
	})

	m.tick()
	if !m.InMaintenance() {
		t.Fatal("expected maintenance after failing tick")
	}

	// Recovery: checker now passes, tick should exit maintenance.
	fail = false
	m.tick()
	if m.InMaintenance() {
		t.Fatal("expected exit maintenance after passing tick")
	}
	if m.State() != StateNormal {
		t.Fatalf("expected normal, got %s", m.State())
	}
}

func TestTickStartingToNormal(t *testing.T) {
	m := newTestMonitor(true, 0)
	m.SetChecker(func() (bool, string, string) { return true, "", "" })
	m.tick()
	if m.State() != StateNormal {
		t.Fatalf("expected normal from starting, got %s", m.State())
	}
}

func TestMaxAttemptsCap(t *testing.T) {
	m := newTestMonitor(true, 2)
	m.SetChecker(func() (bool, string, string) { return false, ReasonDatabaseConnection, "db down" })

	m.tick()
	for i := 0; i < 5; i++ {
		m.tick()
	}
	if got := m.Snapshot().Attempts; got != 2 {
		t.Fatalf("expected attempts capped at 2, got %d", got)
	}
}

func TestCleanerRunsOnFileWrite(t *testing.T) {
	m := newTestMonitor(true, 0)
	var cleaned int
	m.SetCleaner(func() { cleaned++ })
	fail := true
	m.SetChecker(func() (bool, string, string) {
		if fail {
			return false, ReasonFileWrite, "disk full"
		}
		return true, "", ""
	})
	// Enter maintenance via file_write.
	m.tick()
	if !m.InMaintenance() {
		t.Fatal("expected maintenance")
	}
	// Next tick while in maintenance with file_write reason must run cleaner.
	m.tick()
	if cleaned == 0 {
		t.Fatal("expected cleaner to run during file_write maintenance")
	}
}

func TestSelfHealingDisabledStaysInMaintenance(t *testing.T) {
	m := newTestMonitor(false, 0)
	fail := true
	m.SetChecker(func() (bool, string, string) {
		if fail {
			return false, ReasonDatabaseConnection, "db down"
		}
		return true, "", ""
	})
	m.tick()
	if !m.InMaintenance() {
		t.Fatal("expected maintenance")
	}
	fail = false
	m.tick()
	if !m.InMaintenance() {
		t.Fatal("self-healing disabled: should remain in maintenance")
	}
}

func TestRetrySeconds(t *testing.T) {
	if got := (Snapshot{RetryInterval: 30 * time.Second}).RetrySeconds(); got != 30 {
		t.Fatalf("expected 30, got %d", got)
	}
	if got := (Snapshot{RetryInterval: 100 * time.Millisecond}).RetrySeconds(); got != 1 {
		t.Fatalf("expected minimum 1, got %d", got)
	}
}

func TestStartStopsOnContextCancel(t *testing.T) {
	m := newTestMonitor(true, 0)
	m.SetChecker(func() (bool, string, string) { return true, "", "" })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}
