package handler_test

// Tests for PasteHandler HTTP operations.
// All tests use net/http/httptest and a real SQLite database so that the
// tests exercise the full handler→database path, not just mocks.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/handler"
	"github.com/go-chi/chi/v5"
)

// ─── Test helpers ──────────────────────────────────────────────────────────────

func newTestDB(t *testing.T) database.DB {
	t.Helper()
	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "pastebin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	db, err := database.NewDatabase("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestHandler(t *testing.T) (*handler.PasteHandler, database.DB) {
	t.Helper()
	db := newTestDB(t)
	return handler.NewPasteHandler(db, ""), db
}

// withID injects a chi URL param "id" into the request context so that
// chi.URLParam(r, "id") works without a full router.
func withID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// createViaAPI posts JSON to the handler and returns the parsed response body.
func createViaAPI(t *testing.T, h *handler.PasteHandler, body string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	var m map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&m); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, rr.Body.String())
	}
	return m
}

// ─── CreatePaste ──────────────────────────────────────────────────────────────

// TestCreatePaste_JSON verifies JSON create returns 201 with ok:true and an id.
func TestCreatePaste_JSON(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(`{"content":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d", rr.Code, http.StatusCreated)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok: got %v, want true", resp["ok"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	id, ok := data["id"].(string)
	if !ok || id == "" {
		t.Error("id missing or empty in response data")
	}
}

// TestCreatePaste_EmptyContent expects 400 when content is empty.
func TestCreatePaste_EmptyContent(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"empty string", `{"content":""}`},
		{"missing field", `{}`},
		{"whitespace only", `{"content":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHandler(t)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")

			rr := httptest.NewRecorder()
			h.CreatePaste(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("status: got %d, want 400", rr.Code)
			}
		})
	}
}

// TestCreatePaste_Form verifies form submit redirects to /{id} with 303.
func TestCreatePaste_Form(t *testing.T) {
	h, _ := newTestHandler(t)

	form := url.Values{"content": {"hello from form"}}
	req := httptest.NewRequest(http.MethodPost, "/paste",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Do NOT set Accept: application/json — browser form path.

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/") {
		t.Errorf("Location %q should start with /", loc)
	}
	// Location must be /{id} — 8-char alphanumeric id after the slash.
	if len(loc) < 2 {
		t.Errorf("Location %q too short", loc)
	}
}

// ─── GetPaste ─────────────────────────────────────────────────────────────────

// TestGetPaste creates a paste via API, then retrieves it by ID.
func TestGetPaste(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"test content"}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/paste/"+id, nil)
	req.Header.Set("Accept", "application/json")
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.GetPaste(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200\nbody: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok: got %v", resp["ok"])
	}
}

// TestGetPaste_NotFound expects 404 for an unknown ID.
func TestGetPaste_NotFound(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/paste/badid00x", nil)
	req.Header.Set("Accept", "application/json")
	req = withID(req, "badid00x")

	rr := httptest.NewRecorder()
	h.GetPaste(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rr.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ok"] != false {
		t.Errorf("ok: got %v, want false", resp["ok"])
	}
}

// TestGetRawPaste creates a paste and verifies the raw endpoint returns plain text.
func TestGetRawPaste(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"raw hello"}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/paste/"+id+"/raw", nil)
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.GetRawPaste(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type: got %q, want text/plain", ct)
	}
	body := rr.Body.String()
	if body != "raw hello" {
		t.Errorf("body: got %q, want %q", body, "raw hello")
	}
}

// ─── DeletePaste ──────────────────────────────────────────────────────────────

// TestDeletePaste creates a paste, extracts its delete token, and deletes it.
func TestDeletePaste(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"to be deleted"}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)
	token := data["delete_token"].(string)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/paste/%s?token=%s", id, token), nil)
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.DeletePaste(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200\nbody: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok: got %v, want true", resp["ok"])
	}
}

