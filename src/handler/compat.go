package handler

// compat.go — route handlers for third-party paste-tool API compatibility so
// existing scripts and CLIs work unmodified by pointing them at this server.
//
// Supported wire-compatible APIs and their reference docs:
//   pastebin.com:        https://pastebin.com/doc_api
//   microbin:            https://github.com/szabodanika/microbin
//   lenpaste fork:       https://github.com/forksmgr/lcomrade-lenpaste
//   stikked:             https://github.com/claudehohl/Stikked
//   hastebin/haste:      https://github.com/toptal/haste-server
//   dpaste:              https://github.com/bartTC/dpaste
//   sprunge/0x0/ix.io:   curl-upload family (POST / with a single field)

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/data"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/model"
	"github.com/go-chi/chi/v5"
)

// maxCompatBody caps raw request bodies accepted by compatibility upload
// handlers (hastebin, 0x0.st) at 10 MiB.
const maxCompatBody = 10 << 20

// CompatHandler handles compatibility routes.
type CompatHandler struct {
	ph      *PasteHandler
	db      database.DB
	version string
}

// NewCompatHandler creates a new CompatHandler.
func NewCompatHandler(ph *PasteHandler, db database.DB, version string) *CompatHandler {
	return &CompatHandler{ph: ph, db: db, version: version}
}

// ─── pastebin.com compatibility ───────────────────────────────────────────────

// PastebinPost handles POST /api/api_post.php (pastebin.com API create)
//
// Accepted form fields (subset):
//
//	api_paste_code       — content (required)
//	api_paste_name       — title (optional)
//	api_paste_format     — language (optional)
//	api_paste_private    — 0=public, 1=unlisted, 2=private (treat 2 as unlisted)
//	api_paste_expire_date — N/A/10M/1H/1D/1W/2W/1M/6M/1Y → expiry
//	api_dev_key          — silently ignored
//	api_user_key         — silently ignored
//
// PastebinPost handles POST /api/api_post.php — dispatches on api_option.
//
//	api_option=paste        — create paste (default when field absent)
//	api_option=list         — list recent public pastes as XML
//	api_option=delete       — delete paste by api_paste_key using api_user_key as token
//	api_option=userdetails  — return stub XML user record
func (c *CompatHandler) PastebinPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request | invalid form", http.StatusBadRequest)
		return
	}

	switch r.FormValue("api_option") {
	case "list":
		c.pastebinList(w, r)
	case "delete":
		c.pastebinDelete(w, r)
	case "userdetails":
		c.pastebinUserDetails(w, r)
	default:
		// "paste" or empty
		c.pastebinCreate(w, r)
	}
}

// pastebinCreate handles api_option=paste.
func (c *CompatHandler) pastebinCreate(w http.ResponseWriter, r *http.Request) {
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

	pasteID, _, err := c.ph.createPasteInternal(title, content, lang, vis, 0, expiresAt)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	link := c.ph.pasteURL(r, pasteID)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, link)
}

// pastebinList handles api_option=list — returns XML paste list.
// Honours api_results_limit (1-1000, default 50).
func (c *CompatHandler) pastebinList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.FormValue("api_results_limit"))
	if limit < 1 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	pastes, _, err := c.db.GetPublicPastes(1, limit)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	fmt.Fprint(w, "<pastes>")
	for _, p := range pastes {
		expireDate := "0"
		if p.ExpiresAt != nil {
			expireDate = strconv.FormatInt(p.ExpiresAt.Unix(), 10)
		}
		fmt.Fprintf(w,
			"<paste><paste_key>%s</paste_key><paste_title>%s</paste_title>"+
				"<paste_date>%d</paste_date><paste_expire_date>%s</paste_expire_date>"+
				"<paste_hits>%d</paste_hits><paste_private>0</paste_private></paste>",
			xmlEscape(p.ID), xmlEscape(p.Title), p.CreatedAt.Unix(), expireDate,
			p.Views,
		)
	}
	fmt.Fprint(w, "</pastes>")
}

