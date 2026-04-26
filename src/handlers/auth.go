package handlers

import (
	"encoding/json"
	"net/http"

	auth2 "github.com/apimgr/pastebin/src/auth"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/models"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AuthHandler struct {
	db          database.DB
	authService *auth2.AuthService
}

func NewAuthHandler(db database.DB, authService *auth2.AuthService) *AuthHandler {
	return &AuthHandler{
		db:          db,
		authService: authService,
	}
}

// Register handles user registration
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Email == "" || req.Password == "" {
		jsonError(w, "Username, email, and password are required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 6 {
		jsonError(w, "Password must be at least 6 characters long", http.StatusBadRequest)
		return
	}

	user, token, err := h.authService.Register(req.Username, req.Email, req.Password)
	if err != nil {
		if err == auth2.ErrUserExists {
			jsonError(w, "User already exists", http.StatusConflict)
			return
		}
		jsonError(w, "Failed to register user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "User registered successfully",
		"user":    user.ToResponse(),
		"token":   token,
	})
}

// Login handles user authentication
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Username == "" || req.Password == "" {
		jsonError(w, "Username and password are required", http.StatusBadRequest)
		return
	}

	user, token, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		if err == auth2.ErrInvalidCredentials {
			jsonError(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}
		jsonError(w, "Login failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Login successful",
		"user":    user.ToResponse(),
		"token":   token,
	})
}

// GetMe returns the current user info
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	user := auth2.GetUserFromContext(r)
	if user == nil {
		jsonError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": user.ToResponse(),
	})
}

// CreateToken creates a new API token
func (h *AuthHandler) CreateToken(w http.ResponseWriter, r *http.Request) {
	user := auth2.GetUserFromContext(r)
	if user == nil {
		jsonError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Name == "" {
		req.Name = "Default Token"
	}

	token := &models.Token{
		ID:     uuid.New().String(),
		Name:   req.Name,
		Token:  uuid.New().String(),
		UserID: user.ID,
	}

	if err := h.db.CreateToken(token); err != nil {
		jsonError(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "API token created successfully",
		"token": map[string]interface{}{
			"id":         token.ID,
			"name":       token.Name,
			"token":      token.Token,
			"created_at": token.CreatedAt,
		},
	})
}

// ListTokens returns all tokens for the current user
func (h *AuthHandler) ListTokens(w http.ResponseWriter, r *http.Request) {
	user := auth2.GetUserFromContext(r)
	if user == nil {
		jsonError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	tokens, err := h.db.GetTokensByUserID(user.ID)
	if err != nil {
		jsonError(w, "Failed to fetch tokens", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tokens": tokens,
	})
}

// DeleteToken deletes an API token
func (h *AuthHandler) DeleteToken(w http.ResponseWriter, r *http.Request) {
	user := auth2.GetUserFromContext(r)
	if user == nil {
		jsonError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	tokenID := chi.URLParam(r, "tokenId")
	if err := h.db.DeleteToken(tokenID, user.ID); err != nil {
		jsonError(w, "Token not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Token deleted successfully",
	})
}

// GetMyPastes returns pastes for the current user
func (h *AuthHandler) GetMyPastes(w http.ResponseWriter, r *http.Request) {
	user := auth2.GetUserFromContext(r)
	if user == nil {
		jsonError(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	page := 1
	limit := 20

	pastes, total, err := h.db.GetPastesByUserID(user.ID, page, limit)
	if err != nil {
		jsonError(w, "Failed to fetch pastes", http.StatusInternalServerError)
		return
	}

	totalPages := (total + limit - 1) / limit

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"pastes": pastes,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      total,
			"totalPages": totalPages,
		},
	})
}

func jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
