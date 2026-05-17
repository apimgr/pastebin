package handler

// compat.go — route handlers for pastebin.com, microbin, and lenpaste
// compatibility so existing scripts and CLIs work without changes.
//
// pastebin.com API docs: https://pastebin.com/doc_api
// microbin: https://github.com/szabodanika/microbin
// lenpaste: https://github.com/lcomrade/lenpaste (archived; using fork data)

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/data"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/model"
	"github.com/go-chi/chi/v5"
)

// CompatHandler handles compatibility routes.
type CompatHandler struct {
	ph *PasteHandler
	db database.DB
}

// NewCompatHandler creates a new CompatHandler.
func NewCompatHandler(ph *PasteHandler, db database.DB) *CompatHandler {
	return &CompatHandler{ph: ph, db: db}
}

// ─── pastebin.com compatibility ───────────────────────────────────────────────

// PastebinPost handles POST /api/api_post.php (pastebin.com API create)
//
// Accepted form fields (subset):
//   api_paste_code       — content (required)
//   api_paste_name       — title (optional)
//   api_paste_format     — language (optional)
//   api_paste_private    — 0=public, 1=unlisted, 2=private (treat 2 as unlisted)
//   api_paste_expire_date — N/A/10M/1H/1D/1W/2W/1M/6M/1Y → expiry
//   api_dev_key          — silently ignored
//   api_user_key         — silently ignored
func (c *CompatHandler) PastebinPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request | invalid form", http.StatusBadRequest)
		return
	}

	content := r.FormValue("api_paste_code")
	if strings.TrimSpace(content) == "" {
		http.Error(w, "Bad API request, the value you use for 'api_paste_code' is empty.", http.StatusBadRequest)
		return
	}

	title := r.FormValue("api_paste_name")
	lang := r.FormValue("api_paste_format")
	if lang == "" {
		lang = "text"
	}

	vis := model.VisibilityPublic
	if priv := r.FormValue("api_paste_private"); priv == "1" || priv == "2" {
		vis = model.VisibilityUnlisted
	}

	expiresAt := parsePastebinExpiry(r.FormValue("api_paste_expire_date"))

	pasteID, err := c.ph.createPasteInternal(title, content, lang, vis, 0, expiresAt)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	link := c.ph.pasteURL(r, pasteID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, link)
}

// PastebinRaw handles GET /api/api_raw.php?i={id}
func (c *CompatHandler) PastebinRaw(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("i")
	if id == "" {
		http.Error(w, "Bad API request", http.StatusBadRequest)
		return
	}

	paste, err := c.db.GetPasteByID(id)
	if err != nil || paste == nil {
		http.Error(w, "Bad API request, invalid paste ID", http.StatusNotFound)
		return
	}
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		c.db.DeletePaste(id)
		http.Error(w, "Bad API request, invalid paste ID", http.StatusNotFound)
		return
	}

	c.db.IncrementPasteViews(id)
	if paste.BurnAfter > 0 && paste.Views+1 >= paste.BurnAfter {
		c.db.DeletePaste(id)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, paste.Content)
}

// PastebinLogin handles POST /api/api_login.php — always returns "ANONYMOUS"
// because this instance has no user accounts.
func (c *CompatHandler) PastebinLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "ANONYMOUS")
}

// parsePastebinExpiry maps pastebin.com expire_date codes to a time.Time.
func parsePastebinExpiry(code string) *time.Time {
	var d time.Duration
	switch code {
	case "10M":
		d = 10 * time.Minute
	case "1H":
		d = time.Hour
	case "1D":
		d = 24 * time.Hour
	case "1W":
		d = 7 * 24 * time.Hour
	case "2W":
		d = 14 * 24 * time.Hour
	case "1M":
		d = 30 * 24 * time.Hour
	case "6M":
		d = 180 * 24 * time.Hour
	case "1Y":
		d = 365 * 24 * time.Hour
	default: // "N" (never) or unknown
		return nil
	}
	t := time.Now().Add(d)
	return &t
}

// ─── microbin compatibility ───────────────────────────────────────────────────

// MicrobinCreate handles POST /api/v1/pasta — microbin create endpoint.
// microbin sends multipart or JSON with: content, title, visibility, expiry, burn_after.
func (c *CompatHandler) MicrobinCreate(w http.ResponseWriter, r *http.Request) {
	// Delegate to the standard create handler — it already speaks multipart + JSON.
	c.ph.CreatePaste(w, r)
}

// MicrobinGet handles GET /api/v1/pasta/{id}
func (c *CompatHandler) MicrobinGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Reuse the standard JSON get handler path.
	chi.RouteContext(r.Context()).URLParams.Keys[0] = "id"
	chi.RouteContext(r.Context()).URLParams.Values[0] = id
	c.ph.GetPaste(w, r)
}

// MicrobinDelete handles DELETE /api/v1/pasta/{id}?token=xxx
func (c *CompatHandler) MicrobinDelete(w http.ResponseWriter, r *http.Request) {
	c.ph.DeletePaste(w, r)
}

// MicrobinList handles GET /api/v1/pasta
func (c *CompatHandler) MicrobinList(w http.ResponseWriter, r *http.Request) {
	c.ph.ListPastes(w, r)
}