// pastebinDelete handles api_option=delete.
// api_paste_key is the paste ID; api_user_key is treated as the delete token
// when non-empty and not "ANONYMOUS".
func (c *CompatHandler) pastebinDelete(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("api_paste_key")
	if id == "" {
		http.Error(w, "Bad API request, you need to be logged in to delete a paste.", http.StatusBadRequest)
		return
	}

	token := r.FormValue("api_user_key")
	if token == "" || token == "ANONYMOUS" {
		http.Error(w, "Bad API request, you are not authorized to delete this paste.", http.StatusForbidden)
		return
	}

	if err := c.db.DeletePasteByToken(id, HashToken(token)); err != nil {
		http.Error(w, "Bad API request, invalid paste ID.", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, "Paste Removed")
}

// pastebinUserDetails handles api_option=userdetails — returns stub XML.
func (c *CompatHandler) pastebinUserDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	fmt.Fprint(w, `<user>`+
		`<user_name>anonymous</user_name>`+
		`<user_email></user_email>`+
		`<user_website></user_website>`+
		`<user_avatar_url></user_avatar_url>`+
		`<user_location></user_location>`+
		`<user_account_type>0</user_account_type>`+
		`<user_private>0</user_private>`+
		`<user_format_short>text</user_format_short>`+
		`<user_expiration>N</user_expiration>`+
		`</user>`,
	)
}

// xmlEscape replaces the five XML special characters.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
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
	default:
		// "N" (never) or unknown
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
//
//	title          — paste title
//	body           — content
//	syntax         — language
//	lifetime       — seconds until expiry (0=never)
//	oneUse         — "true" maps to burn_after=1
//	createTokenHash — ignored (public instance)
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

	pasteID, deleteToken, err := c.ph.createPasteInternal(title, content, lang, model.VisibilityPublic, burn, expiresAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create paste"})
		return
	}

	link := c.ph.pasteURL(r, pasteID)
	writeJSON(w, http.StatusOK, map[string]string{
		"id":          pasteID,
		"url":         link,
		"deleteToken": deleteToken,
	})
}

// LenGet handles GET /api/get?id={id}&openOneUse=true
//
// When a paste has burn_after==1 and openOneUse is NOT set to "true", only
// {"id":"...","oneUse":true} is returned (body withheld), consistent with
// lenpaste behaviour for one-time pastes.
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

	// Withhold body for one-time pastes unless the caller explicitly
	// acknowledges they want to consume it (openOneUse=true).
	if paste.BurnAfter == 1 && r.URL.Query().Get("openOneUse") != "true" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":     paste.ID,
			"oneUse": true,
		})
		return
	}

	c.db.IncrementPasteViews(id)
	paste.Views++

	if paste.BurnAfter > 0 && paste.Views >= paste.BurnAfter {
		c.db.DeletePaste(id)
	}

	paste.DeleteTokenHash = ""
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":         paste.ID,
		"title":      paste.Title,
		"body":       paste.Content,
		"syntax":     paste.Language,
		"oneUse":     paste.BurnAfter == 1,
		"createTime": paste.CreatedAt.Unix(),
		"deleteTime": expiryUnix(paste.ExpiresAt),
		"views":      paste.Views,
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
// going through HTTP request parsing.
// Returns the new paste ID and a raw delete token (for compat protocols that
// need to return it, e.g. lenpaste). The delete token is stored as a SHA-256
// hash in paste.DeleteTokenHash and is NOT part of the api_tokens system.
func (c *PasteHandler) createPasteInternal(
	title, content, lang string,
	vis, burnAfter int,
	expiresAt *time.Time,
) (pasteID, deleteToken string, err error) {
	if title == "" {
		title = "Untitled"
	}
	if lang == "" {
		lang = "text"
	}

	for range 10 {
		id, e := generateID()
		if e != nil {
			return "", "", e
		}
		existing, _ := c.db.GetPasteByID(id)
		if existing == nil {
			pasteID = id
			break
		}
	}
	if pasteID == "" {
		return "", "", fmt.Errorf("could not generate unique paste ID")
	}

	// Generate a compat delete token: 16 random bytes hex-encoded.
	// Store its SHA-256 hash in the paste row; NOT stored in api_tokens.
	rawBytes := make([]byte, 16)
	if _, e := rand.Read(rawBytes); e != nil {
		return "", "", e
	}
	deleteToken = hex.EncodeToString(rawBytes)

	paste := &model.Paste{
		ID:              pasteID,
		Title:           title,
		Content:         content,
		Language:        lang,
		Visibility:      vis,
		ExpiresAt:       expiresAt,
		BurnAfter:       burnAfter,
		Views:           0,
		DeleteTokenHash: HashToken(deleteToken),
	}

	if e := c.db.CreatePaste(paste); e != nil {
		return "", "", e
	}
	return pasteID, deleteToken, nil
}

// LenServerInfo handles GET /api/v1/getServerInfo (lenpaste compat).
func (c *CompatHandler) LenServerInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":        c.version,
		"titleMaxlength": 100,
		"bodyMaxlength":  10 * 1024 * 1024,
		"maxLifeTime":    -1,
		"serverAbout":    "",
		"serverRules":    "",
		"adminName":      "",
		"adminMail":      "",
		"syntaxes":       data.Languages(),
	})
}

