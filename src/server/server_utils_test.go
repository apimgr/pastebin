package server

// Tests for server utility functions: fmtUserDate, fmtUserTime,
// handleThemeSet, assetPrefix, resolveCORSOrigin,
// txtExtensionMiddleware, metricsIPAllowlistMiddleware.

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/config"
)

// ─── fmtUserDate / fmtUserTime ───────────────────────────────────────────────

func TestFmtUserDate_Zero(t *testing.T) {
	if got := fmtUserDate(time.Time{}); got != "" {
		t.Errorf("fmtUserDate(zero) = %q, want empty", got)
	}
}

func TestFmtUserDate_NonZero(t *testing.T) {
	ts := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	got := fmtUserDate(ts)
	if !strings.Contains(got, "2024") {
		t.Errorf("fmtUserDate(%v) = %q, expected year 2024", ts, got)
	}
	if !strings.Contains(got, "March") {
		t.Errorf("fmtUserDate(%v) = %q, expected month March", ts, got)
	}
	if !strings.Contains(got, "15") {
		t.Errorf("fmtUserDate(%v) = %q, expected day 15", ts, got)
	}
}

func TestFmtUserTime_Zero(t *testing.T) {
	if got := fmtUserTime(time.Time{}); got != "" {
		t.Errorf("fmtUserTime(zero) = %q, want empty", got)
	}
}

func TestFmtUserTime_NonZero(t *testing.T) {
	ts := time.Date(2024, 6, 1, 14, 5, 30, 0, time.UTC)
	got := fmtUserTime(ts)
	if got == "" {
		t.Errorf("fmtUserTime(%v) = empty, want non-empty", ts)
	}
	if !strings.Contains(got, "2024") {
		t.Errorf("fmtUserTime(%v) = %q, expected year 2024", ts, got)
	}
}

// ─── handleThemeSet ───────────────────────────────────────────────────────────

func TestHandleThemeSet_ValidTheme(t *testing.T) {
	for _, theme := range []string{"light", "dark", "auto"} {
		t.Run(theme, func(t *testing.T) {
			s := newMinimalServer(config.DefaultConfig())
			form := url.Values{"theme": {theme}}
			r := httptest.NewRequest(http.MethodPost, "/server/theme", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			s.handleThemeSet(w, r)

			if w.Code != http.StatusSeeOther {
				t.Errorf("theme=%s status = %d, want 303", theme, w.Code)
			}
			// Check that a cookie was set.
			setCookie := w.Header().Get("Set-Cookie")
			if !strings.Contains(setCookie, "theme="+theme) {
				t.Errorf("theme=%s: cookie %q missing theme value", theme, setCookie)
			}
		})
	}
}

func TestHandleThemeSet_InvalidTheme_DefaultsDark(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	form := url.Values{"theme": {"invalid"}}
	r := httptest.NewRequest(http.MethodPost, "/server/theme", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleThemeSet(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", w.Code)
	}
	setCookie := w.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, "theme=dark") {
		t.Errorf("cookie %q should default to dark", setCookie)
	}
}

func TestHandleThemeSet_RedirectsToReferer(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	form := url.Values{"theme": {"light"}}
	r := httptest.NewRequest(http.MethodPost, "/server/theme", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Referer", "http://example.com/pastes")
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.handleThemeSet(w, r)

	dest := w.Header().Get("Location")
	if dest != "/pastes" {
		t.Errorf("Location = %q, want /pastes", dest)
	}
}

func TestHandleThemeSet_CrossOriginReferer_RedirectsToRoot(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	form := url.Values{"theme": {"dark"}}
	r := httptest.NewRequest(http.MethodPost, "/server/theme", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Referer", "http://evil.com/phish")
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.handleThemeSet(w, r)

	dest := w.Header().Get("Location")
	if dest != "/" {
		t.Errorf("cross-origin referer: Location = %q, want /", dest)
	}
}

// ─── resolveCORSOrigin ────────────────────────────────────────────────────────

func TestResolveCORSOrigin_OperatorConfig_Matches(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Cors.AllowedOrigins = []string{"https://trusted.example.com"}
	cfg.Server.Cors.AllowCredentials = true
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://trusted.example.com")
	got, creds, disabled := s.resolveCORSOrigin(r)
	if got != "https://trusted.example.com" || !creds || disabled {
		t.Errorf("resolveCORSOrigin = (%q, %v, %v), want matched origin with credentials", got, creds, disabled)
	}
}

func TestResolveCORSOrigin_OperatorConfig_Unmatched(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Cors.AllowedOrigins = []string{"https://trusted.example.com"}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://other.com")
	got, _, _ := s.resolveCORSOrigin(r)
	if got != "*" {
		t.Errorf("resolveCORSOrigin = %q, want wildcard fallback for unmatched origin", got)
	}
}

func TestResolveCORSOrigin_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Cors.AllowedOrigins = []string{""}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://other.com")
	got, creds, disabled := s.resolveCORSOrigin(r)
	if !disabled || got != "" || creds {
		t.Errorf("resolveCORSOrigin = (%q, %v, %v), want disabled", got, creds, disabled)
	}
}

func TestResolveCORSOrigin_NoOriginHeader(t *testing.T) {
	cfg := config.DefaultConfig()
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got, _, _ := s.resolveCORSOrigin(r)
	if got != "*" {
		t.Errorf("resolveCORSOrigin (no Origin) = %q, want *", got)
	}
}

func TestResolveCORSOrigin_NoDomainLearner_ReturnsWildcard(t *testing.T) {
	cfg := config.DefaultConfig()
	// Clear operator CORS to exercise the learner/fallback path.
	cfg.Server.Cors.AllowedOrigins = nil
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://example.com")
	got, _, _ := s.resolveCORSOrigin(r)
	if got != "*" {
		t.Errorf("resolveCORSOrigin (no learner) = %q, want *", got)
	}
}

func TestResolveCORSOrigin_MatchedLearnedDomain(t *testing.T) {
	cfg := config.DefaultConfig()
	// Clear operator CORS to exercise the learner path.
	cfg.Server.Cors.AllowedOrigins = nil
	s := newMinimalServer(cfg)
	dl := newDomainLearner(&config.URLDetectionConfig{
		Learning: true, MinSamples: 1, SampleWindow: 5 * time.Minute,
	})
	dl.Observe("example.com")
	s.domainLearner = dl

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://example.com")
	got, _, _ := s.resolveCORSOrigin(r)
	if got != "https://example.com" {
		t.Errorf("resolveCORSOrigin (matched) = %q, want origin reflected", got)
	}
}

func TestResolveCORSOrigin_SubdomainMatchedLearnedDomain(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Cors.AllowedOrigins = nil
	s := newMinimalServer(cfg)
	dl := newDomainLearner(&config.URLDetectionConfig{
		Learning: true, MinSamples: 1, SampleWindow: 5 * time.Minute,
	})
	dl.Observe("example.com")
	s.domainLearner = dl

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://sub.example.com")
	got, _, _ := s.resolveCORSOrigin(r)
	if got != "https://sub.example.com" {
		t.Errorf("resolveCORSOrigin (subdomain match) = %q, want subdomain origin", got)
	}
}

func TestResolveCORSOrigin_NoMatch_ReturnsWildcard(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Cors.AllowedOrigins = nil
	s := newMinimalServer(cfg)
	dl := newDomainLearner(&config.URLDetectionConfig{
		Learning: true, MinSamples: 1, SampleWindow: 5 * time.Minute,
	})
	dl.Observe("example.com")
	s.domainLearner = dl

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "https://attacker.com")
	got, _, _ := s.resolveCORSOrigin(r)
	if got != "*" {
		t.Errorf("resolveCORSOrigin (no match) = %q, want *", got)
	}
}

// ─── txtExtensionMiddleware ───────────────────────────────────────────────────

func TestTxtExtensionMiddleware_NonAPIPath_PassThrough(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/foo.txt" {
			t.Errorf("path should be unchanged for non-API, got %q", r.URL.Path)
		}
	})
	h := s.txtExtensionMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/foo.txt", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !called {
		t.Error("next handler was not called")
	}
}

