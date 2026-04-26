package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/models"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// DB is the database interface
type DB interface {
	Close() error
	Type() string

	// User methods
	CreateUser(user *models.User) error
	GetUserByID(id string) (*models.User, error)
	GetUserByUsername(username string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByUsernameOrEmail(identifier string) (*models.User, error)

	// Token methods
	CreateToken(token *models.Token) error
	GetTokenByValue(tokenValue string) (*models.Token, error)
	GetTokensByUserID(userID string) ([]models.Token, error)
	DeleteToken(id, userID string) error

	// Paste methods
	CreatePaste(paste *models.Paste) error
	GetPasteByID(id string) (*models.Paste, error)
	GetPublicPastes(page, limit int) ([]models.PasteListItem, int, error)
	GetPastesByUserID(userID string, page, limit int) ([]models.PasteListItem, int, error)
	IncrementPasteViews(id string) error
	DeletePaste(id, userID string) error
	DeleteExpiredPastes() (int64, error)
}

// SQLiteDB implements DB interface for SQLite
type SQLiteDB struct {
	db *sql.DB
}

// NewDatabase creates a new database connection based on config
func NewDatabase(cfg *config.DatabaseConfig) (DB, error) {
	switch cfg.Type {
	case "sqlite":
		return newSQLiteDB(cfg.Path)
	case "postgres", "postgresql":
		return nil, fmt.Errorf("PostgreSQL support not yet implemented - use sqlite")
	case "mysql", "mariadb":
		return nil, fmt.Errorf("MySQL support not yet implemented - use sqlite")
	default:
		return newSQLiteDB(cfg.Path)
	}
}

func newSQLiteDB(path string) (*SQLiteDB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path+"?_foreign_keys=on")
	if err != nil {
		return nil, err
	}

	// Initialize tables
	if err := initSQLiteTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteDB{db: db}, nil
}

func initSQLiteTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			email TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			id TEXT PRIMARY KEY,
			name TEXT DEFAULT 'Default Token',
			token TEXT NOT NULL UNIQUE,
			user_id TEXT NOT NULL,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS pastes (
			id TEXT PRIMARY KEY,
			title TEXT DEFAULT 'Untitled',
			content TEXT NOT NULL,
			language TEXT DEFAULT 'text',
			is_public INTEGER DEFAULT 1,
			expires_at DATETIME,
			user_id TEXT,
			views INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_user_id ON pastes(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_is_public ON pastes(is_public)`,
		`CREATE INDEX IF NOT EXISTS idx_pastes_created_at ON pastes(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token)`,
	}

	for _, table := range tables {
		if _, err := db.Exec(table); err != nil {
			return fmt.Errorf("failed to create table: %w", err)
		}
	}

	return nil
}

func (s *SQLiteDB) Close() error {
	return s.db.Close()
}

func (s *SQLiteDB) Type() string {
	return "sqlite"
}

// User methods
func (s *SQLiteDB) CreateUser(user *models.User) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	_, err := s.db.Exec(
		`INSERT INTO users (id, username, email, password, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.Email, user.Password, user.CreatedAt, user.UpdatedAt,
	)
	return err
}

func (s *SQLiteDB) GetUserByID(id string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(
		`SELECT id, username, email, password, created_at, updated_at FROM users WHERE id = ?`,
		id,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (s *SQLiteDB) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(
		`SELECT id, username, email, password, created_at, updated_at FROM users WHERE username = ?`,
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (s *SQLiteDB) GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(
		`SELECT id, username, email, password, created_at, updated_at FROM users WHERE email = ?`,
		email,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

func (s *SQLiteDB) GetUserByUsernameOrEmail(identifier string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(
		`SELECT id, username, email, password, created_at, updated_at FROM users WHERE username = ? OR email = ?`,
		identifier, identifier,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return user, err
}

// Token methods
func (s *SQLiteDB) CreateToken(token *models.Token) error {
	if token.ID == "" {
		token.ID = uuid.New().String()
	}
	if token.Token == "" {
		token.Token = uuid.New().String()
	}
	now := time.Now()
	token.CreatedAt = now
	token.UpdatedAt = now
	token.IsActive = true

	_, err := s.db.Exec(
		`INSERT INTO tokens (id, name, token, user_id, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.Name, token.Token, token.UserID, token.IsActive, token.CreatedAt, token.UpdatedAt,
	)
	return err
}

func (s *SQLiteDB) GetTokenByValue(tokenValue string) (*models.Token, error) {
	token := &models.Token{}
	err := s.db.QueryRow(
		`SELECT id, name, token, user_id, is_active, created_at, updated_at FROM tokens WHERE token = ? AND is_active = 1`,
		tokenValue,
	).Scan(&token.ID, &token.Name, &token.Token, &token.UserID, &token.IsActive, &token.CreatedAt, &token.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return token, err
}

func (s *SQLiteDB) GetTokensByUserID(userID string) ([]models.Token, error) {
	rows, err := s.db.Query(
		`SELECT id, name, token, user_id, is_active, created_at, updated_at FROM tokens WHERE user_id = ? ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []models.Token
	for rows.Next() {
		var t models.Token
		if err := rows.Scan(&t.ID, &t.Name, &t.Token, &t.UserID, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (s *SQLiteDB) DeleteToken(id, userID string) error {
	result, err := s.db.Exec(`DELETE FROM tokens WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

// Paste methods
func (s *SQLiteDB) CreatePaste(paste *models.Paste) error {
	now := time.Now()
	paste.CreatedAt = now
	paste.UpdatedAt = now
	if paste.Title == "" {
		paste.Title = "Untitled"
	}
	if paste.Language == "" {
		paste.Language = "text"
	}

	_, err := s.db.Exec(
		`INSERT INTO pastes (id, title, content, language, is_public, expires_at, user_id, views, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		paste.ID, paste.Title, paste.Content, paste.Language, paste.IsPublic, paste.ExpiresAt, paste.UserID, paste.Views, paste.CreatedAt, paste.UpdatedAt,
	)
	return err
}

func (s *SQLiteDB) GetPasteByID(id string) (*models.Paste, error) {
	paste := &models.Paste{}
	var userID sql.NullString
	var expiresAt sql.NullTime

	err := s.db.QueryRow(
		`SELECT id, title, content, language, is_public, expires_at, user_id, views, created_at, updated_at FROM pastes WHERE id = ?`,
		id,
	).Scan(&paste.ID, &paste.Title, &paste.Content, &paste.Language, &paste.IsPublic, &expiresAt, &userID, &paste.Views, &paste.CreatedAt, &paste.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if userID.Valid {
		paste.UserID = &userID.String
	}
	if expiresAt.Valid {
		paste.ExpiresAt = &expiresAt.Time
	}

	return paste, nil
}

func (s *SQLiteDB) GetPublicPastes(page, limit int) ([]models.PasteListItem, int, error) {
	offset := (page - 1) * limit
	now := time.Now()

	// Get total count
	var total int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pastes WHERE is_public = 1 AND (expires_at IS NULL OR expires_at > ?)`,
		now,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get pastes
	rows, err := s.db.Query(
		`SELECT id, title, language, views, created_at FROM pastes WHERE is_public = 1 AND (expires_at IS NULL OR expires_at > ?) ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		now, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var pastes []models.PasteListItem
	for rows.Next() {
		var p models.PasteListItem
		if err := rows.Scan(&p.ID, &p.Title, &p.Language, &p.Views, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		pastes = append(pastes, p)
	}
	return pastes, total, rows.Err()
}

func (s *SQLiteDB) GetPastesByUserID(userID string, page, limit int) ([]models.PasteListItem, int, error) {
	offset := (page - 1) * limit

	// Get total count
	var total int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pastes WHERE user_id = ?`,
		userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// Get pastes
	rows, err := s.db.Query(
		`SELECT id, title, language, views, created_at FROM pastes WHERE user_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var pastes []models.PasteListItem
	for rows.Next() {
		var p models.PasteListItem
		if err := rows.Scan(&p.ID, &p.Title, &p.Language, &p.Views, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		pastes = append(pastes, p)
	}
	return pastes, total, rows.Err()
}

func (s *SQLiteDB) IncrementPasteViews(id string) error {
	_, err := s.db.Exec(`UPDATE pastes SET views = views + 1 WHERE id = ?`, id)
	return err
}

func (s *SQLiteDB) DeletePaste(id, userID string) error {
	result, err := s.db.Exec(`DELETE FROM pastes WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("paste not found")
	}
	return nil
}

func (s *SQLiteDB) DeleteExpiredPastes() (int64, error) {
	result, err := s.db.Exec(`DELETE FROM pastes WHERE expires_at IS NOT NULL AND expires_at <= ?`, time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
