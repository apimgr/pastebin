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

	"github.com/apimgr/pastebin/src/common/httputil"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/metrics"
	"github.com/apimgr/pastebin/src/model"
	"github.com/go-chi/chi/v5"
)

// charset for paste IDs — URL-safe alphanumeric.
const idCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// PasteHandler handles all paste HTTP operations.
type PasteHandler struct {
	db database.DB
	// baseURL is an optional override, e.g. "https://paste.example.com".
	baseURL string
	// baseURLFn resolves the canonical per-request base URL (PART 12: full
	// {proto}/{fqdn}/{port}/{baseurl} chain with trusted-proxy gating and the
	// configured-DOMAIN fallback). Injected by the server layer so the resolver
	// is shared instead of duplicated; nil in unit tests, where base() falls
	// back to the static baseURL / request Host.
	baseURLFn func(*http.Request) string
	// operatorTokenHash is SHA-256(server.token), cached at construction time.
	// A constant-time compare against this lets operator tokens bypass the api_tokens
	// lookup and delete any paste unconditionally (PART 11).
	operatorTokenHash [32]byte
}

// NewPasteHandler constructs a PasteHandler.
// operatorTokenHash must be sha256.Sum256([]byte(cfg.Server.Token)); pass a zero
// array when the server token is not set (all operator paths will return 401).
func NewPasteHandler(db database.DB, baseURL string, operatorTokenHash [32]byte) *PasteHandler {
	h := &PasteHandler{db: db, baseURL: baseURL, operatorTokenHash: operatorTokenHash}
	h.refreshActiveTokenGauge()
	return h
}

// SetBaseURLResolver injects the server's canonical per-request base-URL
// resolver (PART 12) so pasteURL/origin reuse the full reverse-proxy + FQDN
// chain instead of a simplified copy.
func (h *PasteHandler) SetBaseURLResolver(fn func(*http.Request) string) {
	h.baseURLFn = fn
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

// extractToken pulls a bearer/owner token from the request using all accepted
// delivery mechanisms (in priority order):
//  1. Authorization: Bearer tok_...
//  2. Authorization: tok_...   (bare, no scheme prefix)
//  3. X-Api-Token: tok_...
//  4. X-Token: tok_...
//  5. X-Delete-Token: tok_...  (legacy compat header)
//  6. ?token= query param
//  7. JSON body {"token":"tok_..."}
func extractToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return auth[len("Bearer "):]
		}
		// Bare token — no scheme prefix.
		return auth
	}
	for _, h := range []string{"X-Api-Token", "X-Token", "X-Delete-Token"} {
		if v := r.Header.Get(h); v != "" {
			return v
		}
	}
	if v := r.URL.Query().Get("token"); v != "" {
		return v
	}
	// Web form (no-JS create) supplies the token as a urlencoded field.
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		if v := r.PostFormValue("owner_token"); v != "" {
			return v
		}
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Token string `json:"token"`
		}
		// Peek without consuming — clone the body via a bytes buffer.
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<10))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		json.Unmarshal(raw, &body)
		return body.Token
	}
	return ""
}

// ─── Create ───────────────────────────────────────────────────────────────────

// CreateRequest is the JSON body for paste creation.
type CreateRequest struct {
	Content  string `json:"content"`
	Title    string `json:"title"`
	Language string `json:"language"`
	// Visibility is "public" | "unlisted".
	Visibility string `json:"visibility"`
	// ExpiresIn is "1h","1d","1w","1m","3m","6m","1y","18m","2y","never", or seconds.
	ExpiresIn string `json:"expires_in"`
	// BurnAfter is 0=disabled, 1-9999.
	BurnAfter int `json:"burn_after"`
}

