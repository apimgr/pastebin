package handler

// Tests for the stikked, hastebin/haste-server, dpaste, and curl-upload
// (sprunge/0x0/ix.io) compatibility handlers. Package is `handler` so the
// shared test helpers (newCompatHandler, withID, seedPaste) are reused.

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ─── stikked ────────────────────────────────────────────────────────────────

func TestStikkedCreate_Success(t *testing.T) {
	h, _ := newCompatHandler(t)

	form := url.Values{"text": {"hello stikked"}, "title": {"t"}, "lang": {"go"}, "expire": {"60"}, "private": {"1"}}
	req := httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.StikkedCreate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/view/") {
		t.Fatalf("body = %q, want a /view/ URL", rr.Body.String())
	}
}

func TestStikkedCreate_EmptyText(t *testing.T) {
	h, _ := newCompatHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/create", strings.NewReader("text="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.StikkedCreate(rr, req)

	if !strings.HasPrefix(rr.Body.String(), "Error:") {
		t.Fatalf("body = %q, want an Error: prefix", rr.Body.String())
	}
}

func TestStikkedJSON_FoundAndMissing(t *testing.T) {
	h, _ := newCompatHandler(t)
	id, _ := seedPaste(t, h.ph, "stikked json body")

	req := withID(httptest.NewRequest(http.MethodGet, "/api/paste/"+id, nil), id)
	rr := httptest.NewRecorder()
	h.StikkedJSON(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["raw"] != "stikked json body" || got["pid"] != id {
		t.Fatalf("unexpected payload: %v", got)
	}

	req = withID(httptest.NewRequest(http.MethodGet, "/api/paste/nope", nil), "nope")
	rr = httptest.NewRecorder()
	h.StikkedJSON(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", rr.Code)
	}
}

// ─── hastebin / haste-server ──────────────────────────────────────────────────

func TestHastebinCreateAndGet(t *testing.T) {
	h, _ := newCompatHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader("raw haste content"))
	rr := httptest.NewRecorder()
	h.HastebinCreate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200", rr.Code)
	}
	var created map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("invalid create json: %v", err)
	}
	key := created["key"]
	if key == "" {
		t.Fatal("create returned empty key")
	}

	req = withID(httptest.NewRequest(http.MethodGet, "/documents/"+key, nil), key)
	rr = httptest.NewRecorder()
	h.HastebinGet(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", rr.Code)
	}
	var fetched map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("invalid get json: %v", err)
	}
	if fetched["data"] != "raw haste content" || fetched["key"] != key {
		t.Fatalf("unexpected get payload: %v", fetched)
	}
}

