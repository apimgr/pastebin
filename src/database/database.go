package database

import (
	"context"
	crand "crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/model"
	_ "modernc.org/sqlite"
)

// APITokenRecord represents a stored resource-owner token row (PART 11).
// The raw token is never stored — only the SHA-256 hex digest.
type APITokenRecord struct {
	ID            int64
	TokenPrefix   string
	ResourceType  string
	ResourceID    string
	CreatedAt     time.Time
	ExpiresAt     *time.Time
	LastUsedAt    *time.Time
	RevokedAt     *time.Time
	RevokedReason string
}

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
	SetTaskEnabled(taskID string, enabled bool) error
	RecordTaskHistory(h *TaskHistory) error
	ListTaskHistory(taskID string, limit int) ([]*TaskHistory, error)

	// API token operations (PART 11).
	// CreateAPIToken stores a new resource-owner token. tokenHash is the
	// SHA-256 hex digest; tokenPrefix is the first 12 chars of the raw token.
	CreateAPIToken(tokenHash, tokenPrefix, resourceType, resourceID string, expiresAt *time.Time) error
	// VerifyAPIToken checks that tokenHash is active and belongs to the given
	// resource, then updates last_used_at. Returns an error if invalid/revoked.
	VerifyAPIToken(tokenHash [32]byte, resourceType, resourceID string) error
	// ValidateAPIToken checks that tokenHash is active for the given resource_type
	// (any resource_id). Used when reusing a token on paste creation.
	ValidateAPIToken(tokenHash [32]byte, resourceType string) error
	// RevokeAPIToken marks the token with the given prefix as revoked.
	RevokeAPIToken(prefix, reason string) error
	// ListAPITokens returns all non-revoked token records (token_hash omitted).
	ListAPITokens() ([]*APITokenRecord, error)
	// DeleteExpiredAPITokens removes rows that are expired or revoked.
	DeleteExpiredAPITokens() (int64, error)

	// App secrets — server-wide HMAC / signing keys stored in the DB (PART 11).
	// EnsureAppSecret returns the raw bytes for key, generating 32 random bytes on
	// first call. The value is stored base64-encoded and never returned in API responses.
	EnsureAppSecret(key string) ([]byte, error)

	// CountPastes returns the total number of pastes stored (for healthz stats).
	CountPastes() (int64, error)

	// Coordinated-disclosure security reports (PART 11). Report bodies are
	// encrypted at rest; only sanitized triage metadata is stored in the clear.
	CreateSecurityReport(r *SecurityReport) error
	GetSecurityReport(trackingID string) (*SecurityReport, error)
	UpdateSecurityReportStatus(trackingID, status, maintainerComment string) error
	ListDisclosedSecurityReports() ([]*SecurityReport, error)
	MarkSecurityReportTokenUsed(trackingID string, at time.Time) error

	// Project PGP keypair metadata (PART 11 → GPG Keypair Management). The keys
	// themselves live on disk; only fingerprint/expiry/publish state is stored.
	GetSecurityKeypair() (*SecurityKeypair, error)
	UpsertSecurityKeypair(kp *SecurityKeypair) error
}

// Query timeout constants per PART 10.
const (
	// dbReadTimeout is for simple SELECT queries.
	dbReadTimeout = 5 * time.Second
	// dbWriteTimeout is for INSERT, UPDATE, DELETE.
	dbWriteTimeout = 10 * time.Second
	// dbComplexTimeout is for multi-step or JOIN queries.
	dbComplexTimeout = 15 * time.Second
	// dbBulkTimeout is for bulk delete / migration operations.
	dbBulkTimeout = 60 * time.Second
)

