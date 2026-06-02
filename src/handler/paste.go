package handler

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"

	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/model"
	"github.com/go-chi/chi/v5"
)

// charset for paste IDs — URL-safe alphanumeric.
const idCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// PasteHandler handles all paste HTTP operations.
type PasteHandler struct {
	db      database.DB
	baseURL string // optional override, e.g. "https://paste.example.com"
	// operatorTokenHash is SHA-256(server.token), cached at construction time.
	// A constant-time compare against this lets operator tokens bypass the api_tokens
	// lookup and delete any paste unconditionally (PART 11).
	operatorTokenHash [32]byte
}

// NewPasteHandler constructs a PasteHandler.
// operatorTokenHash must be sha256.Sum256([]byte(cfg.Server.Token)); pass a zero
// array when the server token is not set (all operator paths will return 401).
func NewPasteHandler(db database.DB, baseURL string, operatorTokenHash [32]byte) *PasteHandler {
	return &PasteHandler{db: db, baseURL: baseURL, operatorTokenHash: operatorTokenHash}
}

// ─── ID & token generation ────────────────────────────────────────────────────

// generateID returns an 8-character random alphanumeric string using crypto/rand.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i, v := range b {
		b[i] = idCharset[int(v)%len(idCharset)]
	}
	return string(b), nil
}

// tokenCharset is the base62 alphabet for owner tokens (PART 11).
const tokenCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateOwnerToken generates a spec-compliant resource-owner token.
// Format: "tok_" prefix + 32 random base62 chars.
// Returns the raw plaintext token and its SHA-256 [32]byte hash.
func generateOwnerToken() (plaintext string, tokenHash [32]byte, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", tokenHash, err
	}
	b := make([]byte, 32)
	for i, v := range raw {
		b[i] = tokenCharset[int(v)%len(tokenCharset)]
	}
	plaintext = "tok_" + string(b)
	tokenHash = sha256.Sum256([]byte(plaintext))
	return plaintext, tokenHash, nil
}

// HashToken returns the SHA-256 hex digest of a token string.
// Exported so the server layer can call it for web form submissions.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ─── Create ───────────────────────────────────────────────────────────────────

// CreateRequest is the JSON body for paste creation.
type CreateRequest struct {
	Content    string `json:"content"`
	Title      string `json:"title"`
	Language   string `json:"language"`
	Visibility string `json:"visibility"` // "public" | "unlisted"
	ExpiresIn  string `json:"expires_in"` // "1h","1d","1w","1m","3m","6m","1y","18m","2y","never", or seconds
	BurnAfter  int    `json:"burn_after"` // 0=disabled, 1-9999
}