// createFromRequest parses the request body (JSON, multipart, urlencoded, or
// raw), creates the paste, and returns the result. It writes NO HTTP response;
// callers render the outcome. On failure it returns an HTTP status and error.
func (h *PasteHandler) createFromRequest(r *http.Request) (*model.CreateResponse, int, error) {
	var req CreateRequest

	ct := r.Header.Get("Content-Type")

	switch {
	case strings.HasPrefix(ct, "application/json"):
		dec := json.NewDecoder(io.LimitReader(r.Body, 10<<20))
		if err := dec.Decode(&req); err != nil {
			return nil, http.StatusBadRequest, fmt.Errorf("invalid JSON")
		}

	case strings.HasPrefix(ct, "multipart/form-data"):
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			return nil, http.StatusBadRequest, fmt.Errorf("failed to parse form")
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
			return nil, http.StatusBadRequest, fmt.Errorf("failed to parse form")
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
		return nil, http.StatusBadRequest, fmt.Errorf("content is required")
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
			return nil, http.StatusInternalServerError, fmt.Errorf("failed to generate ID")
		}
		existing, _ := h.db.GetPasteByID(id)
		if existing == nil {
			pasteID = id
			break
		}
	}
	if pasteID == "" {
		return nil, http.StatusInternalServerError, fmt.Errorf("could not generate unique ID")
	}

	// Resolve owner token: reuse an existing valid token if the caller provides one,
	// otherwise generate a fresh tok_+32base62 token (PART 11).
	// An invalid/unknown provided token is non-fatal — a new token is generated instead,
	// so web-UI users who paste a stale token from CLI still get a working paste.
	var plainToken string
	var tokenHash [32]byte
	if incoming := extractToken(r); incoming != "" {
		inHash := sha256.Sum256([]byte(incoming))
		if err := h.db.ValidateAPIToken(inHash, "paste"); err == nil {
			plainToken = incoming
			tokenHash = inHash
		}
	}
	if plainToken == "" {
		var err error
		plainToken, tokenHash, err = generateOwnerToken()
		if err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("failed to generate token")
		}
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
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create paste")
	}
	metrics.PastesCreatedTotal.Inc()

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
	} else {
		h.refreshActiveTokenGauge()
	}

	link := h.pasteURL(r, paste.ID)
	resp := &model.CreateResponse{
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
	return resp, 0, nil
}

// CreateFromForm creates a paste from an HTML form POST and returns the result
// for server-side template rendering — the no-JS web create flow (PART 16).
func (h *PasteHandler) CreateFromForm(r *http.Request) (*model.CreateResponse, int, error) {
	return h.createFromRequest(r)
}

// CreatePaste handles paste creation for API and CLI callers (JSON, multipart,
// raw, or urlencoded) and writes the response per content negotiation.
func (h *PasteHandler) CreatePaste(w http.ResponseWriter, r *http.Request) {
	resp, status, err := h.createFromRequest(r)
	if err != nil {
		h.errJSON(w, err.Error(), status)
		return
	}

	ct := r.Header.Get("Content-Type")
	accept := r.Header.Get("Accept")
	isAPI := strings.HasPrefix(r.URL.Path, "/api/")
	isJSON := strings.Contains(accept, "application/json")

	// Browser form submit without JS that reaches the API handler directly:
	// redirect to the paste view (the /create web route renders a confirmation).
	if !isAPI && !isJSON && strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		http.Redirect(w, r, "/"+resp.ID, http.StatusSeeOther)
		return
	}

	// curl / raw / non-JSON API callers: return the URL as plain text.
	if !isAPI && !isJSON {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, resp.Link)
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
	metrics.PastesViewedTotal.Inc()

	// After incrementing, check burn limit.
	if paste.BurnAfter > 0 && paste.Views >= paste.BurnAfter {
		h.db.DeletePaste(id)
		metrics.PastesDeletedTotal.Inc()
	}

	// Never return delete token hash.
	paste.DeleteTokenHash = ""

	// Content negotiation: text format returns key=value summary (PART 14).
	if httputil.GetAPIResponseFormat(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "id: %s\ntitle: %s\nlanguage: %s\nviews: %d\ncreated: %s\n",
			paste.ID, paste.Title, paste.Language, paste.Views, paste.CreatedAt.Format(time.RFC3339))
		if paste.ExpiresAt != nil {
			fmt.Fprintf(w, "expires: %s\n", paste.ExpiresAt.Format(time.RFC3339))
		}
		fmt.Fprintf(w, "\n%s\n", paste.Content)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": paste})
}