// ─── stikked compatibility ──────────────────────────────────────────────────

// StikkedCreate handles POST /api/create (stikked API create).
//
// stikked form fields:
//
//	text     — content (required)
//	title    — paste title (optional)
//	name     — author name (ignored)
//	lang     — language (default "text")
//	expire   — minutes until expiry (0/absent = never)
//	private  — "1" marks the paste unlisted
//	apikey   — ignored (open instance)
//
// Responds with a plain-text view URL, or "Error: <msg>" on failure.
func (c *CompatHandler) StikkedCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err := r.ParseForm(); err != nil {
		fmt.Fprint(w, "Error: invalid form")
		return
	}
	content := r.FormValue("text")
	if strings.TrimSpace(content) == "" {
		fmt.Fprint(w, "Error: No paste data sent.")
		return
	}
	title := r.FormValue("title")
	lang := r.FormValue("lang")
	if lang == "" {
		lang = "text"
	}
	vis := model.VisibilityPublic
	if r.FormValue("private") == "1" {
		vis = model.VisibilityUnlisted
	}
	var expiresAt *time.Time
	if m, err := strconv.Atoi(r.FormValue("expire")); err == nil && m > 0 {
		t := time.Now().Add(time.Duration(m) * time.Minute)
		expiresAt = &t
	}
	pasteID, _, err := c.ph.createPasteInternal(title, content, lang, vis, 0, expiresAt)
	if err != nil {
		fmt.Fprint(w, "Error: could not create paste")
		return
	}
	fmt.Fprint(w, c.origin(r)+"/view/"+pasteID)
}

// StikkedJSON handles GET /api/paste/{id} — stikked JSON metadata + raw body.
func (c *CompatHandler) StikkedJSON(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	paste, err := c.db.GetPasteByID(id)
	if err != nil || paste == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "paste not found"})
		return
	}
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		c.db.DeletePaste(id)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "paste not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pid":     paste.ID,
		"title":   paste.Title,
		"name":    "",
		"created": paste.CreatedAt.Unix(),
		"lang":    paste.Language,
		"raw":     paste.Content,
		"hits":    paste.Views,
	})
}

// ─── hastebin / haste-server compatibility ──────────────────────────────────

// HastebinCreate handles POST /documents (haste-server create).
// The request body is the raw paste content. Responds {"key":"<id>"} on success;
// raw retrieval is served by the native GET /raw/{id} route.
func (c *CompatHandler) HastebinCreate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCompatBody))
	if err != nil || strings.TrimSpace(string(body)) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "no content"})
		return
	}
	pasteID, _, err := c.ph.createPasteInternal("", string(body), "text", model.VisibilityPublic, 0, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "could not create document"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"key": pasteID})
}

// HastebinGet handles GET /documents/{id} (haste-server fetch).
func (c *CompatHandler) HastebinGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	paste, err := c.db.GetPasteByID(id)
	if err != nil || paste == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "document not found"})
		return
	}
	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		c.db.DeletePaste(id)
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "document not found"})
		return
	}
	c.db.IncrementPasteViews(id)
	writeJSON(w, http.StatusOK, map[string]string{"key": paste.ID, "data": paste.Content})
}

// ─── dpaste compatibility ────────────────────────────────────────────────────

// DpasteCreate handles POST /api/ and POST /api/v2/ (dpaste create).
//
// dpaste form fields:
//
//	content  — content (required)
//	lexer    — language (aliases: "syntax", "filename")
//	expires  — days until expiry (0/absent = never)
//	format   — "default" (quoted URL), "url" (bare URL), "json"
//
// The native GET /{id}/raw route serves raw content for dpaste clients.
func (c *CompatHandler) DpasteCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")
	if strings.TrimSpace(content) == "" {
		http.Error(w, "This field is required.", http.StatusBadRequest)
		return
	}
	lang := r.FormValue("lexer")
	if lang == "" {
		lang = r.FormValue("syntax")
	}
	if lang == "" {
		lang = r.FormValue("filename")
	}
	if lang == "" {
		lang = "text"
	}
	var expiresAt *time.Time
	if d, err := strconv.Atoi(r.FormValue("expires")); err == nil && d > 0 {
		t := time.Now().Add(time.Duration(d) * 24 * time.Hour)
		expiresAt = &t
	}
	pasteID, _, err := c.ph.createPasteInternal("", content, lang, model.VisibilityPublic, 0, expiresAt)
	if err != nil {
		http.Error(w, "could not create snippet", http.StatusInternalServerError)
		return
	}
	link := c.origin(r) + "/" + pasteID
	switch r.FormValue("format") {
	case "url":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, link)
	case "json":
		writeJSON(w, http.StatusOK, map[string]string{
			"url":     link,
			"content": content,
			"lexer":   lang,
		})
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%q", link)
	}
}

