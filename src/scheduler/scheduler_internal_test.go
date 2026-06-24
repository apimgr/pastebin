package scheduler

// Internal tests for unexported scheduler methods that cannot be called from
// the external test package. These tests verify tick() and runMissed() without
// starting a real goroutine loop.

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestTick_NoTasksDue verifies that tick() on an empty scheduler does not
// panic and does not execute any tasks.
func TestTick_NoTasksDue(t *testing.T) {
	s := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("tick() panicked: %v", r)
		}
	}()
	s.tick()
}

// TestTick_SkipsNotYetDueTasks verifies that tick() does not execute a task
// whose nextRun is in the future.
func TestTick_SkipsNotYetDueTasks(t *testing.T) {
	var called int32
	s := New(nil)
	sched, err := ParseSchedule("@every 1h")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	s.tasks["notyet"] = &taskEntry{
		id:      "notyet",
		name:    "Not Yet Due",
		schedule: sched,
		enabled: true,
		nextRun: time.Now().Add(1 * time.Hour),
		fn:      func() error { atomic.AddInt32(&called, 1); return nil },
	}
	s.tick()
	if atomic.LoadInt32(&called) != 0 {
		t.Error("tick() must not execute tasks whose nextRun is in the future")
	}
}

// TestTick_ExecutesDueTask verifies that tick() executes a task that is past
// its nextRun time. The task sets a flag; tick() launches it in a goroutine,
// so we wait briefly for it to complete.
func TestTick_ExecutesDueTask(t *testing.T) {
	done := make(chan struct{})
	s := New(nil)
	sched, err := ParseSchedule("@every 1m")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	s.tasks["overdue"] = &taskEntry{
		id:      "overdue",
		name:    "Overdue Task",
		schedule: sched,
		enabled: true,
		nextRun: time.Now().Add(-1 * time.Second),
		fn: func() error {
			close(done)
			return nil
		},
	}
	s.tick()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("tick() did not execute an overdue task within 2 seconds")
	}
}

// TestTick_SkipsDisabledTasks verifies that disabled tasks are not run even
// when they are past their nextRun.
func TestTick_SkipsDisabledTasks(t *testing.T) {
	var called int32
	s := New(nil)
	sched, err := ParseSchedule("@every 1m")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	s.tasks["disabled"] = &taskEntry{
		id:      "disabled",
		name:    "Disabled Task",
		schedule: sched,
		enabled: false,
		nextRun: time.Now().Add(-1 * time.Second),
		fn:      func() error { atomic.AddInt32(&called, 1); return nil },
	}
	s.tick()
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&called) != 0 {
		t.Error("tick() must not execute disabled tasks")
	}
}

// TestRunMissed_NoTasksDue verifies that runMissed() on an empty scheduler
// does not panic.
func TestRunMissed_NoTasksDue(t *testing.T) {
	s := New(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("runMissed() panicked: %v", r)
		}
	}()
	s.runMissed()
}

// TestRunMissed_ExecutesMissedTask verifies that runMissed() executes a task
// that was due within the catch-up window while the server was down.
func TestRunMissed_ExecutesMissedTask(t *testing.T) {
	done := make(chan struct{})
	s := New(nil)
	s.catchUpWindow = 2 * time.Hour
	sched, err := ParseSchedule("@every 1h")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	s.tasks["missed"] = &taskEntry{
		id:      "missed",
		name:    "Missed Task",
		schedule: sched,
		enabled: true,
		nextRun: time.Now().Add(-30 * time.Minute),
		fn: func() error {
			close(done)
			return nil
		},
	}
	s.runMissed()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("runMissed() did not execute a missed task within 2 seconds")
	}
}

// TestRunMissed_SkipsOutsideCatchUpWindow verifies that runMissed() does not
// execute a task whose nextRun is outside the catch-up window.
func TestRunMissed_SkipsOutsideCatchUpWindow(t *testing.T) {
	var called int32
	s := New(nil)
	s.catchUpWindow = 30 * time.Minute
	sched, err := ParseSchedule("@every 1h")
	if err != nil {
		t.Fatalf("ParseSchedule: %v", err)
	}
	s.tasks["ancient"] = &taskEntry{
		id:      "ancient",
		name:    "Ancient Task",
		schedule: sched,
		enabled: true,
		nextRun: time.Now().Add(-2 * time.Hour),
		fn:      func() error { atomic.AddInt32(&called, 1); return nil },
	}
	s.runMissed()
	time.Sleep(50 * time.Millisecond)
	if atomic.LoadInt32(&called) != 0 {
		t.Error("runMissed() must not execute tasks outside the catch-up window")
	}
}