func TestHastebinCreate_Empty(t *testing.T) {
	h, _ := newCompatHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/documents", strings.NewReader("   "))
	rr := httptest.NewRecorder()
	h.HastebinCreate(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestHastebinGet_Missing(t *testing.T) {
	h, _ := newCompatHandler(t)
	req := withID(httptest.NewRequest(http.MethodGet, "/documents/nope", nil), "nope")
	rr := httptest.NewRecorder()
	h.HastebinGet(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

// ─── dpaste ───────────────────────────────────────────────────────────────────

func dpastePost(t *testing.T, h *CompatHandler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.DpasteCreate(rr, req)
	return rr
}

func TestDpasteCreate_Formats(t *testing.T) {
	h, _ := newCompatHandler(t)

	// default → quoted URL.
	rr := dpastePost(t, h, url.Values{"content": {"x"}, "expires": {"7"}})
	if rr.Code != http.StatusOK || !strings.HasPrefix(rr.Body.String(), `"`) {
		t.Fatalf("default format body = %q (status %d)", rr.Body.String(), rr.Code)
	}

	// url → bare URL.
	rr = dpastePost(t, h, url.Values{"content": {"x"}, "format": {"url"}})
	if strings.HasPrefix(rr.Body.String(), `"`) || !strings.Contains(rr.Body.String(), "http") {
		t.Fatalf("url format body = %q", rr.Body.String())
	}

	// json → object with url/content/lexer.
	rr = dpastePost(t, h, url.Values{"content": {"y"}, "lexer": {"go"}, "format": {"json"}})
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if got["content"] != "y" || got["lexer"] != "go" || got["url"] == "" {
		t.Fatalf("unexpected json payload: %v", got)
	}
}

func TestDpasteCreate_Empty(t *testing.T) {
	h, _ := newCompatHandler(t)
	rr := dpastePost(t, h, url.Values{"content": {""}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

// ─── curl-upload family ───────────────────────────────────────────────────────

func TestRootUpload_Sprunge(t *testing.T) {
	h, _ := newCompatHandler(t)
	form := url.Values{"sprunge": {"sprunge body"}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.RootUpload(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "/raw/") {
		t.Fatalf("sprunge body = %q (status %d), want a /raw/ URL", rr.Body.String(), rr.Code)
	}
}

func TestRootUpload_IxIo(t *testing.T) {
	h, _ := newCompatHandler(t)
	form := url.Values{"f:1": {"ixio body"}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.RootUpload(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "/raw/") {
		t.Fatalf("ix.io body = %q (status %d)", rr.Body.String(), rr.Code)
	}
}

func TestRootUpload_ZeroXMultipartFile(t *testing.T) {
	h, _ := newCompatHandler(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "paste.txt")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("0x0 file content"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rr := httptest.NewRecorder()
	h.RootUpload(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("0x0 status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "/raw/") {
		t.Fatalf("0x0 body = %q, want a /raw/ URL", rr.Body.String())
	}
	if rr.Header().Get("X-Token") == "" {
		t.Fatal("0x0 response missing X-Token header")
	}
}

// ─── termbin / fiche (raw TCP) ────────────────────────────────────────────────

// termbinRoundTrip drives TermbinServe over an in-memory socket pair: it writes
// content, half-closes, and returns the server's response line.
func termbinRoundTrip(t *testing.T, h *CompatHandler, content string, maxSize int64) string {
	t.Helper()
	srvConn, cliConn := net.Pipe()
	// net.Pipe has no half-close, so the server's read ends on the deadline; a
	// short timeout keeps the test fast.
	go h.TermbinServe(srvConn, "http://paste.example", maxSize, 250*time.Millisecond)

	_ = cliConn.SetDeadline(time.Now().Add(3 * time.Second))
	// Write in a goroutine: when the server reads less than the full input
	// (over-limit case) the remaining write blocks until the server closes, so
	// the write error here is expected and ignored.
	go func() {
		_, _ = cliConn.Write([]byte(content))
	}()
	resp, _ := io.ReadAll(cliConn)
	cliConn.Close()
	return string(resp)
}

func TestTermbinServe_Success(t *testing.T) {
	h, _ := newCompatHandler(t)
	resp := termbinRoundTrip(t, h, "termbin content\n", 32768)
	if !strings.HasPrefix(resp, "http://paste.example/") {
		t.Fatalf("response = %q, want a base URL line", resp)
	}
	if !strings.HasSuffix(resp, "\n") {
		t.Fatalf("response = %q, want trailing newline", resp)
	}
}

func TestTermbinServe_Empty(t *testing.T) {
	h, _ := newCompatHandler(t)
	resp := termbinRoundTrip(t, h, "   \n", 32768)
	if !strings.HasPrefix(resp, "Error:") {
		t.Fatalf("response = %q, want an Error: line", resp)
	}
}

func TestTermbinServe_TooLarge(t *testing.T) {
	h, _ := newCompatHandler(t)
	resp := termbinRoundTrip(t, h, strings.Repeat("x", 64), 16)
	if !strings.Contains(resp, "too large") {
		t.Fatalf("response = %q, want a too-large error", resp)
	}
}

func TestRootUpload_FallthroughToCreate(t *testing.T) {
	h, _ := newCompatHandler(t)

	// No curl-upload field present → delegate to the native create handler.
	form := url.Values{"content": {"native create"}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	h.RootUpload(rr, req)

	// The native create handler responds (redirect or success), never a 404.
	if rr.Code == http.StatusNotFound {
		t.Fatalf("fallthrough status = %d, native create did not run", rr.Code)
	}
}