// dbCtx returns a context with the given timeout rooted at Background.
func dbCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
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

	// Connection pool settings (PART 10).
	// SQLite WAL mode supports 1 writer + concurrent readers; 5 open conns is safe.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

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

		// App-level server secrets (PART 11): installation_secret, cookie_signing_key,
		// csrf_token_secret. 32 bytes each, stored base64-encoded. Never returned
		// in any API response; always included in backups.
		`CREATE TABLE IF NOT EXISTS app_secrets (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,

		// Config key-value storage (PART 11 / PART 5): mirrors YAML structure as flat keys.
		`CREATE TABLE IF NOT EXISTS config (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			type       TEXT NOT NULL DEFAULT 'string',
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_config_key ON config(key)`,

		// Config metadata: single-row version counter, incremented by trigger on change.
		`CREATE TABLE IF NOT EXISTS config_meta (
			id         INTEGER PRIMARY KEY CHECK (id = 1),
			version    INTEGER NOT NULL DEFAULT 1,
			updated_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
		)`,
		`INSERT OR IGNORE INTO config_meta (id, version) VALUES (1, 1)`,
		// SQLite requires one event per trigger — one each for INSERT, UPDATE, DELETE.
		`CREATE TRIGGER IF NOT EXISTS config_version_bump_ins
			AFTER INSERT ON config
			BEGIN
				UPDATE config_meta SET version = version + 1, updated_at = strftime('%s','now') WHERE id = 1;
			END`,
		`CREATE TRIGGER IF NOT EXISTS config_version_bump_upd
			AFTER UPDATE ON config
			BEGIN
				UPDATE config_meta SET version = version + 1, updated_at = strftime('%s','now') WHERE id = 1;
			END`,
		`CREATE TRIGGER IF NOT EXISTS config_version_bump_del
			AFTER DELETE ON config
			BEGIN
				UPDATE config_meta SET version = version + 1, updated_at = strftime('%s','now') WHERE id = 1;
			END`,

		// Rate limiting: sliding-window counters per IP/key (PART 11).
		`CREATE TABLE IF NOT EXISTS rate_limits (
			key          TEXT PRIMARY KEY,
			count        INTEGER NOT NULL DEFAULT 1,
			window_start INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			updated_at   INTEGER NOT NULL DEFAULT (strftime('%s', 'now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_rate_limits_window ON rate_limits(window_start)`,

		// Audit log: config changes, security events, request log (PART 11).
		`CREATE TABLE IF NOT EXISTS audit_log (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp   INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			level       TEXT NOT NULL DEFAULT 'info',
			category    TEXT NOT NULL,
			action      TEXT NOT NULL,
			actor_ip    TEXT,
			target_type TEXT,
			target_id   TEXT,
			details     TEXT,
			success     INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_category  ON audit_log(category)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_actor_ip  ON audit_log(actor_ip)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_target    ON audit_log(target_type, target_id)`,

		// Backup history and metadata (PART 21).
		`CREATE TABLE IF NOT EXISTS backups (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			filename   TEXT NOT NULL UNIQUE,
			filepath   TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			type       TEXT NOT NULL DEFAULT 'auto',
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			checksum   TEXT,
			notes      TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at)`,

		// API tokens: server-generated resource-owner tokens (PART 11).
		// The server.token (server.yml) is NOT stored here — validated directly from config.
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash    TEXT NOT NULL UNIQUE,
			token_prefix  TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id   TEXT NOT NULL,
			created_at    INTEGER NOT NULL DEFAULT (strftime('%s', 'now')),
			expires_at    INTEGER,
			last_used_at  INTEGER,
			revoked_at    INTEGER,
			revoked_reason TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_hash     ON api_tokens(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_prefix   ON api_tokens(token_prefix)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_resource ON api_tokens(resource_type, resource_id)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_active   ON api_tokens(revoked_at) WHERE revoked_at IS NULL`,

		// Coordinated-disclosure security reports (PART 11). The vulnerability
		// content lives only in encrypted_body (PGP or AES-256-GCM); the other
		// columns hold sanitized triage metadata with no PII or report detail.
		`CREATE TABLE IF NOT EXISTS security_reports (
			tracking_id        TEXT PRIMARY KEY,
			status             TEXT NOT NULL DEFAULT 'Received',
			severity           TEXT NOT NULL DEFAULT '',
			component          TEXT NOT NULL DEFAULT '',
			encrypted_body     BLOB NOT NULL,
			enc_method         TEXT NOT NULL DEFAULT 'aes-256-gcm',
			credit_preference  TEXT NOT NULL DEFAULT 'no',
			credit_name        TEXT NOT NULL DEFAULT '',
			token_hash         TEXT NOT NULL DEFAULT '',
			maintainer_comment TEXT NOT NULL DEFAULT '',
			disclosure_days    INTEGER NOT NULL DEFAULT 90,
			created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			disclosed_at       DATETIME,
			token_last_used    DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_secreports_status  ON security_reports(status)`,
		`CREATE INDEX IF NOT EXISTS idx_secreports_created ON security_reports(created_at)`,

		// Project PGP keypair metadata (PART 11 → GPG Keypair Management). Single
		// row (id = 1); the key material lives on disk, never in the DB. The
		// keyservers_published column holds a JSON array of {url, published_at}.
		`CREATE TABLE IF NOT EXISTS security_keypair (
			id                   INTEGER PRIMARY KEY CHECK (id = 1),
			fingerprint          TEXT NOT NULL DEFAULT '',
			created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at           DATETIME,
			last_rotated_at      DATETIME,
			keyservers_published TEXT NOT NULL DEFAULT '',
			revoked              INTEGER NOT NULL DEFAULT 0
		)`,
	}

	// Schema updates — idempotent; ignore "already exists" errors.
	updates := []string{
		`ALTER TABLE pastes ADD COLUMN burn_after INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE pastes ADD COLUMN delete_token_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE security_reports ADD COLUMN token_last_used DATETIME`,
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

	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
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

	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	err := s.db.QueryRowContext(ctx,
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

	ctx, cancel := dbCtx(dbComplexTimeout)
	defer cancel()

	var total int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pastes WHERE visibility = 0 AND (expires_at IS NULL OR expires_at > ?)`,
		now,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
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
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `UPDATE pastes SET views = views + 1, updated_at = ? WHERE id = ?`, time.Now(), id)
	return err
}

// DeletePaste removes a paste by ID with no token check (admin / internal use).
func (s *SQLiteDB) DeletePaste(id string) error {
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	result, err := s.db.ExecContext(ctx, `DELETE FROM pastes WHERE id = ?`, id)
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
// The comparison is performed in constant time to prevent timing-based token oracle attacks.
func (s *SQLiteDB) DeletePasteByToken(id, deleteTokenHash string) error {
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	var storedHash string
	err := s.db.QueryRowContext(ctx, `SELECT delete_token_hash FROM pastes WHERE id = ?`, id).Scan(&storedHash)
	if err == sql.ErrNoRows {
		return fmt.Errorf("paste not found or invalid token")
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(deleteTokenHash)) != 1 {
		return fmt.Errorf("paste not found or invalid token")
	}
	ctx2, cancel2 := dbCtx(dbWriteTimeout)
	defer cancel2()
	_, err = s.db.ExecContext(ctx2, `DELETE FROM pastes WHERE id = ?`, id)
	return err
}

// DeleteExpiredPastes removes all pastes whose expiry has passed.
func (s *SQLiteDB) DeleteExpiredPastes() (int64, error) {
	ctx, cancel := dbCtx(dbBulkTimeout)
	defer cancel()
	result, err := s.db.ExecContext(ctx,
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
	ctx, cancel := dbCtx(dbBulkTimeout)
	defer cancel()
	result, err := s.db.ExecContext(ctx,
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
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
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
	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	row := s.db.QueryRowContext(ctx, `
		SELECT task_id, task_name, schedule, last_run, last_status, last_error,
		       next_run, run_count, fail_count, enabled
		FROM   scheduler_tasks WHERE task_id = ?`, taskID)
	return scanTaskState(row)
}

// ListSchedulerTasks returns all registered tasks ordered by task_id.
func (s *SQLiteDB) ListSchedulerTasks() ([]*TaskState, error) {
	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
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

// SetTaskEnabled enables or disables a scheduled task in the persistent store.
func (s *SQLiteDB) SetTaskEnabled(taskID string, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`UPDATE scheduler_tasks SET enabled = ? WHERE task_id = ?`,
		enabledInt, taskID,
	)
	return err
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
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
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
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scheduler_history
			(task_id, started_at, finished_at, status, error_msg, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?)`,
		h.TaskID, h.StartedAt, h.FinishedAt, h.Status, h.ErrorMsg, h.DurationMS,
	)
	return err
}

// ListTaskHistory returns the most recent history entries for a task, newest first.
// If limit <= 0 it defaults to 20.
func (s *SQLiteDB) ListTaskHistory(taskID string, limit int) ([]*TaskHistory, error) {
	if limit <= 0 {
		limit = 20
	}
	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `
		SELECT task_id, started_at, finished_at, status, error_msg, duration_ms
		FROM   scheduler_history
		WHERE  task_id = ?
		ORDER  BY started_at DESC
		LIMIT  ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TaskHistory
	for rows.Next() {
		h := &TaskHistory{}
		if err := rows.Scan(&h.TaskID, &h.StartedAt, &h.FinishedAt, &h.Status, &h.ErrorMsg, &h.DurationMS); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ── App secrets (PART 11) ────────────────────────────────────────────────────

// EnsureAppSecret returns the raw bytes for key.
// On first call for a given key, 32 random bytes are generated, stored
// base64-encoded in app_secrets, and returned. On subsequent calls the stored
// value is decoded and returned unchanged.
func (s *SQLiteDB) EnsureAppSecret(key string) ([]byte, error) {
	// Attempt to fetch existing value.
	var encoded string
	rCtx, rCancel := dbCtx(dbReadTimeout)
	defer rCancel()
	err := s.db.QueryRowContext(rCtx,
		`SELECT value FROM app_secrets WHERE key = ?`, key,
	).Scan(&encoded)
	if err == nil {
		raw, decErr := base64.StdEncoding.DecodeString(encoded)
		if decErr != nil {
			return nil, fmt.Errorf("app_secrets: decode %q: %w", key, decErr)
		}
		return raw, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("app_secrets: query %q: %w", key, err)
	}

	// Not found — generate 32 random bytes.
	var buf [32]byte
	if _, err := crand.Read(buf[:]); err != nil {
		return nil, fmt.Errorf("app_secrets: generate %q: %w", key, err)
	}
	encoded = base64.StdEncoding.EncodeToString(buf[:])
	now := time.Now()
	wCtx, wCancel := dbCtx(dbWriteTimeout)
	defer wCancel()
	_, err = s.db.ExecContext(wCtx,
		`INSERT INTO app_secrets (key, value, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		key, encoded, now, now,
	)
	if err != nil {
		// Race: another goroutine may have inserted simultaneously.
		var existing string
		rCtx2, rCancel2 := dbCtx(dbReadTimeout)
		defer rCancel2()
		if qErr := s.db.QueryRowContext(rCtx2,
			`SELECT value FROM app_secrets WHERE key = ?`, key,
		).Scan(&existing); qErr == nil {
			raw, decErr := base64.StdEncoding.DecodeString(existing)
			if decErr != nil {
				return nil, fmt.Errorf("app_secrets: decode %q after race: %w", key, decErr)
			}
			return raw, nil
		}
		return nil, fmt.Errorf("app_secrets: insert %q: %w", key, err)
	}
	return buf[:], nil
}

// CountPastes returns the total number of rows in the paste table.
func (s *SQLiteDB) CountPastes() (int64, error) {
	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pastes`).Scan(&count)
	return count, err
}

// ── API Token operations (PART 11) ──────────────────────────────────────────

// CreateAPIToken inserts a new resource-owner token record.
// tokenHash must be the SHA-256 hex digest of the raw token.
// tokenPrefix must be the first 12 characters of the raw token.
func (s *SQLiteDB) CreateAPIToken(tokenHash, tokenPrefix, resourceType, resourceID string, expiresAt *time.Time) error {
	now := time.Now().Unix()
	var expiresUnix *int64
	if expiresAt != nil {
		v := expiresAt.Unix()
		expiresUnix = &v
	}
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens
			(token_hash, token_prefix, resource_type, resource_id, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, tokenPrefix, resourceType, resourceID, now, expiresUnix,
	)
	return err
}

// VerifyAPIToken checks that the token hash matches an active, non-expired row for the
// given resource, then updates last_used_at. Constant-time hash comparison prevents
// timing oracle attacks.
func (s *SQLiteDB) VerifyAPIToken(tokenHash [32]byte, resourceType, resourceID string) error {
	hashHex := hex.EncodeToString(tokenHash[:])
	now := time.Now().Unix()

	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	var storedHash string
	var id int64
	var expiresAt sql.NullInt64
	var revokedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id, token_hash, expires_at, revoked_at
		 FROM api_tokens
		 WHERE token_hash = ? AND resource_type = ? AND resource_id = ?`,
		hashHex, resourceType, resourceID,
	).Scan(&id, &storedHash, &expiresAt, &revokedAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("paste not found or invalid token")
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashHex)) != 1 {
		return fmt.Errorf("paste not found or invalid token")
	}
	if revokedAt.Valid {
		return fmt.Errorf("paste not found or invalid token")
	}
	if expiresAt.Valid && expiresAt.Int64 <= now {
		return fmt.Errorf("paste not found or invalid token")
	}

	wCtx, wCancel := dbCtx(dbWriteTimeout)
	defer wCancel()
	_, _ = s.db.ExecContext(wCtx,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, id,
	)
	return nil
}

// ValidateAPIToken checks that the token hash is active and not expired for
// any row with the given resource_type. Used to verify a reusable owner token
// before linking it to a new paste via CreateAPIToken.
func (s *SQLiteDB) ValidateAPIToken(tokenHash [32]byte, resourceType string) error {
	hashHex := hex.EncodeToString(tokenHash[:])
	now := time.Now().Unix()

	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	var storedHash string
	var expiresAt sql.NullInt64
	var revokedAt sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT token_hash, expires_at, revoked_at
		 FROM api_tokens
		 WHERE token_hash = ? AND resource_type = ?
		 ORDER BY created_at DESC LIMIT 1`,
		hashHex, resourceType,
	).Scan(&storedHash, &expiresAt, &revokedAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("token not found")
	}
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(hashHex)) != 1 {
		return fmt.Errorf("token not found")
	}
	if revokedAt.Valid {
		return fmt.Errorf("token has been revoked")
	}
	if expiresAt.Valid && expiresAt.Int64 <= now {
		return fmt.Errorf("token has expired")
	}
	return nil
}

// RevokeAPIToken marks the token whose prefix matches as revoked.
func (s *SQLiteDB) RevokeAPIToken(prefix, reason string) error {
	now := time.Now().Unix()
	ctx, cancel := dbCtx(dbWriteTimeout)
	defer cancel()
	result, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET revoked_at = ?, revoked_reason = ?
		 WHERE token_prefix = ? AND revoked_at IS NULL`,
		now, reason, prefix,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("token not found or already revoked")
	}
	return nil
}

// ListAPITokens returns all non-revoked token records, ordered newest first.
// The token_hash is intentionally omitted from the returned struct.
func (s *SQLiteDB) ListAPITokens() ([]*APITokenRecord, error) {
	ctx, cancel := dbCtx(dbReadTimeout)
	defer cancel()
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, token_prefix, resource_type, resource_id,
		        created_at, expires_at, last_used_at
		 FROM api_tokens
		 WHERE revoked_at IS NULL
		 ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*APITokenRecord
	for rows.Next() {
		rec := &APITokenRecord{}
		var createdAt int64
		var expiresAt, lastUsedAt sql.NullInt64
		if err := rows.Scan(
			&rec.ID, &rec.TokenPrefix, &rec.ResourceType, &rec.ResourceID,
			&createdAt, &expiresAt, &lastUsedAt,
		); err != nil {
			return nil, err
		}
		rec.CreatedAt = time.Unix(createdAt, 0)
		if expiresAt.Valid {
			t := time.Unix(expiresAt.Int64, 0)
			rec.ExpiresAt = &t
		}
		if lastUsedAt.Valid {
			t := time.Unix(lastUsedAt.Int64, 0)
			rec.LastUsedAt = &t
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// DeleteExpiredAPITokens removes rows that are expired or revoked.
func (s *SQLiteDB) DeleteExpiredAPITokens() (int64, error) {
	now := time.Now().Unix()
	ctx, cancel := dbCtx(dbBulkTimeout)
	defer cancel()
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM api_tokens
		 WHERE revoked_at IS NOT NULL
		    OR (expires_at IS NOT NULL AND expires_at <= ?)`,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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
