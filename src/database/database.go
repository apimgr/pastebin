package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/model"
	_ "modernc.org/sqlite"
)

// TaskState holds the persistent state for a single scheduler task.
type TaskState struct {
	TaskID     string
	TaskName   string
	Schedule   string
	LastRun    time.Time
	LastStatus string // "pending" | "success" | "failed" | "skipped"
	LastError  string
	NextRun    time.Time
	RunCount   int64
	FailCount  int64
	Enabled    bool
}

// TaskHistory records a single execution event.
type TaskHistory struct {
	TaskID     string
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string
	ErrorMsg   string
	DurationMS int64
}

// DB is the database interface for paste and scheduler operations.
type DB interface {
	Close() error
	Type() string
	Ping() error

	// Paste operations
	CreatePaste(paste *model.Paste) error
	GetPasteByID(id string) (*model.Paste, error)
	GetPublicPastes(page, limit int) ([]model.PasteListItem, int, error)
	IncrementPasteViews(id string) error
	DeletePaste(id string) error
	DeletePasteByToken(id, deleteTokenHash string) error
	DeleteExpiredPastes() (int64, error)
	DeleteBurnedPastes() (int64, error)

	// Scheduler operations (PART 18)
	UpsertSchedulerTask(t *TaskState) error
	GetSchedulerTask(taskID string) (*TaskState, error)
	ListSchedulerTasks() ([]*TaskState, error)
	UpdateTaskRun(taskID string, lastRun time.Time, status, lastError string, runCount, failCount int64, nextRun time.Time) error
	RecordTaskHistory(h *TaskHistory) error
}

// SQLiteDB implements DB for SQLite.
type SQLiteDB struct {
	db *sql.DB
}

// NewDatabase creates a database connection. Currently SQLite only.
func NewDatabase(dbType, path string) (DB, error) {
	switch strings.ToLower(dbType) {
	case "sqlite", "":
		return newSQLiteDB(path)
	default:
		return newSQLiteDB(path)
	}
}

func newSQLiteDB(path string) (*SQLiteDB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteDB{db: db}, nil
}

func ensureSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS pastes (
			id               TEXT PRIMARY KEY,
			title            TEXT NOT NULL DEFAULT 'Untitled',
			content          TEXT NOT NULL,
			language         TEXT NOT NULL DEFAULT 'text',
			visibility       INTEGER NOT NULL DEFAULT 0,
			expires_at       DATETIME,
			burn_after       INTEGER NOT NULL DEFAULT 0,
			delete_token_hash TEXT NOT NULL DEFAULT '',
			views            INTEGER NOT NULL DEFAULT 0,
			created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_visibility   ON pastes(visibility)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_created_at   ON pastes(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_expires_at   ON pastes(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_burn_after   ON pastes(burn_after)`,

		// Scheduler persistent state (PART 18).
		`CREATE TABLE IF NOT EXISTS scheduler_tasks (
			task_id      TEXT PRIMARY KEY,
			task_name    TEXT NOT NULL,
			schedule     TEXT NOT NULL,
			last_run     DATETIME,
			last_status  TEXT NOT NULL DEFAULT 'pending',
			last_error   TEXT NOT NULL DEFAULT '',
			next_run     DATETIME,
			run_count    INTEGER NOT NULL DEFAULT 0,
			fail_count   INTEGER NOT NULL DEFAULT 0,
			enabled      INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS scheduler_history (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id      TEXT NOT NULL,
			started_at   DATETIME NOT NULL,
			finished_at  DATETIME,
			status       TEXT NOT NULL,
			error_msg    TEXT NOT NULL DEFAULT '',
			duration_ms  INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sched_history_task ON scheduler_history(task_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sched_history_started ON scheduler_history(started_at)`,
	}

	// Schema updates — idempotent; ignore "already exists" errors.
	updates := []string{
		`ALTER TABLE pastes ADD COLUMN burn_after INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE pastes ADD COLUMN delete_token_hash TEXT NOT NULL DEFAULT ''`,
	}

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}

	for _, s := range updates {
		if _, err := db.Exec(s); err != nil {
			if !isAlreadyExists(err) {
				return fmt.Errorf("schema update: %w", err)
			}
		}
	}

	return nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate column") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "Duplicate column name")
}

// Close closes the database connection.
func (s *SQLiteDB) Close() error { return s.db.Close() }

// Type returns the database type name.
func (s *SQLiteDB) Type() string { return "sqlite" }

// Ping verifies the database connection is alive.
func (s *SQLiteDB) Ping() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.db.PingContext(ctx)
}

// CreatePaste inserts a new paste.
func (s *SQLiteDB) CreatePaste(p *model.Paste) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Title == "" {
		p.Title = "Untitled"
	}
	if p.Language == "" {
		p.Language = "text"
	}

	_, err := s.db.Exec(
		`INSERT INTO pastes
			(id, title, content, language, visibility, expires_at, burn_after, delete_token_hash, views, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Title, p.Content, p.Language, p.Visibility, p.ExpiresAt,
		p.BurnAfter, p.DeleteTokenHash, p.Views, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// GetPasteByID retrieves a paste by ID without touching views.
func (s *SQLiteDB) GetPasteByID(id string) (*model.Paste, error) {
	p := &model.Paste{}
	var expiresAt sql.NullTime

	err := s.db.QueryRow(
		`SELECT id, title, content, language, visibility, expires_at, burn_after, delete_token_hash, views, created_at, updated_at
		 FROM pastes WHERE id = ?`, id,
	).Scan(&p.ID, &p.Title, &p.Content, &p.Language, &p.Visibility,
		&expiresAt, &p.BurnAfter, &p.DeleteTokenHash, &p.Views, &p.CreatedAt, &p.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		p.ExpiresAt = &expiresAt.Time
	}
	return p, nil
}

// GetPublicPastes returns paginated public (visibility=0) non-expired pastes.
func (s *SQLiteDB) GetPublicPastes(page, limit int) ([]model.PasteListItem, int, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit
	now := time.Now()

	var total int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pastes WHERE visibility = 0 AND (expires_at IS NULL OR expires_at > ?)`,
		now,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, title, language, views, expires_at, burn_after, created_at
		 FROM pastes
		 WHERE visibility = 0 AND (expires_at IS NULL OR expires_at > ?)
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		now, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.PasteListItem
	for rows.Next() {
		var item model.PasteListItem
		var expiresAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.Title, &item.Language, &item.Views, &expiresAt, &item.BurnAfter, &item.CreatedAt); err != nil {
			return nil, 0, err
		}
		if expiresAt.Valid {
			item.ExpiresAt = &expiresAt.Time
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// IncrementPasteViews atomically increments the view counter.
func (s *SQLiteDB) IncrementPasteViews(id string) error {
	_, err := s.db.Exec(`UPDATE pastes SET views = views + 1, updated_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

// DeletePaste removes a paste by ID with no token check (admin / internal use).
func (s *SQLiteDB) DeletePaste(id string) error {
	result, err := s.db.Exec(`DELETE FROM pastes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("paste not found")
	}
	return nil
}

// DeletePasteByToken removes a paste only when the delete token hash matches.
func (s *SQLiteDB) DeletePasteByToken(id, deleteTokenHash string) error {
	result, err := s.db.Exec(
		`DELETE FROM pastes WHERE id = ? AND delete_token_hash = ?`,
		id, deleteTokenHash,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("paste not found or invalid token")
	}
	return nil
}

// DeleteExpiredPastes removes all pastes whose expiry has passed.
func (s *SQLiteDB) DeleteExpiredPastes() (int64, error) {
	result, err := s.db.Exec(
		`DELETE FROM pastes WHERE expires_at IS NOT NULL AND expires_at <= ?`,
		time.Now(),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteBurnedPastes removes pastes whose view count has reached their burn_after limit.
func (s *SQLiteDB) DeleteBurnedPastes() (int64, error) {
	result, err := s.db.Exec(
		`DELETE FROM pastes WHERE burn_after > 0 AND views >= burn_after`,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ── Scheduler operations (PART 18) ──────────────────────────────────────────

// UpsertSchedulerTask inserts or replaces a task's persistent state.
func (s *SQLiteDB) UpsertSchedulerTask(t *TaskState) error {
	var lastRun *time.Time
	if !t.LastRun.IsZero() {
		lastRun = &t.LastRun
	}
	var nextRun *time.Time
	if !t.NextRun.IsZero() {
		nextRun = &t.NextRun
	}
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO scheduler_tasks
			(task_id, task_name, schedule, last_run, last_status, last_error,
			 next_run, run_count, fail_count, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			task_name   = excluded.task_name,
			schedule    = excluded.schedule,
			last_run    = COALESCE(scheduler_tasks.last_run, excluded.last_run),
			last_status = COALESCE(NULLIF(scheduler_tasks.last_status,''), excluded.last_status),
			last_error  = COALESCE(NULLIF(scheduler_tasks.last_error,''),  excluded.last_error),
			next_run    = COALESCE(scheduler_tasks.next_run, excluded.next_run),
			run_count   = scheduler_tasks.run_count,
			fail_count  = scheduler_tasks.fail_count,
			enabled     = excluded.enabled`,
		t.TaskID, t.TaskName, t.Schedule, lastRun, t.LastStatus, t.LastError,
		nextRun, t.RunCount, t.FailCount, enabled,
	)
	return err
}

// GetSchedulerTask retrieves the persistent state for a single task.
func (s *SQLiteDB) GetSchedulerTask(taskID string) (*TaskState, error) {
	row := s.db.QueryRow(`
		SELECT task_id, task_name, schedule, last_run, last_status, last_error,
		       next_run, run_count, fail_count, enabled
		FROM   scheduler_tasks WHERE task_id = ?`, taskID)
	return scanTaskState(row)
}

// ListSchedulerTasks returns all registered tasks ordered by task_id.
func (s *SQLiteDB) ListSchedulerTasks() ([]*TaskState, error) {
	rows, err := s.db.Query(`
		SELECT task_id, task_name, schedule, last_run, last_status, last_error,
		       next_run, run_count, fail_count, enabled
		FROM   scheduler_tasks ORDER BY task_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*TaskState
	for rows.Next() {
		t, err := scanTaskState(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// UpdateTaskRun records the outcome of a completed task execution.
func (s *SQLiteDB) UpdateTaskRun(
	taskID string, lastRun time.Time, status, lastError string,
	runCount, failCount int64, nextRun time.Time,
) error {
	var nextRunPtr *time.Time
	if !nextRun.IsZero() {
		nextRunPtr = &nextRun
	}
	_, err := s.db.Exec(`
		UPDATE scheduler_tasks
		SET last_run = ?, last_status = ?, last_error = ?,
		    run_count = ?, fail_count = ?, next_run = ?
		WHERE task_id = ?`,
		lastRun, status, lastError, runCount, failCount, nextRunPtr, taskID,
	)
	return err
}

// RecordTaskHistory appends a history entry for a task execution.
func (s *SQLiteDB) RecordTaskHistory(h *TaskHistory) error {
	_, err := s.db.Exec(`
		INSERT INTO scheduler_history
			(task_id, started_at, finished_at, status, error_msg, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?)`,
		h.TaskID, h.StartedAt, h.FinishedAt, h.Status, h.ErrorMsg, h.DurationMS,
	)
	return err
}

// scanTaskState scans a row into a TaskState.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTaskState(row rowScanner) (*TaskState, error) {
	t := &TaskState{}
	var lastRun, nextRun *time.Time
	var enabled int
	err := row.Scan(
		&t.TaskID, &t.TaskName, &t.Schedule,
		&lastRun, &t.LastStatus, &t.LastError,
		&nextRun, &t.RunCount, &t.FailCount, &enabled,
	)
	if err != nil {
		return nil, err
	}
	if lastRun != nil {
		t.LastRun = *lastRun
	}
	if nextRun != nil {
		t.NextRun = *nextRun
	}
	t.Enabled = enabled != 0
	return t, nil
}
