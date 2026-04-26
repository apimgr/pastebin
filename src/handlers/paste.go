package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/auth"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/models"
	"github.com/go-chi/chi/v5"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type PasteHandler struct {
	db      database.DB
	baseURL string
}

func NewPasteHandler(db database.DB, baseURL string) *PasteHandler {
	return &PasteHandler{
		db:      db,
		baseURL: baseURL,
	}
}

func generatePasteID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// CreatePaste handles paste creation
func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
	var content, title, language string
	var isPublic = true
	var expiresAt *time.Time

	contentType := r.Header.Get("Content-Type")

	// Handle different content types
	if strings.HasPrefix(contentType, "application/json") {
		var req struct {
			Content   string `json:"content"`
			Title     string `json:"title"`
			Language  string `json:"language"`
			IsPublic  *bool  `json:"is_public"`
			ExpiresIn string `json:"expires_in"` // "1h", "1d", "1w", "never"
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.errorResponse(w, r, "Invalid JSON", http.StatusBadRequest)
			return
		}
		content = req.Content
		title = req.Title
		language = req.Language
		if req.IsPublic != nil {
			isPublic = *req.IsPublic
		}
		if req.ExpiresIn != "" && req.ExpiresIn != "never" {
			exp := parseExpiry(req.ExpiresIn)
			if exp != nil {
				expiresAt = exp
			}
		}
	} else if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle form upload
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			h.errorResponse(w, r, "Failed to parse form", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("files")
		if err == nil {
			defer file.Close()
			buf := make([]byte, header.Size)
			file.Read(buf)
			content = string(buf)
			title = header.Filename
			language = detectLanguage(header.Filename)
		} else {
			content = r.FormValue("content")
			title = r.FormValue("title")
			language = r.FormValue("language")
		}
	} else {
		// Raw text upload (curl --data-binary)
		buf := make([]byte, 10<<20) // 10MB max
		n, _ := r.Body.Read(buf)
		content = string(buf[:n])
		title = r.Header.Get("X-Title")
		language = r.Header.Get("X-Language")
	}

	if strings.TrimSpace(content) == "" {
		h.errorResponse(w, r, "Content is required", http.StatusBadRequest)
		return
	}

	// Generate unique ID
	var pasteID string
	for attempts := 0; attempts < 10; attempts++ {
		pasteID = generatePasteID()
		existing, _ := h.db.GetPasteByID(pasteID)
		if existing == nil {
			break
		}
	}

	// Get user from context (may be nil)
	user := auth.GetUserFromContext(r)
	var userID *string
	if user != nil {
		userID = &user.ID
	}

	paste := &models.Paste{
		ID:        pasteID,
		Title:     title,
		Content:   strings.TrimSpace(content),
		Language:  language,
		IsPublic:  isPublic,
		ExpiresAt: expiresAt,
		UserID:    userID,
		Views:     0,
	}

	if paste.Title == "" {
		paste.Title = "Untitled"
	}
	if paste.Language == "" {
		paste.Language = "text"
	}

	if err := h.db.CreatePaste(paste); err != nil {
		h.errorResponse(w, r, "Failed to create paste", http.StatusInternalServerError)
		return
	}

	pasteURL := h.getPasteURL(r, paste.ID)

	// Return response based on Accept header
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":         paste.ID,
			"title":      paste.Title,
			"language":   paste.Language,
			"is_public":  paste.IsPublic,
			"created_at": paste.CreatedAt,
			"link":       pasteURL,
		})
	} else {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(pasteURL))
	}
}

// GetPaste retrieves a paste by ID
func (h *PasteHandler) GetPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	paste, err := h.db.GetPasteByID(id)
	if err != nil || paste == nil {
		h.errorResponse(w, r, "Paste not found", http.StatusNotFound)
		return
	}

	// Check expiration
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		h.errorResponse(w, r, "Paste has expired", http.StatusGone)
		return
	}

	// Check visibility
	if !paste.IsPublic {
		user := auth.GetUserFromContext(r)
		if user == nil || (paste.UserID != nil && *paste.UserID != user.ID) {
			h.errorResponse(w, r, "This paste is private", http.StatusForbidden)
			return
		}
	}

	// Increment views
	h.db.IncrementPasteViews(id)
	paste.Views++

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"data":    paste,
	})
}

// GetRawPaste returns the raw content
func (h *PasteHandler) GetRawPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	paste, err := h.db.GetPasteByID(id)
	if err != nil || paste == nil {
		http.Error(w, "Paste not found", http.StatusNotFound)
		return
	}

	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Paste has expired", http.StatusGone)
		return
	}

	if !paste.IsPublic {
		user := auth.GetUserFromContext(r)
		if user == nil || (paste.UserID != nil && *paste.UserID != user.ID) {
			http.Error(w, "This paste is private", http.StatusForbidden)
			return
		}
	}

	h.db.IncrementPasteViews(id)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(paste.Content))
}

// ListPastes lists public pastes
func (h *PasteHandler) ListPastes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	pastes, total, err := h.db.GetPublicPastes(page, limit)
	if err != nil {
		h.errorResponse(w, r, "Failed to fetch pastes", http.StatusInternalServerError)
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
			"hasNext":    page < totalPages,
			"hasPrev":    page > 1,
		},
	})
}

// DeletePaste deletes a paste
func (h *PasteHandler) DeletePaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := auth.GetUserFromContext(r)

	if user == nil {
		h.errorResponse(w, r, "Authentication required", http.StatusUnauthorized)
		return
	}

	if err := h.db.DeletePaste(id, user.ID); err != nil {
		h.errorResponse(w, r, "Paste not found or access denied", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Paste deleted successfully",
	})
}

func (h *PasteHandler) getPasteURL(r *http.Request, id string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if h.baseURL != "" {
		return h.baseURL + "/" + id
	}
	return scheme + "://" + host + "/" + id
}

func (h *PasteHandler) errorResponse(w http.ResponseWriter, r *http.Request, message string, status int) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") || strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
	} else {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		w.Write([]byte("Error: " + message))
	}
}

func parseExpiry(expiry string) *time.Time {
	var duration time.Duration
	switch expiry {
	case "1h":
		duration = time.Hour
	case "1d":
		duration = 24 * time.Hour
	case "1w":
		duration = 7 * 24 * time.Hour
	case "1m":
		duration = 30 * 24 * time.Hour
	default:
		return nil
	}
	t := time.Now().Add(duration)
	return &t
}

func detectLanguage(filename string) string {
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx != -1 {
		ext = ext[idx+1:]
	}

	langMap := map[string]string{
		"js":    "javascript",
		"ts":    "typescript",
		"py":    "python",
		"rb":    "ruby",
		"go":    "go",
		"rs":    "rust",
		"java":  "java",
		"c":     "c",
		"cpp":   "cpp",
		"h":     "c",
		"hpp":   "cpp",
		"cs":    "csharp",
		"php":   "php",
		"sh":    "bash",
		"bash":  "bash",
		"zsh":   "bash",
		"html":  "html",
		"css":   "css",
		"json":  "json",
		"yaml":  "yaml",
		"yml":   "yaml",
		"xml":   "xml",
		"sql":   "sql",
		"md":    "markdown",
		"txt":   "text",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}
	return "text"
}
