package model

import (
	"time"
)

// Visibility constants for Paste
const (
	// listed in /recent, accessible to all
	VisibilityPublic = 0
	// accessible via direct link only
	VisibilityUnlisted = 1
)

// Paste represents a stored paste/snippet.
type Paste struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Content         string     `json:"content,omitempty"`      // omitted in list views
	ContentType     string     `json:"content_type,omitempty"` // detected MIME type; empty = plain text
	Language        string     `json:"language"`
	Visibility      int        `json:"visibility"` // 0=public, 1=unlisted
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	BurnAfter       int        `json:"burn_after"` // 0=disabled, 1-9999 views then delete
	DeleteTokenHash string     `json:"-"`          // SHA-256(delete_token), never in JSON
	Views           int        `json:"views"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PasteListItem is a minimal paste for list and recent views.
type PasteListItem struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Language  string     `json:"language"`
	Views     int        `json:"views"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	BurnAfter int        `json:"burn_after"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateResponse is returned once on paste creation and includes the plaintext owner token.
// The OwnerToken is the raw tok_... value shown exactly once — it is never retrievable again.
type CreateResponse struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Language   string     `json:"language"`
	Visibility int        `json:"visibility"`
	BurnAfter  int        `json:"burn_after"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Views      int        `json:"views"`
	CreatedAt  time.Time  `json:"created_at"`
	Link       string     `json:"link"`
	OwnerToken string     `json:"owner_token"` // plaintext tok_... shown once, never retrievable again
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