// TestDeletePaste_WrongToken expects 404 when the token is incorrect.
func TestDeletePaste_WrongToken(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"delete me wrong"}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/paste/%s?token=wrongtoken", id), nil)
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.DeletePaste(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rr.Code)
	}
}

// TestDeletePaste_NoToken expects 400 when no token is supplied.
func TestDeletePaste_NoToken(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"delete me no token"}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/paste/"+id, nil)
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.DeletePaste(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", rr.Code)
	}
}

// ─── ParseExpiry unit tests ────────────────────────────────────────────────────

// TestParseExpiry exercises the ParseExpiry helper directly.
func TestParseExpiry(t *testing.T) {
	cases := []struct {
		input   string
		wantNil bool
		// When not nil, the result must be within this duration of now.
		withinHi time.Duration
	}{
		{"1h", false, 2 * time.Hour},
		{"1d", false, 25 * time.Hour},
		{"1w", false, 8 * 24 * time.Hour},
		{"1m", false, 31 * 24 * time.Hour},
		{"3m", false, 91 * 24 * time.Hour},
		{"6m", false, 181 * 24 * time.Hour},
		{"18m", false, 541 * 24 * time.Hour},
		{"1y", false, 366 * 24 * time.Hour},
		{"2y", false, 731 * 24 * time.Hour},
		{"never", true, 0},
		{"3600", false, 2 * time.Hour},   // raw seconds = 1 hour
		{"bad", true, 0},
		{"0", true, 0},   // non-positive seconds → nil
		{"", true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := handler.ParseExpiry(tc.input)
			if tc.wantNil {
				if got != nil {
					t.Errorf("ParseExpiry(%q): got %v, want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("ParseExpiry(%q): got nil, want non-nil", tc.input)
			}
			now := time.Now()
			if got.Before(now) {
				t.Errorf("ParseExpiry(%q): result %v is in the past", tc.input, got)
			}
			if got.After(now.Add(tc.withinHi)) {
				t.Errorf("ParseExpiry(%q): result %v is too far in the future (> %v)", tc.input, got, tc.withinHi)
			}
		})
	}
}

// ─── DetectLanguage unit tests ────────────────────────────────────────────────

// TestDetectLanguage verifies well-known filename→language mappings and the fallback.
func TestDetectLanguage(t *testing.T) {
	cases := []struct {
		filename string
		want     string
	}{
		{"main.go", "go"},
		{"index.js", "javascript"},
		{"app.ts", "typescript"},
		{"script.py", "python"},
		{"style.css", "css"},
		{"README.md", "markdown"},
		{"data.json", "json"},
		{"config.yml", "yaml"},
		{"config.yaml", "yaml"},
		{"Dockerfile", "dockerfile"}, // no dot → whole lowercase name is the key
		{"unknown.xyz", "text"},
		{"noextension", "text"},
	}
	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			got := handler.DetectLanguage(tc.filename)
			if got != tc.want {
				t.Errorf("DetectLanguage(%q): got %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

// ─── HashToken unit tests ─────────────────────────────────────────────────────

// TestHashToken verifies determinism and that distinct inputs produce distinct hashes.
func TestHashToken(t *testing.T) {
	// Same input → same output (determinism).
	h1 := handler.HashToken("mytoken")
	h2 := handler.HashToken("mytoken")
	if h1 != h2 {
		t.Errorf("HashToken is not deterministic: %q != %q", h1, h2)
	}

	// Different inputs → different hashes.
	h3 := handler.HashToken("other")
	if h1 == h3 {
		t.Error("HashToken: different inputs produced the same hash")
	}

	// Verify against the expected SHA-256 of "mytoken".
	sum := sha256.Sum256([]byte("mytoken"))
	expected := hex.EncodeToString(sum[:])
	if h1 != expected {
		t.Errorf("HashToken: got %q, want %q", h1, expected)
	}

	// Empty string does not panic and produces a stable hash.
	he := handler.HashToken("")
	if he == "" {
		t.Error("HashToken of empty string should not be empty")
	}
	_ = bytes.NewBufferString(he) // silence unused import warning if any
}
