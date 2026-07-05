package server

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/pastebin/src/common/i18n"
	"github.com/apimgr/pastebin/src/config"
)

func newErrorTestServer(t *testing.T) *Server {
	t.Helper()
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"t":               func(lang, key string) string { return i18n.Translate(lang, key) },
		"i18njs":          func(lang string) template.JS { return template.JS(i18n.JSBundle(lang)) },
		"markdown":        func(s string) template.HTML { return renderMarkdown(s) },
		"trackingEnabled": func() bool { return false },
		"trackingScript":  func() template.HTML { return "" },
		"consentConfig":   func() template.JS { return template.JS("{}") },
		"fmtTime":         fmtUserTime,
		"fmtDate":         fmtUserDate,
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	return &Server{
		cfg:       &config.Config{Web: config.WebConfig{SiteTitle: "Pastebin", Theme: "dark"}},
		templates: tmpl,
		version:   "test",
		buildDate: "2026-01-01",
	}
}

func TestErrorCodeForStatus(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:          "BAD_REQUEST",
		http.StatusUnauthorized:        "UNAUTHORIZED",
		http.StatusForbidden:           "FORBIDDEN",
		http.StatusNotFound:            "NOT_FOUND",
		http.StatusMethodNotAllowed:    "METHOD_NOT_ALLOWED",
		http.StatusBadGateway:          "BAD_GATEWAY",
		http.StatusServiceUnavailable:  "MAINTENANCE",
		http.StatusInternalServerError: "SERVER_ERROR",
		http.StatusTeapot:              "SERVER_ERROR",
	}
	for status, want := range cases {
		if got := errorCodeForStatus(status); got != want {
			t.Errorf("status %d: got %q, want %q", status, got, want)
		}
	}
}

func TestRenderErrorPageJSON(t *testing.T) {
	s := newErrorTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("Accept", "application/json")

	s.renderErrorPage(rec, req, http.StatusNotFound, "gone")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type: got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{`"ok": false`, `"error": "NOT_FOUND"`, `"message": "gone"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

func TestRenderErrorPageHTML(t *testing.T) {
	s := newErrorTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("Accept", "text/html")

	s.renderErrorPage(rec, req, http.StatusNotFound, "no such paste")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("content-type: got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"404", "Not Found", "no such paste", `class="theme-dark"`, "Pastebin"} {
		if !strings.Contains(body, want) {
			t.Errorf("html body missing %q", want)
		}
	}
}

func TestRenderErrorPageDefaultMessage(t *testing.T) {
	s := newErrorTestServer(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept", "application/json")

	s.renderErrorPage(rec, req, http.StatusInternalServerError, "")

	if !strings.Contains(rec.Body.String(), "Internal Server Error") {
		t.Errorf("expected default status text in body: %s", rec.Body.String())
	}
}

func TestRenderErrorPageNoTemplatesFallback(t *testing.T) {
	s := &Server{cfg: &config.Config{Web: config.WebConfig{SiteTitle: "P", Theme: "dark"}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept", "text/html")

	s.renderErrorPage(rec, req, http.StatusNotFound, "boom")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("fallback body missing message: %s", rec.Body.String())
	}
}

func TestRecovererRendersThemed500(t *testing.T) {
	s := newErrorTestServer(t)
	h := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "500") {
		t.Errorf("expected themed 500 page")
	}
}
