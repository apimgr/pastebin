package model

import (
	"time"
)

// Visibility constants for Paste
const (
	VisibilityPublic   = 0 // listed in /recent, accessible to all
	VisibilityUnlisted = 1 // accessible via direct link only
)

// Paste represents a stored paste/snippet.
type Paste struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Content         string     `json:"content,omitempty"` // omitted in list views
	Language        string     `json:"language"`
	Visibility      int        `json:"visibility"` // 0=public, 1=unlisted
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	BurnAfter       int        `json:"burn_after"`        // 0=disabled, 1-9999 views then delete
	DeleteTokenHash string     `json:"-"`                 // SHA-256(delete_token), never in JSON
	Views           int        `json:"views"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PasteListItem is a minimal paste for list and recent views.
type PasteListItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Language  string    `json:"language"`
	Views     int       `json:"views"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	BurnAfter int       `json:"burn_after"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateResponse is returned once on paste creation and includes the plaintext delete token.
type CreateResponse struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Language    string     `json:"language"`
	Visibility  int        `json:"visibility"`
	BurnAfter   int        `json:"burn_after"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Views       int        `json:"views"`
	CreatedAt   time.Time  `json:"created_at"`
	Link        string     `json:"link"`
	DeleteToken string     `json:"delete_token"` // plaintext, shown once
}

// ToListItem converts a Paste to its minimal list representation.
func (p *Paste) ToListItem() PasteListItem {
	return PasteListItem{
		ID:        p.ID,
		Title:     p.Title,
		Language:  p.Language,
		Views:     p.Views,
		ExpiresAt: p.ExpiresAt,
		BurnAfter: p.BurnAfter,
		CreatedAt: p.CreatedAt,
	}
}