func TestTxtExtensionMiddleware_APIPath_StripsTxt(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	var gotPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	})
	h := s.txtExtensionMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/pastes/abc123.txt", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if gotPath != "/api/v1/pastes/abc123" {
		t.Errorf("path after strip = %q, want /api/v1/pastes/abc123", gotPath)
	}
}

func TestTxtExtensionMiddleware_APIPathNoTxt_PassThrough(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	var gotPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	})
	h := s.txtExtensionMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/pastes/abc123", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if gotPath != "/api/v1/pastes/abc123" {
		t.Errorf("path = %q, want /api/v1/pastes/abc123", gotPath)
	}
}

// ─── metricsIPAllowlistMiddleware ─────────────────────────────────────────────

func TestMetricsIPAllowlist_LoopbackAllowed(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := s.metricsIPAllowlistMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !called {
		t.Error("loopback should be allowed through")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestMetricsIPAllowlist_PublicIP_Denied(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.metricsIPAllowlistMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.RemoteAddr = "8.8.8.8:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("public IP status = %d, want 403", w.Code)
	}
}

func TestMetricsIPAllowlist_AllowedIP_Passes(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Metrics.AllowedIPs = []string{"203.0.113.5"}
	s := newMinimalServer(cfg)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := s.metricsIPAllowlistMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.RemoteAddr = "203.0.113.5:9100"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !called {
		t.Error("allowlisted IP should pass through")
	}
}

func TestMetricsIPAllowlist_IPv6Loopback_Allowed(t *testing.T) {
	s := newMinimalServer(config.DefaultConfig())
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	h := s.metricsIPAllowlistMiddleware(next)
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.RemoteAddr = "[::1]:9100"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !called {
		t.Error("IPv6 loopback should be allowed through")
	}
}

// ─── assetPrefix ─────────────────────────────────────────────────────────────

func TestAssetPrefix_NoBaseURL_NoHeader(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.BaseURL = ""
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := s.assetPrefix(r)
	if got != "" {
		t.Errorf("assetPrefix = %q, want empty", got)
	}
}

func TestAssetPrefix_BaseURLWithPath(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.BaseURL = "https://example.com/paste"
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := s.assetPrefix(r)
	if got != "/paste" {
		t.Errorf("assetPrefix = %q, want /paste", got)
	}
}

func TestAssetPrefix_BaseURLRootPath_Empty(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.BaseURL = "https://example.com/"
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := s.assetPrefix(r)
	if got != "" {
		t.Errorf("assetPrefix (root path) = %q, want empty", got)
	}
}