// ─── lenpaste compatibility ───────────────────────────────────────────────────

// LenCreate handles POST /api/new
//
// lenpaste form fields:
//   title          — paste title
//   body           — content
//   syntax         — language
//   lifetime       — seconds until expiry (0=never)
//   oneUse         — "true" maps to burn_after=1
//   createTokenHash — ignored (public instance)
func (c *CompatHandler) LenCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
		return
	}

	content := r.FormValue("body")
	if strings.TrimSpace(content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body is required"})
		return
	}

	title := r.FormValue("title")
	lang := r.FormValue("syntax")
	if lang == "" {
		lang = "text"
	}

	burn := 0
	if r.FormValue("oneUse") == "true" {
		burn = 1
	}

	var expiresAt *time.Time
	if lt, err := strconv.ParseInt(r.FormValue("lifetime"), 10, 64); err == nil && lt > 0 {
		t := time.Now().Add(time.Duration(lt) * time.Second)
		expiresAt = &t
	}

	pasteID, err := c.ph.createPasteInternal(title, content, lang, model.VisibilityPublic, burn, expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create paste"})
		return
	}

	link := c.ph.pasteURL(r, pasteID)
	writeJSON(w, http.StatusOK, map[string]string{
		"id":  pasteID,
		"url": link,
	})
}

// LenGet handles GET /api/get?id={id}
func (c *CompatHandler) LenGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	paste, err := c.db.GetPasteByID(id)
	if err != nil || paste == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "paste not found"})
		return
	}
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		c.db.DeletePaste(id)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "paste has expired"})
		return
	}

	c.db.IncrementPasteViews(id)
	paste.Views++

	if paste.BurnAfter > 0 && paste.Views >= paste.BurnAfter {
		c.db.DeletePaste(id)
	}

	paste.DeleteTokenHash = ""
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       paste.ID,
		"title":    paste.Title,
		"body":     paste.Content,
		"syntax":   paste.Language,
		"oneUse":   paste.BurnAfter == 1,
		"createTime": paste.CreatedAt.Unix(),
		"deleteTime": expiryUnix(paste.ExpiresAt),
		"views":    paste.Views,
	})
}

// LenRemove handles DELETE /api/remove?id={id}&deleteToken={token}
func (c *CompatHandler) LenRemove(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	token := r.URL.Query().Get("deleteToken")

	if id == "" || token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id and deleteToken are required"})
		return
	}

	if err := c.db.DeletePasteByToken(id, HashToken(token)); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "paste not found or invalid token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "paste deleted"})
}

// LenList handles GET /api/list?pageSize=N&page=N
func (c *CompatHandler) LenList(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if limit < 1 || limit > 100 {
		limit = 20
	}

	pastes, total, err := c.db.GetPublicPastes(page, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch"})
		return
	}

	type lenItem struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Syntax     string `json:"syntax"`
		CreateTime int64  `json:"createTime"`
		Views      int    `json:"views"`
	}

	items := make([]lenItem, 0, len(pastes))
	for _, p := range pastes {
		items = append(items, lenItem{
			ID:         p.ID,
			Title:      p.Title,
			Syntax:     p.Language,
			CreateTime: p.CreatedAt.Unix(),
			Views:      p.Views,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pastes": items,
		"count":  total,
	})
}

// ─── Auth stubs ───────────────────────────────────────────────────────────────

// AuthStubRedirect redirects any auth web route (login/register/logout/settings) to /.
func AuthStubRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

// createPasteInternal is used by compatibility handlers to create pastes without
// going through HTTP request parsing. Returns the new paste ID.
func (c *PasteHandler) createPasteInternal(
	title, content, lang string,
	vis, burnAfter int,
	expiresAt *time.Time,
) (string, error) {
	if title == "" {
		title = "Untitled"
	}
	if lang == "" {
		lang = "text"
	}

	var pasteID string
	for range 10 {
		id, err := generateID()
		if err != nil {
			return "", err
		}
		existing, _ := c.db.GetPasteByID(id)
		if existing == nil {
			pasteID = id
			break
		}
	}
	if pasteID == "" {
		return "", fmt.Errorf("could not generate unique paste ID")
	}

	_, tokenHash, err := generateDeleteToken()
	if err != nil {
		return "", err
	}

	paste := &model.Paste{
		ID:              pasteID,
		Title:           title,
		Content:         content,
		Language:        lang,
		Visibility:      vis,
		ExpiresAt:       expiresAt,
		BurnAfter:       burnAfter,
		DeleteTokenHash: tokenHash,
		Views:           0,
	}

	return pasteID, c.db.CreatePaste(paste)
}

// LenServerInfo handles GET /api/v1/getServerInfo (lenpaste compat).
func (c *CompatHandler) LenServerInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":        "1.0.0",
		"titleMaxlength": 100,
		"bodyMaxlength":  10 * 1024 * 1024,
		"maxLifeTime":    -1,
		"serverAbout":    "",
		"serverRules":    "",
		"adminName":      "",
		"adminMail":      "",
		"syntaxes": data.Languages(),
	})
}


func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, `{"ok":false,"error":"SERVER_ERROR","message":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
	w.Write([]byte("\n"))
}

func expiryUnix(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}