// activeContentTypes are MIME types that a browser can execute or interpret as
// markup when served inline. Per PART 11 security rules, these MUST be forced
// to Content-Disposition: attachment so browsers treat them as downloads.
var activeContentTypes = map[string]bool{
	"text/html":                true,
	"application/xhtml+xml":    true,
	"image/svg+xml":            true,
	"text/xml":                 true,
	"application/xml":          true,
	"application/javascript":   true,
	"text/javascript":          true,
}

// GetRawPaste returns paste content with a MIME type detected from the content
// bytes (PART 11: exact Content-Type + nosniff; active types forced to attachment).
func (h *PasteHandler) GetRawPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	paste, err := h.loadLivePaste(w, id)
	if paste == nil || err != nil {
		return
	}

	h.db.IncrementPasteViews(id)
	metrics.PastesViewedTotal.Inc()

	if paste.BurnAfter > 0 && paste.Views+1 >= paste.BurnAfter {
		h.db.DeletePaste(id)
		metrics.PastesDeletedTotal.Inc()
	}

	body := []byte(paste.Content)

	// Sniff the content type from the first 512 bytes.
	// DetectContentType always returns a valid MIME type.
	detected := http.DetectContentType(body[:min512(len(body))])

	// Strip parameters for the allow-list check (e.g. "text/plain; charset=utf-8" → "text/plain").
	baseType := detected
	if idx := strings.IndexByte(detected, ';'); idx >= 0 {
		baseType = strings.TrimSpace(detected[:idx])
	}

	// Active content types must be served as downloads to prevent browser execution.
	if activeContentTypes[baseType] {
		w.Header().Set("Content-Disposition", "attachment")
	}

	w.Header().Set("Content-Type", detected)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(body)
}

// min512 returns n capped at 512 for the DetectContentType sniff buffer.
func min512(n int) int {
	if n > 512 {
		return 512
	}
	return n
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

	// Content negotiation: text format returns a tab-separated list (PART 14).
	if httputil.GetAPIResponseFormat(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "# pastes: %d (page %d)\n", total, page)
		for _, p := range pastes {
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Language, p.Title)
		}
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

	token := extractToken(r)
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
		metrics.PastesDeletedTotal.Inc()
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
	metrics.PastesDeletedTotal.Inc()
	h.refreshActiveTokenGauge()

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
	metrics.PastesViewedTotal.Inc()

	if paste.BurnAfter > 0 && paste.Views >= paste.BurnAfter {
		h.db.DeletePaste(id)
		metrics.PastesDeletedTotal.Inc()
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

// refreshActiveTokenGauge recomputes the active API-token gauge from the
// database. ListAPITokens returns only non-revoked rows (PART 20).
func (h *PasteHandler) refreshActiveTokenGauge() {
	toks, err := h.db.ListAPITokens()
	if err != nil {
		return
	}
	metrics.APITokensActive.Set(float64(len(toks)))
}

func (h *PasteHandler) pasteURL(r *http.Request, id string) string {
	return h.base(r) + "/" + id
}

// base resolves the per-request absolute base URL (no trailing slash). It
// delegates to the injected server resolver (PART 12 full chain) when present;
// otherwise it honours the static baseURL override, and finally falls back to
// the request scheme+Host. The trailing-slash trim keeps callers that append
// "/{id}" from emitting a double slash.
func (h *PasteHandler) base(r *http.Request) string {
	if h.baseURLFn != nil {
		return strings.TrimRight(h.baseURLFn(r), "/")
	}
	if h.baseURL != "" {
		return strings.TrimRight(h.baseURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
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
