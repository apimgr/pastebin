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

// DB is the database interface for paste operations.
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
