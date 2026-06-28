package scheduler

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/metrics"
)

const defaultCatchUpWindow = time.Hour

// TaskFunc is the function executed by a scheduled task.
type TaskFunc func() error

// taskEntry is an in-memory representation of a registered task.
type taskEntry struct {
	id       string
	name     string
	schedule Schedule
	fn       TaskFunc
	enabled  bool

	// mutable state — protected by Scheduler.mu
	lastRun    time.Time
	lastStatus string
	lastError  string
	nextRun    time.Time
	runCount   int64
	failCount  int64
	running    bool
}

// Scheduler is the built-in cron-capable scheduler (PART 18).
// It stores persistent state in the database and recovers missed runs on
// startup within the catch-up window.
type Scheduler struct {
	db            database.DB
	catchUpWindow time.Duration
	loc           *time.Location

	mu      sync.Mutex
	tasks   map[string]*taskEntry
	stop    chan struct{}
	running bool
}

// New creates a new Scheduler.
// db may be nil — in that case state is not persisted (useful for tests).
func New(db database.DB) *Scheduler {
	loc := time.Local
	return &Scheduler{
		db:            db,
		catchUpWindow: defaultCatchUpWindow,
		loc:           loc,
		tasks:         make(map[string]*taskEntry),
		stop:          make(chan struct{}),
	}
}

// SetCatchUpWindow overrides the default 1-hour catch-up window.
func (s *Scheduler) SetCatchUpWindow(d time.Duration) { s.catchUpWindow = d }

// SetLocation sets the timezone used for cron expressions.
func (s *Scheduler) SetLocation(loc *time.Location) { s.loc = loc }

// Register adds a task. Call before Start. schedule is any expression
// understood by ParseSchedule. enabled=false means the task is registered but
// will not run until explicitly enabled.
func (s *Scheduler) Register(id, name, schedExpr string, enabled bool, fn TaskFunc) error {
	sched, err := ParseSchedule(schedExpr)
	if err != nil {
		return fmt.Errorf("scheduler.Register %q: %w", id, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e := &taskEntry{
		id:         id,
		name:       name,
		schedule:   sched,
		fn:         fn,
		enabled:    enabled,
		lastStatus: "pending",
	}
	s.tasks[id] = e
	return nil
}

// Start loads persistent state from DB, runs any missed tasks within the
// catch-up window, then starts the scheduler loop.
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.mu.Unlock()

	// Load persistent state and compute initial nextRun values.
	s.loadState()

	// Run any missed tasks (within catch-up window) immediately.
	s.runMissed()

	log.Printf("scheduler: started with %d tasks", len(s.tasks))

	go s.loop()
}

// Running reports whether the scheduler loop is currently active.
func (s *Scheduler) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Stop signals the scheduler loop to exit. It waits for the current tick to
// finish but does not wait for running task goroutines.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stop)
	s.mu.Unlock()
	log.Printf("scheduler: stopped")
}

// RunNow executes a task immediately (bypasses the schedule but still updates
// persistent state).
func (s *Scheduler) RunNow(id string) error {
	s.mu.Lock()
	e, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("task %q not found", id)
	}
	if e.running {
		s.mu.Unlock()
		return fmt.Errorf("task %q is already running", id)
	}
	e.running = true
	s.mu.Unlock()

	err := s.execute(e)

	s.mu.Lock()
	e.running = false
	s.mu.Unlock()
	return err
}

// EnableTask enables a registered task.
func (s *Scheduler) EnableTask(id string) {
	s.mu.Lock()
	if e, ok := s.tasks[id]; ok {
		e.enabled = true
		e.nextRun = e.schedule.Next(time.Now())
		s.persistState(e)
	}
	s.mu.Unlock()
}

// DisableTask disables a registered task (it will no longer fire on schedule).
func (s *Scheduler) DisableTask(id string) {
	s.mu.Lock()
	if e, ok := s.tasks[id]; ok {
		e.enabled = false
		s.persistState(e)
	}
	s.mu.Unlock()
}

// GetTask returns the state for a single task by ID, or false if not found.
func (s *Scheduler) GetTask(id string) (database.TaskState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.tasks[id]
	if !ok {
		return database.TaskState{}, false
	}
	return taskEntryToState(e), true
}

// GetTasks returns a snapshot of all registered tasks.
func (s *Scheduler) GetTasks() []database.TaskState {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]database.TaskState, 0, len(s.tasks))
	for _, e := range s.tasks {
		out = append(out, taskEntryToState(e))
	}
	return out
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (s *Scheduler) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	now := time.Now()

	s.mu.Lock()
	var due []*taskEntry
	for _, e := range s.tasks {
		if e.enabled && !e.running && !e.nextRun.IsZero() && now.After(e.nextRun) {
			due = append(due, e)
			e.running = true
		}
	}
	s.mu.Unlock()

	for _, e := range due {
		go func(entry *taskEntry) {
			s.execute(entry) //nolint:errcheck

			s.mu.Lock()
			entry.running = false
			s.mu.Unlock()
		}(e)
	}
}