// ─── curl-upload family (sprunge / 0x0.st / ix.io) ───────────────────────────

// RootUpload handles POST / for the curl-upload family. It inspects the form
// fields to identify the client and replies with a bare raw URL; when no
// recognised field is present it delegates to the native create handler so the
// lenpaste form-POST-to-root behaviour is preserved.
//
//	0x0.st:  multipart file field "file" (delete token returned in X-Token)
//	sprunge: form field "sprunge"
//	ix.io:   form field "f:1"
func (c *CompatHandler) RootUpload(w http.ResponseWriter, r *http.Request) {
	if file, _, err := r.FormFile("file"); err == nil {
		defer file.Close()
		body, _ := io.ReadAll(io.LimitReader(file, maxCompatBody))
		c.curlRespond(w, r, string(body), r.FormValue("expires"), true)
		return
	}
	if v := r.FormValue("sprunge"); strings.TrimSpace(v) != "" {
		c.curlRespond(w, r, v, "", false)
		return
	}
	if v := r.FormValue("f:1"); strings.TrimSpace(v) != "" {
		c.curlRespond(w, r, v, "", false)
		return
	}
	c.ph.CreatePaste(w, r)
}

// curlRespond creates a paste from raw content and writes a bare raw URL plus a
// trailing newline, matching sprunge/0x0/ix.io behaviour. When withToken is set
// (0x0.st) the compat delete token is returned in the X-Token response header.
// expires is an optional 0x0-style hours value; non-positive means never.
func (c *CompatHandler) curlRespond(w http.ResponseWriter, r *http.Request, content, expires string, withToken bool) {
	if strings.TrimSpace(content) == "" {
		http.Error(w, "no content", http.StatusBadRequest)
		return
	}
	var expiresAt *time.Time
	if h, err := strconv.Atoi(expires); err == nil && h > 0 {
		t := time.Now().Add(time.Duration(h) * time.Hour)
		expiresAt = &t
	}
	pasteID, deleteToken, err := c.ph.createPasteInternal("", content, "text", model.VisibilityPublic, 0, expiresAt)
	if err != nil {
		http.Error(w, "could not create paste", http.StatusInternalServerError)
		return
	}
	if withToken {
		w.Header().Set("X-Token", deleteToken)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, c.rawURL(r, pasteID))
}

// TermbinServe handles a single termbin/fiche raw-TCP connection: it reads up
// to maxSize bytes until the client half-closes the connection or the deadline
// elapses, creates a paste, and writes back "{base}/{id}\n". base is the URL
// origin without a trailing slash. The connection is always closed on return.
func (c *CompatHandler) TermbinServe(conn net.Conn, base string, maxSize int64, timeout time.Duration) {
	defer conn.Close()

	if timeout > 0 {
		// SetReadDeadline error is non-fatal; the read below still bounds the work.
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
	}

	// Read one extra byte so an over-limit upload can be detected and rejected.
	data, err := io.ReadAll(io.LimitReader(conn, maxSize+1))
	if err != nil && len(data) == 0 {
		fmt.Fprintln(conn, "Error: read failed")
		return
	}
	if int64(len(data)) > maxSize {
		fmt.Fprintln(conn, "Error: paste too large")
		return
	}

	content := strings.TrimRight(string(data), "\r\n")
	if strings.TrimSpace(content) == "" {
		fmt.Fprintln(conn, "Error: no content")
		return
	}

	pasteID, _, err := c.ph.createPasteInternal("", content, "text", model.VisibilityPublic, 0, nil)
	if err != nil {
		fmt.Fprintln(conn, "Error: could not create paste")
		return
	}

	fmt.Fprintf(conn, "%s/%s\n", strings.TrimRight(base, "/"), pasteID)
}

// origin returns the scheme+host base URL for this request, honouring the
// configured base URL override when set.
func (c *CompatHandler) origin(r *http.Request) string {
	return c.ph.base(r)
}

// rawURL returns the raw-content URL for a paste ID.
func (c *CompatHandler) rawURL(r *http.Request, id string) string {
	return c.origin(r) + "/raw/" + id
}

// writeJSON encodes v as indented JSON and writes it to w.
// SetEscapeHTML(false) prevents < > & from being mangled to < > &.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, `{"ok":false,"error":"SERVER_ERROR","message":"Internal server error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func expiryUnix(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}