// CreatePaste handles paste creation via JSON, multipart, or raw body.
func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest

	ct := r.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(ct, "application/json"):
		dec := json.NewDecoder(io.LimitReader(r.Body, 10<<20))
		if err := dec.Decode(&req); err != nil {
			h.errJSON(w, "invalid JSON", http.StatusBadRequest)
			return
		}

	case strings.HasPrefix(ct, "multipart/form-data"):
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			h.errJSON(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("files")
		if err == nil {
			defer file.Close()
			raw, _ := io.ReadAll(io.LimitReader(file, 10<<20))
			req.Content = string(raw)
			req.Title = header.Filename
			req.Language = DetectLanguage(header.Filename)
		} else {
			req.Content = r.FormValue("content")
			req.Title = r.FormValue("title")
			req.Language = r.FormValue("language")
		}
		req.Visibility = r.FormValue("visibility")
		req.ExpiresIn = r.FormValue("expires_in")
		if ba, err := strconv.Atoi(r.FormValue("burn_after")); err == nil {
			req.BurnAfter = ba
		}

	case strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		if err := r.ParseForm(); err != nil {
			h.errJSON(w, "failed to parse form", http.StatusBadRequest)
			return
		}
		req.Content = r.FormValue("content")
		req.Title = r.FormValue("title")
		req.Language = r.FormValue("language")
		req.Visibility = r.FormValue("visibility")
		req.ExpiresIn = r.FormValue("expires_in")
		if ba, err := strconv.Atoi(r.FormValue("burn_after")); err == nil {
			req.BurnAfter = ba
		}

	default:
		// Raw body (curl --data-binary)
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		req.Content = string(raw)
		req.Title = r.Header.Get("X-Title")
		req.Language = r.Header.Get("X-Language")
		req.ExpiresIn = r.Header.Get("X-Expires-In")
	}

	req.Content = strings.TrimRight(req.Content, "\n")
	if strings.TrimSpace(req.Content) == "" {
		h.errJSON(w, "content is required", http.StatusBadRequest)
		return
	}

	// Visibility
	vis := model.VisibilityPublic
	if req.Visibility == "unlisted" || req.Visibility == "1" {
		vis = model.VisibilityUnlisted
	}

	// BurnAfter clamp
	burn := req.BurnAfter
	if burn < 0 {
		burn = 0
	}
	if burn > 9999 {
		burn = 9999
	}

	// Expiry
	var expiresAt *time.Time
	if req.ExpiresIn != "" && req.ExpiresIn != "never" {
		if t := ParseExpiry(req.ExpiresIn); t != nil {
			expiresAt = t
		}
	}

	// Language default
	if req.Language == "" {
		req.Language = "text"
	}
	if req.Title == "" {
		req.Title = "Untitled"
	}

	// Generate unique paste ID
	var pasteID string
	for range 10 {
		id, err := generateID()
		if err != nil {
			h.errJSON(w, "failed to generate ID", http.StatusInternalServerError)
			return
		}
		existing, _ := h.db.GetPasteByID(id)
		if existing == nil {
			pasteID = id
			break
		}
	}
	if pasteID == "" {
		h.errJSON(w, "could not generate unique ID", http.StatusInternalServerError)
		return
	}

	// Generate owner token (PART 11): tok_ + 32 base62 chars, store SHA-256 in api_tokens.
	plainToken, tokenHash, err := generateOwnerToken()
	if err != nil {
		h.errJSON(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	paste := &model.Paste{
		ID:         pasteID,
		Title:      req.Title,
		Content:    req.Content,
		Language:   req.Language,
		Visibility: vis,
		ExpiresAt:  expiresAt,
		BurnAfter:  burn,
		Views:      0,
	}

	if err := h.db.CreatePaste(paste); err != nil {
		h.errJSON(w, "failed to create paste", http.StatusInternalServerError)
		return
	}

	// Store the token in api_tokens. token_prefix = first 12 chars of raw token.
	tokenHashHex := hex.EncodeToString(tokenHash[:])
	tokenPrefix := plainToken
	if len(tokenPrefix) > 12 {
		tokenPrefix = tokenPrefix[:12]
	}
	if err := h.db.CreateAPIToken(tokenHashHex, tokenPrefix, "paste", pasteID, expiresAt); err != nil {
		// Non-fatal: paste is already created; log and continue.
		// The owner token won't work for deletion, but the paste itself is intact.
		fmt.Printf("warning: create api_token for paste %s: %v\n", pasteID, err)
	}

	link := h.pasteURL(r, paste.ID)
	resp := model.CreateResponse{
		ID:         paste.ID,
		Title:      paste.Title,
		Language:   paste.Language,
		Visibility: paste.Visibility,
		BurnAfter:  paste.BurnAfter,
		ExpiresAt:  paste.ExpiresAt,
		Views:      0,
		CreatedAt:  paste.CreatedAt,
		Link:       link,
		OwnerToken: plainToken,
	}

	accept := r.Header.Get("Accept")
	isAPI := strings.HasPrefix(r.URL.Path, "/api/")
	isJSON := strings.Contains(accept, "application/json")

	// Browser form submit (no JS): redirect to the paste view.
	if !isAPI && !isJSON && strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		http.Redirect(w, r, "/"+paste.ID, http.StatusSeeOther)
		return
	}

	// curl / raw / non-JSON API callers: return the URL as plain text.
	if !isAPI && !isJSON {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, link)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{"ok": true, "data": resp})
}

// ─── Get ──────────────────────────────────────────────────────────────────────

// GetPaste returns paste JSON (burns if applicable).
func (h *PasteHandler) GetPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	paste, err := h.loadLivePaste(w, id)
	if paste == nil || err != nil {
		return
	}

	h.db.IncrementPasteViews(id)
	paste.Views++

	// After incrementing, check burn limit.
	if paste.BurnAfter > 0 && paste.Views >= paste.BurnAfter {
		h.db.DeletePaste(id)
	}

	// Never return delete token hash.
	paste.DeleteTokenHash = ""

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": paste})
}

// GetRawPaste returns paste content as plain text.
func (h *PasteHandler) GetRawPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	paste, err := h.loadLivePaste(w, id)
	if paste == nil || err != nil {
		return
	}

	h.db.IncrementPasteViews(id)

	if paste.BurnAfter > 0 && paste.Views+1 >= paste.BurnAfter {
		h.db.DeletePaste(id)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, paste.Content)
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListPastes returns paginated public pastes as JSON.
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
		h.errJSON(w, "failed to fetch pastes", http.StatusInternalServerError)
		return
	}

	totalPages := (total + limit - 1) / limit
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"pastes": pastes,
			"pagination": map[string]interface{}{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": totalPages,
				"has_next":    page < totalPages,
				"has_prev":    page > 1,
			},
		},
	})
}

