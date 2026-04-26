package models

import (
	"time"
)

// User represents a registered user
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // Never expose in JSON
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Token represents an API access token
type Token struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	UserID    string    `json:"user_id"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Paste represents a paste/snippet
type Paste struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	Language  string     `json:"language"`
	IsPublic  bool       `json:"is_public"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	UserID    *string    `json:"user_id,omitempty"`
	Views     int        `json:"views"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PasteListItem is a minimal paste for list views
type PasteListItem struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Language  string    `json:"language"`
	Views     int       `json:"views"`
	CreatedAt time.Time `json:"created_at"`
}

// UserResponse is the safe user data to return in API responses
type UserResponse struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ToResponse converts User to safe UserResponse
func (u *User) ToResponse() UserResponse {
	return UserResponse{
		ID:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		CreatedAt: u.CreatedAt,
	}
}

// ToPasteListItem converts Paste to minimal list item
func (p *Paste) ToListItem() PasteListItem {
	return PasteListItem{
		ID:        p.ID,
		Title:     p.Title,
		Language:  p.Language,
		Views:     p.Views,
		CreatedAt: p.CreatedAt,
	}
}
