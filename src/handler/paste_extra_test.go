package handler_test

// Additional coverage tests: ListPastes, GetPasteForWeb, HighlightedContent,
// extra CreatePaste paths (multipart, raw body, burn_after, unlisted, expiry).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/apimgr/pastebin/src/handler"
	"github.com/apimgr/pastebin/src/model"
)

// ─── ListPastes ───────────────────────────────────────────────────────────────

func TestListPastes_Empty(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pastes", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.ListPastes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["ok"] != true {
		t.Errorf("ok: got %v", resp["ok"])
	}
}

func TestListPastes_WithPastes(t *testing.T) {
	h, _ := newTestHandler(t)

	for i := range 3 {
		body := fmt.Sprintf(`{"content":"paste number %d"}`, i)
		createViaAPI(t, h, body)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pastes?page=1&limit=10", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.ListPastes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200\nbody: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("data field missing")
	}
	pagination, ok := data["pagination"].(map[string]interface{})
	if !ok {
		t.Fatal("pagination field missing")
	}
	total := int(pagination["total"].(float64))
	if total < 1 {
		t.Errorf("total: got %d, want >= 1", total)
	}
}

func TestListPastes_DefaultPagination(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pastes?page=0&limit=0", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.ListPastes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
}

func TestListPastes_LimitExceedsMax(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/pastes?limit=999", nil)
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.ListPastes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
}

// ─── GetPasteForWeb ───────────────────────────────────────────────────────────

func TestGetPasteForWeb_NotFound(t *testing.T) {
	h, _ := newTestHandler(t)

	p, err := h.GetPasteForWeb("notexist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != nil {
		t.Errorf("expected nil paste for not found, got %+v", p)
	}
}

func TestGetPasteForWeb_Found(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"web view content"}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	p, err := h.GetPasteForWeb(id)
	if err != nil {
		t.Fatalf("GetPasteForWeb: %v", err)
	}
	if p == nil {
		t.Fatal("expected paste, got nil")
	}
	if p.Content != "web view content" {
		t.Errorf("content: got %q, want %q", p.Content, "web view content")
	}
	if p.DeleteTokenHash != "" {
		t.Error("DeleteTokenHash should be cleared")
	}
}

func TestGetPasteForWeb_BurnAfter(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"burn me","burn_after":1}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	p1, err := h.GetPasteForWeb(id)
	if err != nil {
		t.Fatalf("first GetPasteForWeb: %v", err)
	}
	if p1 == nil {
		t.Fatal("first call: expected paste, got nil")
	}

	p2, err := h.GetPasteForWeb(id)
	if err != nil {
		t.Fatalf("second GetPasteForWeb: %v", err)
	}
	if p2 != nil {
		t.Errorf("second call: expected paste deleted after burn_after=1, got %+v", p2)
	}
}

// ─── HighlightedContent ───────────────────────────────────────────────────────

func TestHighlightedContent(t *testing.T) {
	cases := []struct {
		name    string
		lang    string
		content string
	}{
		{"go_code", "go", "package main"},
		{"plain_text", "text", "hello world"},
		{"unknown_lang", "unknownlang99", "some content"},
		{"empty_content", "go", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			paste := &model.Paste{
				Language: tc.lang,
				Content:  tc.content,
			}
			result := handler.HighlightedContent(paste)
			// Result must always be valid (non-panicking); empty content may yield empty.
			_ = result
		})
	}
}

// ─── CreatePaste extra branches ───────────────────────────────────────────────

func TestCreatePaste_RawBody(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader("raw paste content from curl"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Title", "My Raw Paste")
	req.Header.Set("X-Language", "text")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rr.Code, rr.Body.String())
	}
}

func TestCreatePaste_PlainTextResponse(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/paste",
		strings.NewReader("hello plain"))
	req.Header.Set("Content-Type", "text/plain")
	// No Accept or /api/ prefix → plain text response.

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rr.Code, rr.Body.String())
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type: got %q, want text/plain", ct)
	}
}

func TestCreatePaste_BurnAfter(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(`{"content":"burn after 3 reads","burn_after":3}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	ba := int(data["burn_after"].(float64))
	if ba != 3 {
		t.Errorf("burn_after: got %d, want 3", ba)
	}
}

func TestCreatePaste_Unlisted(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(`{"content":"secret paste","visibility":"unlisted"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	vis := int(data["visibility"].(float64))
	if vis != model.VisibilityUnlisted {
		t.Errorf("visibility: got %d, want %d (unlisted)", vis, model.VisibilityUnlisted)
	}
}

func TestCreatePaste_WithExpiry(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(`{"content":"expires soon","expires_in":"1h"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["expires_at"] == nil {
		t.Error("expires_at should be set")
	}
}

func TestCreatePaste_MultipartForm_Content(t *testing.T) {
	h, _ := newTestHandler(t)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("content", "multipart paste content")
	_ = w.WriteField("title", "Multipart Title")
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rr.Code, rr.Body.String())
	}
}

func TestCreatePaste_MultipartForm_FileUpload(t *testing.T) {
	h, _ := newTestHandler(t)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("files", "main.go")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	io.WriteString(fw, "package main\n\nfunc main() {}")
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["language"] != "go" {
		t.Errorf("language: got %v, want go (detected from main.go)", data["language"])
	}
}

func TestCreatePaste_FormWithBurnAfter(t *testing.T) {
	h, _ := newTestHandler(t)

	form := url.Values{
		"content":    {"burn after 2 from form"},
		"burn_after": {"2"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201\nbody: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	ba := int(data["burn_after"].(float64))
	if ba != 2 {
		t.Errorf("burn_after: got %d, want 2", ba)
	}
}

// ─── GetPaste burn_after via HTTP ─────────────────────────────────────────────

func TestGetPaste_BurnAfter(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"burn on read","burn_after":1}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/paste/"+id, nil)
	req.Header.Set("Accept", "application/json")
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.GetPaste(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("first read: status %d, want 200", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/paste/"+id, nil)
	req2.Header.Set("Accept", "application/json")
	req2 = withID(req2, id)

	rr2 := httptest.NewRecorder()
	h.GetPaste(rr2, req2)

	if rr2.Code != http.StatusNotFound {
		t.Errorf("second read (after burn): status %d, want 404", rr2.Code)
	}
}

// ─── GetRawPaste burn_after ───────────────────────────────────────────────────

func TestGetRawPaste_BurnAfter(t *testing.T) {
	h, _ := newTestHandler(t)

	m := createViaAPI(t, h, `{"content":"raw burn content","burn_after":1}`)
	data := m["data"].(map[string]interface{})
	id := data["id"].(string)

	req := httptest.NewRequest(http.MethodGet, "/"+id+"/raw", nil)
	req = withID(req, id)

	rr := httptest.NewRecorder()
	h.GetRawPaste(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/"+id+"/raw", nil)
	req2 = withID(req2, id)

	rr2 := httptest.NewRecorder()
	h.GetRawPaste(rr2, req2)

	if rr2.Code != http.StatusNotFound {
		t.Errorf("after burn: status %d, want 404", rr2.Code)
	}
}

// ─── CreatePaste HTTPS link ───────────────────────────────────────────────────

func TestCreatePaste_HTTPSLink(t *testing.T) {
	h, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/paste",
		strings.NewReader(`{"content":"https test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "paste.example.com"

	rr := httptest.NewRecorder()
	h.CreatePaste(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201", rr.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	link := data["link"].(string)
	if !strings.HasPrefix(link, "https://") {
		t.Errorf("link: got %q, want https:// prefix", link)
	}
}