// ─── Delete ───────────────────────────────────────────────────────────────────

// DeletePaste deletes a paste using two-tier auth (PART 11):
//  1. Authorization: Bearer <token> — primary delivery
//  2. If the token matches server.token (operator) → delete unconditionally
//  3. Otherwise → verify token against api_tokens for this paste
//
// Legacy fallbacks accepted for compatibility:
//   - ?token=tok_... query param
//   - X-Delete-Token: tok_... header
//   - JSON body {"token":"tok_..."}
func (h *PasteHandler) DeletePaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Extract token — primary path is Authorization: Bearer.
	token := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = auth[len("Bearer "):]
	}
	// Legacy fallbacks.
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		token = r.Header.Get("X-Delete-Token")
	}
	if token == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Token string `json:"token"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body)
		token = body.Token
	}

	if token == "" {
		h.errJSON(w, "owner token required (Authorization: Bearer tok_...)", http.StatusUnauthorized)
		return
	}

	incomingHash := sha256.Sum256([]byte(token))

	// Tier 1: operator token — allows deleting any paste.
	var zeroHash [32]byte
	if h.operatorTokenHash != zeroHash &&
		subtle.ConstantTimeCompare(incomingHash[:], h.operatorTokenHash[:]) == 1 {
		if err := h.db.DeletePaste(id); err != nil {
			h.errJSON(w, "paste not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"message": "paste deleted"}})
		return
	}

	// Tier 2: resource-owner token — must match api_tokens for this paste.
	if err := h.db.VerifyAPIToken(incomingHash, "paste", id); err != nil {
		h.errJSON(w, "paste not found or invalid token", http.StatusNotFound)
		return
	}
	if err := h.db.DeletePaste(id); err != nil {
		h.errJSON(w, "paste not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"message": "paste deleted"}})
}

// ─── Web view helpers ─────────────────────────────────────────────────────────

// GetPasteForWeb returns the paste struct for server-side template rendering.
// Increments views and handles burn logic. Returns nil if paste is unavailable.
func (h *PasteHandler) GetPasteForWeb(id string) (*model.Paste, error) {
	paste, err := h.db.GetPasteByID(id)
	if err != nil {
		return nil, err
	}
	if paste == nil {
		return nil, nil
	}
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		h.db.DeletePaste(id)
		return nil, nil
	}

	h.db.IncrementPasteViews(id)
	paste.Views++

	if paste.BurnAfter > 0 && paste.Views >= paste.BurnAfter {
		h.db.DeletePaste(id)
	}

	paste.DeleteTokenHash = ""
	return paste, nil
}

// HighlightedContent returns Chroma-highlighted HTML for the paste content.
// Falls back to HTML-escaped plain text if the language is unknown or highlighting fails.
func HighlightedContent(paste *model.Paste) template.HTML {
	lexer := lexers.Get(paste.Language)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	style := styles.Get("github-dark")
	if style == nil {
		style = styles.Fallback
	}

	formatter := chromahtml.New(
		chromahtml.TabWidth(4),
		chromahtml.WithLineNumbers(false),
	)

	iterator, err := lexer.Tokenise(nil, paste.Content)
	if err != nil {
		return template.HTML(template.HTMLEscapeString(paste.Content))
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return template.HTML(template.HTMLEscapeString(paste.Content))
	}

	return template.HTML(buf.String())
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// loadLivePaste retrieves a paste by ID, enforces expiry, and writes an error
// response if unavailable. Returns nil when a response has already been written.
func (h *PasteHandler) loadLivePaste(w http.ResponseWriter, id string) (*model.Paste, error) {
	paste, err := h.db.GetPasteByID(id)
	if err != nil {
		h.errJSON(w, "internal server error", http.StatusInternalServerError)
		return nil, err
	}
	if paste == nil {
		h.errJSON(w, "paste not found", http.StatusNotFound)
		return nil, nil
	}
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		h.db.DeletePaste(id)
		h.errJSON(w, "paste has expired", http.StatusGone)
		return nil, nil
	}
	return paste, nil
}

func (h *PasteHandler) pasteURL(r *http.Request, id string) string {
	if h.baseURL != "" {
		return h.baseURL + "/" + id
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/" + id
}

func (h *PasteHandler) errJSON(w http.ResponseWriter, msg string, status int) {
	writeJSON(w, status, map[string]interface{}{
		"ok":      false,
		"error":   httpErrCode(status),
		"message": msg,
	})
}

// httpErrCode maps HTTP status to a canonical error code string (PART 9).
func httpErrCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusMethodNotAllowed:
		return "METHOD_NOT_ALLOWED"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusServiceUnavailable:
		return "MAINTENANCE"
	default:
		return "SERVER_ERROR"
	}
}

// mapAPIErrorCodeToHTTPStatus maps a canonical error code to its HTTP status (PART 9).
func mapAPIErrorCodeToHTTPStatus(code string) int {
	switch code {
	case "BAD_REQUEST", "VALIDATION_FAILED":
		return http.StatusBadRequest
	case "UNAUTHORIZED", "TOKEN_EXPIRED", "TOKEN_INVALID":
		return http.StatusUnauthorized
	case "FORBIDDEN", "ACCOUNT_LOCKED":
		return http.StatusForbidden
	case "NOT_FOUND":
		return http.StatusNotFound
	case "METHOD_NOT_ALLOWED":
		return http.StatusMethodNotAllowed
	case "CONFLICT":
		return http.StatusConflict
	case "RATE_LIMITED":
		return http.StatusTooManyRequests
	case "MAINTENANCE":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// sendAPIError writes a canonical error response using the error code (PART 9).
func sendAPIError(w http.ResponseWriter, code, message string) {
	status := mapAPIErrorCodeToHTTPStatus(code)
	writeJSON(w, status, map[string]interface{}{
		"ok":      false,
		"error":   code,
		"message": message,
	})
}

// ─── Expiry parsing ───────────────────────────────────────────────────────────

// ParseExpiry converts an expiry string to an absolute time.
// Accepts: 1h 1d 1w 1m 3m 6m 1y 18m 2y or raw seconds as a decimal string.
func ParseExpiry(s string) *time.Time {
	var d time.Duration
	switch s {
	case "1h":
		d = time.Hour
	case "1d":
		d = 24 * time.Hour
	case "1w":
		d = 7 * 24 * time.Hour
	case "1m":
		d = 30 * 24 * time.Hour
	case "3m":
		d = 90 * 24 * time.Hour
	case "6m":
		d = 180 * 24 * time.Hour
	case "18m":
		d = 540 * 24 * time.Hour
	case "1y":
		d = 365 * 24 * time.Hour
	case "2y":
		d = 730 * 24 * time.Hour
	default:
		// Try raw seconds.
		if sec, err := strconv.ParseInt(s, 10, 64); err == nil && sec > 0 {
			d = time.Duration(sec) * time.Second
		} else {
			return nil
		}
	}
	t := time.Now().Add(d)
	return &t
}

// ─── Language detection ───────────────────────────────────────────────────────

// DetectLanguage infers a syntax-highlighting language name from a filename extension.
func DetectLanguage(filename string) string {
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx != -1 {
		ext = ext[idx+1:]
	}

	m := map[string]string{
		"js":   "javascript",
		"ts":   "typescript",
		"jsx":  "jsx",
		"tsx":  "tsx",
		"py":   "python",
		"rb":   "ruby",
		"go":   "go",
		"rs":   "rust",
		"java": "java",
		"c":    "c",
		"cpp":  "cpp",
		"cc":   "cpp",
		"h":    "c",
		"hpp":  "cpp",
		"cs":   "csharp",
		"php":  "php",
		"sh":   "bash",
		"bash": "bash",
		"zsh":  "bash",
		"fish": "bash",
		"ps1":  "powershell",
		"html": "html",
		"htm":  "html",
		"css":  "css",
		"scss": "scss",
		"sass": "sass",
		"json": "json",
		"yaml": "yaml",
		"yml":  "yaml",
		"toml": "toml",
		"xml":  "xml",
		"sql":  "sql",
		"md":   "markdown",
		"txt":  "text",
		"lua":  "lua",
		"r":    "r",
		"swift": "swift",
		"kt":   "kotlin",
		"dart": "dart",
		"ex":   "elixir",
		"exs":  "elixir",
		"erl":  "erlang",
		"hs":   "haskell",
		"clj":  "clojure",
		"scala": "scala",
		"pl":   "perl",
		"ini":  "ini",
		"conf": "ini",
		"env":  "bash",
		"diff": "diff",
		"patch": "diff",
		"dockerfile": "dockerfile",
		"makefile": "makefile",
		"mk":   "makefile",
	}

	if lang, ok := m[ext]; ok {
		return lang
	}
	return "text"
}
