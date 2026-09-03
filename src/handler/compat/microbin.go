package compat

import "net/http"

// ─── microbin compatibility ───────────────────────────────────────────────────

// MicrobinCreate handles POST /api/v1/pasta — microbin create endpoint.
// microbin sends multipart or JSON with: content, title, visibility, expiry, burn_after.
func (c *Handler) MicrobinCreate(w http.ResponseWriter, r *http.Request) {
	// Delegate to the standard create handler — it already speaks multipart + JSON.
	c.ph.CreatePaste(w, r)
}

// MicrobinGet handles GET /api/v1/pasta/{id}
func (c *Handler) MicrobinGet(w http.ResponseWriter, r *http.Request) {
	// The route registers {id}, so GetPaste reads the same param directly.
	c.ph.GetPaste(w, r)
}

// MicrobinDelete handles DELETE /api/v1/pasta/{id}?token=xxx
func (c *Handler) MicrobinDelete(w http.ResponseWriter, r *http.Request) {
	c.ph.DeletePaste(w, r)
}

// MicrobinList handles GET /api/v1/pasta
func (c *Handler) MicrobinList(w http.ResponseWriter, r *http.Request) {
	c.ph.ListPastes(w, r)
}