// execute runs the task function and updates state.
func (s *Scheduler) execute(e *taskEntry) error {
	start := time.Now()
	log.Printf("scheduler: running %s", e.id)

	metrics.SchedulerTasksRunning.WithLabelValues(e.id).Inc()
	taskErr := e.fn()
	metrics.SchedulerTasksRunning.WithLabelValues(e.id).Dec()

	finished := time.Now()
	durationMS := finished.Sub(start).Milliseconds()

	status := "success"
	errMsg := ""
	if taskErr != nil {
		status = "failed"
		errMsg = taskErr.Error()
		log.Printf("scheduler: task %s failed: %v", e.id, taskErr)
	} else {
		log.Printf("scheduler: task %s completed (%dms)", e.id, durationMS)
	}

	metrics.SchedulerTasksTotal.WithLabelValues(e.id, status).Inc()
	metrics.SchedulerTaskDuration.WithLabelValues(e.id).Observe(finished.Sub(start).Seconds())
	metrics.SchedulerLastRunTimestamp.WithLabelValues(e.id).Set(float64(finished.Unix()))

	next := e.schedule.Next(finished)

	s.mu.Lock()
	e.lastRun = start
	e.lastStatus = status
	e.lastError = errMsg
	e.nextRun = next
	if status == "success" {
		e.runCount++
	} else {
		e.failCount++
	}

	if s.db != nil {
		s.db.UpdateTaskRun(e.id, e.lastRun, e.lastStatus, e.lastError, //nolint:errcheck
			e.runCount, e.failCount, e.nextRun)
		s.db.RecordTaskHistory(&database.TaskHistory{ //nolint:errcheck
			TaskID:     e.id,
			StartedAt:  start,
			FinishedAt: finished,
			Status:     status,
			ErrorMsg:   errMsg,
			DurationMS: durationMS,
		})
	}
	s.mu.Unlock()

	return taskErr
}

// loadState reads persistent task state from the DB and merges it into the
// in-memory task entries. For tasks not yet in the DB, upserts an initial row.
func (s *Scheduler) loadState() {
	if s.db == nil {
		// No DB — just compute initial nextRun values from scratch.
		s.mu.Lock()
		now := time.Now()
		for _, e := range s.tasks {
			e.nextRun = e.schedule.Next(now)
		}
		s.mu.Unlock()
		return
	}

	stored, err := s.db.ListSchedulerTasks()
	if err != nil {
		log.Printf("scheduler: could not load state from DB: %v", err)
	}

	// Index stored tasks by ID for quick lookup.
	stateByID := make(map[string]*database.TaskState, len(stored))
	for _, st := range stored {
		stateByID[st.TaskID] = st
	}

	now := time.Now()

	s.mu.Lock()
	for _, e := range s.tasks {
		if st, ok := stateByID[e.TaskID()]; ok {
			// Restore persistent counters and timestamps.
			e.lastRun = st.LastRun
			e.lastStatus = st.LastStatus
			e.lastError = st.LastError
			e.runCount = st.RunCount
			e.failCount = st.FailCount
			// Recompute nextRun from the stored lastRun (the persisted nextRun
			// may be stale after a schedule change).
			if !e.lastRun.IsZero() {
				e.nextRun = e.schedule.Next(e.lastRun)
			} else {
				e.nextRun = e.schedule.Next(now)
			}
		} else {
			// First registration — compute initial nextRun and upsert.
			e.nextRun = e.schedule.Next(now)
		}
		// Always upsert to register new tasks and reflect schedule changes.
		s.db.UpsertSchedulerTask(toTaskState(e)) //nolint:errcheck
	}
	s.mu.Unlock()
}

// runMissed executes tasks that were due within the catch-up window while the
// server was down.
func (s *Scheduler) runMissed() {
	now := time.Now()
	cutoff := now.Add(-s.catchUpWindow)

	s.mu.Lock()
	var missed []*taskEntry
	for _, e := range s.tasks {
		if !e.enabled {
			continue
		}
		if e.nextRun.IsZero() {
			continue
		}
		// Task was due after cutoff but before now.
		if e.nextRun.After(cutoff) && e.nextRun.Before(now) {
			missed = append(missed, e)
			e.running = true
		}
	}
	s.mu.Unlock()

	for _, e := range missed {
		log.Printf("scheduler: catch-up run for %s (was due %s)", e.id, e.nextRun.Format(time.RFC3339))
		go func(entry *taskEntry) {
			s.execute(entry) //nolint:errcheck

			s.mu.Lock()
			entry.running = false
			s.mu.Unlock()
		}(e)
	}
}

// persistState writes the current in-memory state for a task to the DB.
// Must be called with s.mu held.
func (s *Scheduler) persistState(e *taskEntry) {
	if s.db == nil {
		return
	}
	s.db.UpsertSchedulerTask(toTaskState(e)) //nolint:errcheck
}

// taskEntry.TaskID returns the task ID (helper to satisfy interface method).
func (e *taskEntry) TaskID() string { return e.id }

func toTaskState(e *taskEntry) *database.TaskState {
	return &database.TaskState{
		TaskID:     e.id,
		TaskName:   e.name,
		Schedule:   e.schedule.String(),
		LastRun:    e.lastRun,
		LastStatus: e.lastStatus,
		LastError:  e.lastError,
		NextRun:    e.nextRun,
		RunCount:   e.runCount,
		FailCount:  e.failCount,
		Enabled:    e.enabled,
	}
}

func taskEntryToState(e *taskEntry) database.TaskState {
	return database.TaskState{
		TaskID:     e.id,
		TaskName:   e.name,
		Schedule:   e.schedule.String(),
		LastRun:    e.lastRun,
		LastStatus: e.lastStatus,
		LastError:  e.lastError,
		NextRun:    e.nextRun,
		RunCount:   e.runCount,
		FailCount:  e.failCount,
		Enabled:    e.enabled,
	}
}
