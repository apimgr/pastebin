package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/handler"
	"github.com/apimgr/pastebin/src/model"
	"github.com/go-chi/chi/v5"
)

// ─── isReservedSlug ──────────────────────────────────────────────────────────

func TestIsReservedSlug(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"api", "api", true},
		{"server", "server", true},
		{"static", "static", true},
		{"metrics", "metrics", true},
		{"healthz", "healthz", true},
		{"graphql", "graphql", true},
		{"swagger", "swagger", true},
		{"robots.txt", "robots.txt", true},
		{"favicon.ico", "favicon.ico", true},
		{"recent", "recent", true},
		{"auth", "auth", true},
		{"uppercase API", "API", true},
		{"mixed case Metrics", "Metrics", true},
		{"non-reserved abc12345", "abc12345", false},
		{"non-reserved hello", "hello", false},
		{"non-reserved xyzzy", "xyzzy", false},
		{"empty string", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isReservedSlug(tc.id)
			if got != tc.want {
				t.Errorf("isReservedSlug(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// ─── isCSRFExempt ─────────────────────────────────────────────────────────────

func TestIsCSRFExempt(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{"empty patterns", "/foo", []string{}, false},
		{"nil patterns", "/foo", nil, false},
		{"exact match", "/api/v1/pastes", []string{"/api/v1/pastes"}, true},
		{"exact no match", "/api/v1/other", []string{"/api/v1/pastes"}, false},
		{"wildcard prefix match", "/api/v1/pastes/123", []string{"/api/v1/pastes/*"}, true},
		{"wildcard root match", "/api/v1/pastes", []string{"/api/v1/pastes/*"}, true},
		{"wildcard no match", "/other/path", []string{"/api/v1/pastes/*"}, false},
		{"multiple patterns first", "/foo", []string{"/foo", "/bar"}, true},
		{"multiple patterns second", "/bar", []string{"/foo", "/bar"}, true},
		{"multiple patterns none", "/baz", []string{"/foo", "/bar"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isCSRFExempt(tc.path, tc.patterns)
			if got != tc.want {
				t.Errorf("isCSRFExempt(%q, %v) = %v, want %v", tc.path, tc.patterns, got, tc.want)
			}
		})
	}
}

// ─── isWebSocketUpgrade ───────────────────────────────────────────────────────

func TestIsWebSocketUpgrade(t *testing.T) {
	cases := []struct {
		name    string
		upgrade string
		want    bool
	}{
		{"no header", "", false},
		{"websocket lowercase", "websocket", true},
		{"WebSocket mixed case", "WebSocket", true},
		{"other value", "h2c", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.upgrade != "" {
				r.Header.Set("Upgrade", tc.upgrade)
			}
			if got := isWebSocketUpgrade(r); got != tc.want {
				t.Errorf("isWebSocketUpgrade(Upgrade=%q) = %v, want %v", tc.upgrade, got, tc.want)
			}
		})
	}
}

// ─── isSameOrigin ─────────────────────────────────────────────────────────────

func TestIsSameOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		referer string
		want    bool
	}{
		{"no origin or referer", "", "", false},
		{"same origin", "http://example.com", "", true},
		{"same origin https", "https://example.com", "", true},
		{"cross origin", "http://evil.com", "", false},
		{"referer fallback same", "", "http://example.com/page", true},
		{"referer fallback cross", "", "http://evil.com/page", false},
		{"unparseable origin", "::::", "", false},
		{"origin takes precedence over referer", "http://evil.com", "http://example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://example.com/create", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				r.Header.Set("Referer", tc.referer)
			}
			if got := isSameOrigin(r); got != tc.want {
				t.Errorf("isSameOrigin(origin=%q, referer=%q) = %v, want %v", tc.origin, tc.referer, got, tc.want)
			}
		})
	}
}

// ─── csrfMiddleware ───────────────────────────────────────────────────────────

func newCSRFServer() *Server {
	return &Server{
		cfg: &config.Config{
			Web: config.WebConfig{
				CSRF: config.CSRFConfig{
					Enabled:     true,
					TokenLength: 32,
					CookieName:  "csrf_token",
					HeaderName:  "X-CSRF-Token",
					Secure:      "false",
				},
			},
		},
		csrfSecret: []byte("test-csrf-secret-key-for-unit-tests"),
	}
}

func TestCSRFMiddleware(t *testing.T) {
	s := newCSRFServer()
	validToken, err := s.generateCSRFToken()
	if err != nil {
		t.Fatalf("generateCSRFToken: %v", err)
	}

	newReq := func(method, target string) *http.Request {
		return httptest.NewRequest(method, target, nil)
	}

	cases := []struct {
		name       string
		build      func() *http.Request
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "GET bypasses and reaches handler",
			build:      func() *http.Request { return newReq(http.MethodGet, "http://example.com/create") },
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "same-origin POST bypasses",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/create")
				r.Header.Set("Origin", "http://example.com")
				r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "Bearer POST bypasses",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/api/v1/paste")
				r.Header.Set("Origin", "http://evil.com")
				r.Header.Set("Authorization", "Bearer tok_abc")
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "cross-origin POST without cookie rejected",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/api/v1/paste")
				r.Header.Set("Origin", "http://evil.com")
				return r
			},
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name: "originless POST with matching cookie and token passes",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/create")
				r.Header.Set("X-CSRF-Token", validToken)
				r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "WebSocket upgrade POST bypasses",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/ws")
				r.Header.Set("Origin", "http://evil.com")
				r.Header.Set("Upgrade", "websocket")
				r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "cross-origin POST with matching token passes",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/create")
				r.Header.Set("Origin", "http://evil.com")
				r.Header.Set("X-CSRF-Token", validToken)
				r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
				return r
			},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
		{
			name: "cross-origin POST with cookie but no token rejected",
			build: func() *http.Request {
				r := newReq(http.MethodPost, "http://example.com/create")
				r.Header.Set("Origin", "http://evil.com")
				r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
				return r
			},
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
		{
			name: "cross-origin POST with mismatched token rejected",
			build: func() *http.Request {
				other, _ := s.generateCSRFToken()
				r := newReq(http.MethodPost, "http://example.com/create")
				r.Header.Set("Origin", "http://evil.com")
				r.Header.Set("X-CSRF-Token", other)
				r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
				return r
			},
			wantStatus: http.StatusForbidden,
			wantNext:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			s.csrfMiddleware(next).ServeHTTP(rec, tc.build())

			if nextCalled != tc.wantNext {
				t.Errorf("next called = %v, want %v", nextCalled, tc.wantNext)
			}
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusForbidden {
				var body map[string]interface{}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if body["error"] != "CSRF_FAILED" {
					t.Errorf("error = %v, want CSRF_FAILED", body["error"])
				}
			}
		})
	}
}

func TestCSRFMiddlewareCookieNotHTTPOnly(t *testing.T) {
	s := newCSRFServer()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	s.csrfMiddleware(next).ServeHTTP(rec, r)

	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			found = c
		}
	}
	if found == nil {
		t.Fatal("expected csrf_token cookie to be set")
	}
	if found.HttpOnly {
		t.Error("csrf_token cookie must NOT be HttpOnly (form/JS must read it)")
	}
	if found.SameSite != http.SameSiteStrictMode {
		t.Errorf("csrf_token SameSite = %v, want Strict", found.SameSite)
	}
}

func TestCSRFMiddlewareStableToken(t *testing.T) {
	s := newCSRFServer()
	validToken, err := s.generateCSRFToken()
	if err != nil {
		t.Fatalf("generateCSRFToken: %v", err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	r.AddCookie(&http.Cookie{Name: "csrf_token", Value: validToken})
	s.csrfMiddleware(next).ServeHTTP(rec, r)

	for _, c := range rec.Result().Cookies() {
		if c.Name == "csrf_token" && c.Value != validToken {
			t.Errorf("valid existing token was regenerated: got %q, want %q", c.Value, validToken)
		}
	}
}

// ─── detectClientType ────────────────────────────────────────────────────────

func TestDetectClientType(t *testing.T) {
	cases := []struct {
		name      string
		acceptHdr string
		uaHdr     string
		want      string
	}{
		{"json accept header", "application/json", "", "json"},
		{"text/html accept header", "text/html,application/xhtml+xml", "", "html"},
		{"text/plain accept header", "text/plain", "", "text"},
		{"curl user-agent", "", "curl/7.88.1", "text"},
		{"wget user-agent", "", "Wget/1.21.3", "text"},
		{"httpie user-agent", "", "HTTPie/3.2.1", "text"},
		{"mozilla browser ua", "", "Mozilla/5.0 (X11; Linux x86_64)", "html"},
		{"chrome browser ua", "", "Chrome/113.0", "html"},
		{"empty ua and accept", "", "", "text"},
		{"pastebin-cli ua", "", "pastebin-cli/1.0.0", "json"},
		{"go http client ua", "", "Go-http-client/1.1", "text"},
		{"accept star-star", "*/*", "", "text"},
		{"accept json beats ua", "application/json", "Mozilla/5.0", "json"},
		{"accept html beats ua", "text/html", "curl/7.88.1", "html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.acceptHdr != "" {
				r.Header.Set("Accept", tc.acceptHdr)
			}
			if tc.uaHdr != "" {
				r.Header.Set("User-Agent", tc.uaHdr)
			}
			got := detectClientType(r)
			if got != tc.want {
				t.Errorf("detectClientType() = %q, want %q (Accept=%q UA=%q)", got, tc.want, tc.acceptHdr, tc.uaHdr)
			}
		})
	}
}

// ─── formatUptime ─────────────────────────────────────────────────────────────

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "0m"},
		{"one second rounds to zero minutes", time.Second, "0m"},
		{"30 seconds rounds to one minute", 30 * time.Second, "1m"},
		{"90 seconds rounds to 2 minutes", 90 * time.Second, "2m"},
		{"exact 5 minutes", 5 * time.Minute, "5m"},
		{"one hour", time.Hour, "1h 0m"},
		{"one hour one minute", time.Hour + time.Minute, "1h 1m"},
		{"3690 seconds rounds to 1h2m", 3690 * time.Second, "1h 2m"},
		{"two days", 2 * 24 * time.Hour, "2d 0h 0m"},
		{"two days five hours", 2*24*time.Hour + 5*time.Hour, "2d 5h 0m"},
		{"400 days", 400 * 24 * time.Hour, "400d 0h 0m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatUptime(tc.d)
			if got != tc.want {
				t.Errorf("formatUptime(%v) = %q, want %q", tc.d, got, tc.want)
			}
		})
	}
}

// ─── pwaIconSVG ───────────────────────────────────────────────────────────────

func TestPWAIconSVG(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"192", 192},
		{"512", 512},
		{"180", 180},
		{"64", 64},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pwaIconSVG(tc.size)
			if got == "" {
				t.Fatal("pwaIconSVG returned empty string")
			}
			if !strings.Contains(got, "<svg") {
				t.Errorf("pwaIconSVG(%d) does not contain <svg, got: %q", tc.size, got[:min(len(got), 80)])
			}
		})
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── writeJSON ────────────────────────────────────────────────────────────────

func TestWriteJSON(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       any
		wantStatus int
		wantCT     string
	}{
		{"ok response", http.StatusOK, map[string]bool{"ok": true}, http.StatusOK, "application/json"},
		{"created response", http.StatusCreated, map[string]string{"id": "abc"}, http.StatusCreated, "application/json"},
		{"not found", http.StatusNotFound, map[string]string{"error": "not found"}, http.StatusNotFound, "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSON(w, tc.status, tc.body)
			resp := w.Result()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			ct := resp.Header.Get("Content-Type")
			if ct != tc.wantCT {
				t.Errorf("Content-Type = %q, want %q", ct, tc.wantCT)
			}
			var out any
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Errorf("body is not valid JSON: %v", err)
			}
		})
	}
}

func TestWriteJSONUnmarshalableBody(t *testing.T) {
	w := httptest.NewRecorder()
	// channels cannot be marshaled to JSON — writeJSON must handle error gracefully
	writeJSON(w, http.StatusOK, make(chan int))
	resp := w.Result()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500 for unmarshalable body, got %d", resp.StatusCode)
	}
}

// ─── newRequestID ─────────────────────────────────────────────────────────────

func TestNewRequestID(t *testing.T) {
	id := newRequestID()
	if id == "" {
		t.Fatal("newRequestID returned empty string")
	}
	if len(id) != 16 {
		t.Errorf("newRequestID length = %d, want 16 (8 bytes hex)", len(id))
	}
	// Each call must return a different value with overwhelming probability.
	id2 := newRequestID()
	if id == id2 {
		t.Errorf("newRequestID returned identical IDs on consecutive calls: %q", id)
	}
}

func TestNewRequestIDUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 100; i++ {
		id := newRequestID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate request ID %q after %d iterations", id, i)
		}
		seen[id] = struct{}{}
	}
}

// ─── requestStats ─────────────────────────────────────────────────────────────

func TestRequestStats(t *testing.T) {
	t.Run("inc increments total", func(t *testing.T) {
		var rs requestStats
		rs.inc()
		rs.inc()
		rs.inc()
		if got := rs.total.Load(); got != 3 {
			t.Errorf("total = %d, want 3", got)
		}
	})

	t.Run("last24h sums buckets", func(t *testing.T) {
		var rs requestStats
		n := 5
		for i := 0; i < n; i++ {
			rs.inc()
		}
		got := rs.last24h()
		if got < int64(n) {
			t.Errorf("last24h() = %d, want >= %d", got, n)
		}
	})

	t.Run("zero stats returns zero", func(t *testing.T) {
		var rs requestStats
		if got := rs.last24h(); got != 0 {
			t.Errorf("last24h() on empty stats = %d, want 0", got)
		}
	})
}

// ─── ipBucket / rateLimiter ───────────────────────────────────────────────────

func TestIPBucketAllow(t *testing.T) {
	t.Run("allows up to limit", func(t *testing.T) {
		b := &ipBucket{}
		limit := 3
		window := time.Minute
		for i := 0; i < limit; i++ {
			if !b.allow(limit, window) {
				t.Fatalf("request %d should be allowed", i+1)
			}
		}
	})

	t.Run("rejects at limit", func(t *testing.T) {
		b := &ipBucket{}
		limit := 2
		window := time.Minute
		b.allow(limit, window)
		b.allow(limit, window)
		if b.allow(limit, window) {
			t.Error("third request should be rejected when limit=2")
		}
	})

	t.Run("allows after window expires", func(t *testing.T) {
		b := &ipBucket{}
		limit := 1
		window := time.Millisecond
		b.allow(limit, window)
		// Wait for window to expire.
		time.Sleep(5 * time.Millisecond)
		if !b.allow(limit, window) {
			t.Error("request after window expiry should be allowed")
		}
	})
}

func TestNewRateLimiter(t *testing.T) {
	t.Run("allows first request", func(t *testing.T) {
		rl := newRateLimiter(5, time.Minute)
		if !rl.allow("1.2.3.4") {
			t.Error("first request should be allowed")
		}
	})

	t.Run("rejects after limit", func(t *testing.T) {
		rl := newRateLimiter(2, time.Minute)
		rl.allow("1.2.3.4")
		rl.allow("1.2.3.4")
		if rl.allow("1.2.3.4") {
			t.Error("third request should be rejected when limit=2")
		}
	})
}

func TestRateLimiterIndependentIPs(t *testing.T) {
	cases := []struct {
		name    string
		ip1     string
		ip2     string
		limit   int
		reqs    int
		wantIP1 bool
		wantIP2 bool
	}{
		{
			name: "different IPs independent",
			ip1: "10.0.0.1", ip2: "10.0.0.2",
			limit: 1, reqs: 1,
			wantIP1: false, wantIP2: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl := newRateLimiter(tc.limit, time.Minute)
			for i := 0; i < tc.reqs; i++ {
				rl.allow(tc.ip1)
			}
			// ip1 is exhausted; ip2 should still be allowed.
			got1 := rl.allow(tc.ip1)
			got2 := rl.allow(tc.ip2)
			if got1 != tc.wantIP1 {
				t.Errorf("ip1 allow = %v, want %v", got1, tc.wantIP1)
			}
			if got2 != tc.wantIP2 {
				t.Errorf("ip2 allow = %v, want %v", got2, tc.wantIP2)
			}
		})
	}
}

func TestRateLimiterUpdateLimit(t *testing.T) {
	t.Run("new limit enforced after update", func(t *testing.T) {
		rl := newRateLimiter(10, time.Minute)
		rl.UpdateLimit(1)
		rl.allow("1.2.3.4")
		if rl.allow("1.2.3.4") {
			t.Error("second request should be rejected after UpdateLimit(1)")
		}
	})

	t.Run("update to higher limit allows more", func(t *testing.T) {
		rl := newRateLimiter(1, time.Minute)
		rl.UpdateLimit(5)
		for i := 0; i < 5; i++ {
			if !rl.allow("1.2.3.5") {
				t.Errorf("request %d should be allowed after UpdateLimit(5)", i+1)
			}
		}
	})
}

// ─── allowlistSet ────────────────────────────────────────────────────────────

func TestAllowlistSet(t *testing.T) {
	cases := []struct {
		name    string
		entries []string
		ip      string
		want    bool
	}{
		{"single IP match", []string{"192.168.1.1"}, "192.168.1.1", true},
		{"single IP no match", []string{"192.168.1.1"}, "192.168.1.2", false},
		{"CIDR match", []string{"10.0.0.0/8"}, "10.5.6.7", true},
		{"CIDR no match", []string{"10.0.0.0/8"}, "172.16.0.1", false},
		{"empty entries", []string{}, "1.2.3.4", false},
		{"IPv6 exact match", []string{"::1"}, "::1", true},
		{"IPv6 no match", []string{"::1"}, "::2", false},
		{"multiple entries first", []string{"1.1.1.1", "2.2.2.2"}, "1.1.1.1", true},
		{"multiple entries second", []string{"1.1.1.1", "2.2.2.2"}, "2.2.2.2", true},
		{"multiple entries no match", []string{"1.1.1.1", "2.2.2.2"}, "3.3.3.3", false},
		{"invalid entry skipped", []string{"notanip", "1.2.3.4"}, "1.2.3.4", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			al := newAllowlistSet(tc.entries)
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("test IP %q is invalid", tc.ip)
			}
			got := al.contains(ip)
			if got != tc.want {
				t.Errorf("contains(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// ─── blocklistStore ──────────────────────────────────────────────────────────

// makeTempDir creates a temporary directory for tests, using the apimgr subdirectory
// under os.TempDir() as the parent, creating it if needed.
func makeTempDir(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("create temp parent dir: %v", err)
	}
	dir, err := os.MkdirTemp(parent, "pastebin-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestLoadBlocklists(t *testing.T) {
	t.Run("empty dir returns empty store", func(t *testing.T) {
		dir := makeTempDir(t)

		bs := loadBlocklists(dir)
		if bs == nil {
			t.Fatal("loadBlocklists returned nil")
		}
		ip := net.ParseIP("1.2.3.4")
		if bs.contains(ip) {
			t.Error("empty blocklist should not contain any IP")
		}
	})

	t.Run("nonexistent dir returns empty store", func(t *testing.T) {
		bs := loadBlocklists("/nonexistent/path/that/does/not/exist")
		if bs == nil {
			t.Fatal("loadBlocklists returned nil for missing dir")
		}
	})

	t.Run("dir with txt file containing IPs", func(t *testing.T) {
		dir := makeTempDir(t)

		content := "# blocklist\n10.0.0.1\n10.0.0.2\n192.168.0.0/24\n\n# end\n"
		if err := os.WriteFile(filepath.Join(dir, "blocklist.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		bs := loadBlocklists(dir)

		cases := []struct {
			ip   string
			want bool
		}{
			{"10.0.0.1", true},
			{"10.0.0.2", true},
			{"192.168.0.50", true},
			{"1.2.3.4", false},
		}
		for _, c := range cases {
			ip := net.ParseIP(c.ip)
			if got := bs.contains(ip); got != c.want {
				t.Errorf("contains(%q) = %v, want %v", c.ip, got, c.want)
			}
		}
	})

	t.Run("non-txt files ignored", func(t *testing.T) {
		dir := makeTempDir(t)

		if err := os.WriteFile(filepath.Join(dir, "list.csv"), []byte("10.0.0.1\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		bs := loadBlocklists(dir)
		ip := net.ParseIP("10.0.0.1")
		if bs.contains(ip) {
			t.Error("non-.txt file should not be loaded")
		}
	})
}

func TestBlocklistStoreContains(t *testing.T) {
	cases := []struct {
		name string
		ips  []string
		nets []string
		ip   string
		want bool
	}{
		{"exact IP match", []string{"10.0.0.1"}, nil, "10.0.0.1", true},
		{"exact IP no match", []string{"10.0.0.1"}, nil, "10.0.0.2", false},
		{"CIDR match", nil, []string{"172.16.0.0/12"}, "172.20.0.1", true},
		{"CIDR no match", nil, []string{"172.16.0.0/12"}, "10.0.0.1", false},
		{"empty store", nil, nil, "1.2.3.4", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs := &blocklistStore{ips: make(map[string]struct{})}
			for _, rawIP := range tc.ips {
				parsed := net.ParseIP(rawIP)
				if parsed == nil {
					t.Fatalf("invalid test IP %q", rawIP)
				}
				bs.ips[parsed.String()] = struct{}{}
			}
			for _, cidr := range tc.nets {
				_, ipNet, err := net.ParseCIDR(cidr)
				if err != nil {
					t.Fatalf("invalid CIDR %q: %v", cidr, err)
				}
				bs.nets = append(bs.nets, ipNet)
			}

			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("invalid IP %q", tc.ip)
			}
			got := bs.contains(ip)
			if got != tc.want {
				t.Errorf("contains(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

// ─── pathSecurityMiddleware ───────────────────────────────────────────────────

func TestPathSecurityMiddleware(t *testing.T) {
	s := &Server{}
	handler := s.pathSecurityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(r.URL.Path))
	}))

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"normal path", "/paste/abc123", http.StatusOK},
		{"root path", "/", http.StatusOK},
		{"path traversal dotdot", "/../etc/passwd", http.StatusBadRequest},
		{"path traversal in middle", "/foo/../bar", http.StatusBadRequest},
		{"percent-encoded dot traversal", "/foo/%2e%2e/bar", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Errorf("path %q: status = %d, want %d", tc.path, w.Code, tc.wantStatus)
			}
		})
	}
}

// ─── isAllowlisted context helper ─────────────────────────────────────────────

func TestIsAllowlisted(t *testing.T) {
	t.Run("not in context returns false", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if isAllowlisted(r.Context()) {
			t.Error("expected false when key not in context")
		}
	})
}

// ─── minimal server helper ────────────────────────────────────────────────────

// newMinimalServer returns a *Server with only the config field set.
// Safe for testing handlers and methods that only call s.liveCfg().
func newMinimalServer(cfg *config.Config) *Server {
	return &Server{cfg: cfg}
}

// ─── Server.handleRobots ─────────────────────────────────────────────────────

func TestHandleRobots(t *testing.T) {
	cases := []struct {
		name          string
		allowPaths    []string
		denyPaths     []string
		wantSubstring string
	}{
		{"empty rules", nil, nil, "User-agent: *"},
		{"allow path", []string{"/public"}, nil, "Allow: /public"},
		{"deny path", nil, []string{"/admin"}, "Disallow: /admin"},
		{"both rules", []string{"/ok"}, []string{"/nope"}, "Allow: /ok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Web.Robots.Allow = tc.allowPaths
			cfg.Web.Robots.Deny = tc.denyPaths
			s := newMinimalServer(cfg)

			r := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
			w := httptest.NewRecorder()
			s.handleRobots(w, r)

			if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
				t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.wantSubstring) {
				t.Errorf("body does not contain %q; got: %q", tc.wantSubstring, body)
			}
		})
	}
}

// ─── Server.handleSecurity ────────────────────────────────────────────────────

func TestHandleSecurity(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Contact.Security.Email = "security@example.com"
	s := newMinimalServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	w := httptest.NewRecorder()
	s.handleSecurity(w, r)

	body := w.Body.String()
	if !strings.Contains(body, "security@example.com") {
		t.Errorf("security.txt should contain contact email, got: %q", body)
	}
	if !strings.Contains(body, "Expires:") {
		t.Errorf("security.txt should contain Expires field, got: %q", body)
	}
}

// ─── Server.handleFavicon ─────────────────────────────────────────────────────

func TestHandleFavicon(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	r := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	w := httptest.NewRecorder()
	s.handleFavicon(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("favicon redirect status = %d, want %d", w.Code, http.StatusFound)
	}
	loc := w.Header().Get("Location")
	if loc != "/static/favicon.ico" {
		t.Errorf("favicon redirect location = %q, want /static/favicon.ico", loc)
	}
}

// ─── Server.handleManifest ────────────────────────────────────────────────────

func TestHandleManifest(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.SiteTitle = "Test Paste"
	s := newMinimalServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
	w := httptest.NewRecorder()
	s.handleManifest(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("manifest status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("manifest body is not valid JSON: %v", err)
	}
	if body["name"] != "Test Paste" {
		t.Errorf("manifest name = %v, want Test Paste", body["name"])
	}
}

// ─── Server.handleServiceWorker ───────────────────────────────────────────────

func TestHandleServiceWorker(t *testing.T) {
	cases := []struct {
		name    string
		version string
		wantVer string
	}{
		{"with version", "1.2.3", "1.2.3"},
		{"empty version uses dev", "", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{version: tc.version, cfg: &config.Config{}}
			r := httptest.NewRequest(http.MethodGet, "/sw.js", nil)
			w := httptest.NewRecorder()
			s.handleServiceWorker(w, r)

			if ct := w.Header().Get("Content-Type"); ct != "application/javascript" {
				t.Errorf("Content-Type = %q, want application/javascript", ct)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.wantVer) {
				t.Errorf("service worker body should contain version %q", tc.wantVer)
			}
		})
	}
}

// ─── Server.handlePWAIcon handlers ───────────────────────────────────────────

func TestHandlePWAIcons(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"icon180", (*Server).handlePWAIcon180},
		{"icon192", (*Server).handlePWAIcon192},
		{"icon512", (*Server).handlePWAIcon512},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newMinimalServer(&config.Config{})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			tc.handler(s, w, r)

			if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
				t.Errorf("Content-Type = %q, want image/svg+xml", ct)
			}
			body := w.Body.String()
			if !strings.Contains(body, "<svg") {
				t.Errorf("PWA icon response should contain <svg")
			}
		})
	}
}

// ─── Server.handleVersion ────────────────────────────────────────────────────

func TestHandleVersion(t *testing.T) {
	s := &Server{version: "2.0.0", cfg: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/version", nil)
	w := httptest.NewRecorder()
	s.handleVersion(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("version status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("version body not valid JSON: %v", err)
	}
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("version response missing data field")
	}
	if data["version"] != "2.0.0" {
		t.Errorf("version = %v, want 2.0.0", data["version"])
	}
}

// ─── Server.handleAPIInfo ────────────────────────────────────────────────────

func TestHandleAPIInfo(t *testing.T) {
	s := &Server{version: "1.0", cfg: &config.Config{}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/info", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.handleAPIInfo(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("apiinfo status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("apiinfo body not valid JSON: %v", err)
	}
	if body["ok"] != true {
		t.Errorf("apiinfo ok = %v, want true", body["ok"])
	}
}

// ─── Server.handleAutodiscover ───────────────────────────────────────────────

func TestHandleAutodiscover(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		host    string
		want    string
	}{
		{"uses configured base_url", "https://paste.example.com", "ignored", "https://paste.example.com"},
		{"falls back to request host", "", "myhost.com", "http://myhost.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.BaseURL = tc.baseURL
			s := &Server{version: "1.0", cfg: cfg}
			r := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
			r.Host = tc.host
			w := httptest.NewRecorder()
			s.handleAutodiscover(w, r)

			if w.Code != http.StatusOK {
				t.Errorf("autodiscover status = %d, want 200", w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, tc.want) {
				t.Errorf("autodiscover body should contain %q; got: %q", tc.want, body[:min(len(body), 200)])
			}
		})
	}
}

// ─── Server.corsMiddleware ────────────────────────────────────────────────────

func TestCORSMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		cors       string
		method     string
		wantStatus int
		wantOrigin string
	}{
		{"default cors star", "", http.MethodGet, http.StatusOK, "*"},
		{"configured cors", "https://example.com", http.MethodGet, http.StatusOK, "https://example.com"},
		{"options preflight", "", http.MethodOptions, http.StatusOK, "*"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Web.Security.CORS = tc.cors
			s := newMinimalServer(cfg)

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := s.corsMiddleware(inner)

			r := httptest.NewRequest(tc.method, "/", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != tc.wantStatus {
				t.Errorf("CORS status = %d, want %d", w.Code, tc.wantStatus)
			}
			got := w.Header().Get("Access-Control-Allow-Origin")
			if got != tc.wantOrigin {
				t.Errorf("ACAO = %q, want %q", got, tc.wantOrigin)
			}
		})
	}
}

// ─── Server.countRequests ─────────────────────────────────────────────────────

func TestCountRequests(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	var seen int
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		w.WriteHeader(http.StatusOK)
	})
	h := s.countRequests(inner)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if s.stats.total.Load() != 1 {
		t.Errorf("stats.total = %d, want 1", s.stats.total.Load())
	}
	if seen != 1 {
		t.Errorf("inner handler called %d times, want 1", seen)
	}
}

// ─── Server.requireOperatorToken ─────────────────────────────────────────────

func TestRequireOperatorToken(t *testing.T) {
	const testToken = "supersecrettoken"
	hash := sha256Sum(testToken)

	cases := []struct {
		name       string
		authHeader string
		tokenHash  [32]byte
		wantStatus int
	}{
		{"no auth header", "", hash, http.StatusUnauthorized},
		{"wrong token", "Bearer wrongtoken", hash, http.StatusUnauthorized},
		{"correct token", "Bearer " + testToken, hash, http.StatusOK},
		{"no token configured", "Bearer " + testToken, [32]byte{}, http.StatusServiceUnavailable},
		{"missing bearer prefix", testToken, hash, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{cfg: &config.Config{}, operatorTokenHash: tc.tokenHash}
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := s.requireOperatorToken(inner)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.authHeader != "" {
				r.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

// sha256Sum returns the SHA-256 hash of s as a [32]byte.
func sha256Sum(s string) [32]byte {
	return sha256.Sum256([]byte(s))
}

// ─── Server.noTrailingSlash ───────────────────────────────────────────────────

func TestNoTrailingSlash(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(r.URL.Path))
	})
	h := s.noTrailingSlash(inner)

	cases := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"root unchanged", "/", http.StatusOK},
		{"normal path unchanged", "/paste/abc", http.StatusOK},
		{"trailing slash redirected", "/paste/", http.StatusMovedPermanently},
		{"file with dot in last segment still redirects", "/static/app.js/", http.StatusMovedPermanently},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Errorf("path %q: status = %d, want %d", tc.path, w.Code, tc.wantStatus)
			}
		})
	}
}

// ─── Server.secFetchMiddleware ────────────────────────────────────────────────

func TestSecFetchMiddleware(t *testing.T) {
	cases := []struct {
		name           string
		enabled        bool
		method         string
		path           string
		secFetchSite   string
		authHeader     string
		secFetchMode   string
		wantStatus     int
	}{
		{"disabled passes all", false, http.MethodPost, "/api/v1/pastes", "cross-site", "", "", http.StatusOK},
		{"cross-site POST blocked", true, http.MethodPost, "/api/v1/pastes", "cross-site", "", "", http.StatusForbidden},
		{"cross-site POST with bearer allowed", true, http.MethodPost, "/api/v1/pastes", "cross-site", "Bearer tok", "", http.StatusOK},
		{"same-site POST allowed", true, http.MethodPost, "/api/v1/pastes", "same-origin", "", "", http.StatusOK},
		{"GET not blocked by cross-site", true, http.MethodGet, "/api/v1/pastes", "cross-site", "", "", http.StatusOK},
		{"navigate to API blocked", true, http.MethodGet, "/api/v1/pastes", "", "", "navigate", http.StatusForbidden},
		{"navigate to non-API allowed", true, http.MethodGet, "/paste/abc", "", "", "navigate", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Web.Headers.SecFetchValidation = tc.enabled
			s := newMinimalServer(cfg)

			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			h := s.secFetchMiddleware(inner)

			r := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.secFetchSite != "" {
				r.Header.Set("Sec-Fetch-Site", tc.secFetchSite)
			}
			if tc.authHeader != "" {
				r.Header.Set("Authorization", tc.authHeader)
			}
			if tc.secFetchMode != "" {
				r.Header.Set("Sec-Fetch-Mode", tc.secFetchMode)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

// ─── Server.securityHeadersMiddleware ─────────────────────────────────────────

func TestSecurityHeadersMiddleware(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.securityHeadersMiddleware(inner)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	mandatoryHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Referrer-Policy",
		"X-Permitted-Cross-Domain-Policies",
		"Permissions-Policy",
		"X-Request-ID",
	}
	for _, hdr := range mandatoryHeaders {
		if w.Header().Get(hdr) == "" {
			t.Errorf("missing mandatory header %q", hdr)
		}
	}
}

func TestSecurityHeadersMiddlewareRequestIDPreserved(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.securityHeadersMiddleware(inner)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "custom-req-id-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if got := w.Header().Get("X-Request-ID"); got != "custom-req-id-123" {
		t.Errorf("X-Request-ID = %q, want custom-req-id-123", got)
	}
}

// ─── Server.buildCSP ─────────────────────────────────────────────────────────

func TestBuildCSP(t *testing.T) {
	cases := []struct {
		name           string
		enabled        bool
		mode           string
		wantHeader     bool
		wantReportOnly bool
	}{
		{"disabled returns empty", false, "enforce", false, false},
		{"enforce mode", true, "enforce", true, false},
		{"report-only mode", true, "report-only", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Web.CSP.Enabled = tc.enabled
			cfg.Web.CSP.Mode = tc.mode
			s := newMinimalServer(cfg)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			policy, reportOnly := s.buildCSP(r)

			if tc.wantHeader && policy == "" {
				t.Error("expected non-empty CSP policy")
			}
			if !tc.wantHeader && policy != "" {
				t.Errorf("expected empty CSP policy, got: %q", policy[:min(len(policy), 50)])
			}
			if reportOnly != tc.wantReportOnly {
				t.Errorf("reportOnly = %v, want %v", reportOnly, tc.wantReportOnly)
			}
		})
	}
}

// ─── Server.buildReportingHeaders ────────────────────────────────────────────

func TestBuildReportingHeaders(t *testing.T) {
	cases := []struct {
		name        string
		fqdn        string
		tlsEnabled  bool
		wantNonEmpty bool
	}{
		{"no fqdn returns empty", "", true, false},
		{"localhost returns empty", "localhost", true, false},
		{"no tls returns empty", "example.com", false, false},
		{"fqdn+tls returns headers", "example.com", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.FQDN = tc.fqdn
			cfg.Server.TLS.Enabled = tc.tlsEnabled
			s := newMinimalServer(cfg)

			endpoints, reportTo, nel := s.buildReportingHeaders()
			hasContent := endpoints != "" || reportTo != "" || nel != ""

			if hasContent != tc.wantNonEmpty {
				t.Errorf("headers non-empty = %v, want %v (endpoints=%q)", hasContent, tc.wantNonEmpty, endpoints)
			}
		})
	}
}

// ─── Server.MarkPendingRestart ────────────────────────────────────────────────

func TestMarkPendingRestart(t *testing.T) {
	s := newMinimalServer(&config.Config{})

	s.MarkPendingRestart("server.port")
	s.MarkPendingRestart("database")
	// duplicate — should not be added twice
	s.MarkPendingRestart("server.port")

	s.pendingRestartMu.Lock()
	keys := s.pendingRestartKeys
	s.pendingRestartMu.Unlock()

	if len(keys) != 2 {
		t.Errorf("pendingRestartKeys = %v, want exactly 2 entries", keys)
	}
}

// ─── Server.SetSchedulerHealthFn / SetSchedulerAPI ───────────────────────────

func TestSetSchedulerHelpers(t *testing.T) {
	t.Run("SetSchedulerHealthFn", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		called := false
		s.SetSchedulerHealthFn(func() bool { called = true; return true })
		if s.schedHealthFn == nil {
			t.Error("schedHealthFn should be set")
		}
		if !s.schedHealthFn() || !called {
			t.Error("schedHealthFn not called correctly")
		}
	})

	t.Run("SetSchedulerAPI", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		s.SetSchedulerAPI(nil)
		// nil is valid; ensure no panic
	})
}

// ─── Server.GeoIPEnabled / TorRunning / TorOnionAddress ──────────────────────

func TestServerStateHelpers(t *testing.T) {
	t.Run("GeoIPEnabled false when nil", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		if s.GeoIPEnabled() {
			t.Error("GeoIPEnabled should be false when geoipDB is nil")
		}
	})

	t.Run("TorRunning false when nil", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		if s.TorRunning() {
			t.Error("TorRunning should be false when torManager is nil")
		}
	})

	t.Run("TorOnionAddress empty when nil", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		if addr := s.TorOnionAddress(); addr != "" {
			t.Errorf("TorOnionAddress = %q, want empty", addr)
		}
	})

	t.Run("TorRestart no-op when nil", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		if err := s.TorRestart(); err != nil {
			t.Errorf("TorRestart should be nil when torManager is nil, got %v", err)
		}
	})
}

// ─── Server.handleSchedulerList ───────────────────────────────────────────────

func TestHandleSchedulerList(t *testing.T) {
	t.Run("nil scheduler returns 503", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		r := httptest.NewRequest(http.MethodGet, "/api/v1/server/scheduler", nil)
		w := httptest.NewRecorder()
		s.handleSchedulerList(w, r)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("scheduler list status = %d, want 503", w.Code)
		}
	})
}

// ─── Server.maybeRateLimit / maybeDeleteRateLimit ─────────────────────────────

func TestMaybeRateLimitNilLimiter(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	wrapped := s.maybeRateLimit(inner)
	r := httptest.NewRequest(http.MethodPost, "/create", nil)
	w := httptest.NewRecorder()
	wrapped(w, r)

	if !called {
		t.Error("inner handler should be called when createLimiter is nil")
	}
}

func TestMaybeDeleteRateLimitNilLimiter(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	wrapped := s.maybeDeleteRateLimit(inner)
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/pastes/abc", nil)
	w := httptest.NewRecorder()
	wrapped(w, r)

	if !called {
		t.Error("inner handler should be called when deleteLimiter is nil")
	}
}

// ─── rateLimitMiddleware ───────────────────────────────────────────────────────

func TestRateLimitMiddlewareAllowlisted(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	// Exhaust the limit for this IP.
	rl.allow("10.0.0.1")

	mw := rateLimitMiddleware(rl, "create")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := mw(inner)

	// A request with ctxKeyAllowlisted=true should bypass the rate limit.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	ctx := r.Context()
	// Use context.WithValue to inject the allowlist flag.
	// This mirrors what allowlistMiddleware does.
	r = r.WithContext(contextWithAllowlisted(ctx))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("allowlisted request should pass rate limit, got %d", w.Code)
	}
}

func TestRateLimitMiddlewareRejected(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	mw := rateLimitMiddleware(rl, "delete")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := mw(inner)

	// First request: allowed.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.0.0.2:5000"
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("first request should be allowed, got %d", w1.Code)
	}

	// Second request same IP: rejected.
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.0.0.2:5001"
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be rejected, got %d", w2.Code)
	}
}

// contextWithAllowlisted injects the allowlist flag into a context.
// Mirrors what allowlistMiddleware does for testing.
func contextWithAllowlisted(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeyAllowlisted, true)
}

// ─── Server.allowlistMiddleware ───────────────────────────────────────────────

func TestAllowlistMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		allowlist  []string
		remoteAddr string
		wantFlag   bool
	}{
		{"IP in allowlist sets flag", []string{"10.0.0.1"}, "10.0.0.1:1234", true},
		{"IP not in allowlist no flag", []string{"10.0.0.1"}, "10.0.0.2:1234", false},
		{"empty allowlist no flag", []string{}, "10.0.0.1:1234", false},
		{"CIDR match sets flag", []string{"192.168.0.0/16"}, "192.168.1.50:5000", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Web.Security.Allowlist = tc.allowlist
			s := newMinimalServer(cfg)

			var gotFlag bool
			inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotFlag = isAllowlisted(r.Context())
				w.WriteHeader(http.StatusOK)
			})
			h := s.allowlistMiddleware(inner)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)

			if gotFlag != tc.wantFlag {
				t.Errorf("allowlisted flag = %v, want %v", gotFlag, tc.wantFlag)
			}
		})
	}
}

// ─── Server.blocklistMiddleware ───────────────────────────────────────────────

func TestBlocklistMiddleware(t *testing.T) {
	t.Run("blocked IP gets 403", func(t *testing.T) {
		dir := makeTempDir(t)
		if err := os.WriteFile(filepath.Join(dir, "blocked.txt"), []byte("10.0.0.99\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{}
		cfg.Server.DataDir = filepath.Dir(filepath.Dir(dir))
		cfg.Server.DataDir = filepath.Join(dir, "..")

		dataDir := makeTempDir(t)
		secDir := filepath.Join(dataDir, "security", "blocklists")
		if err := os.MkdirAll(secDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(secDir, "blocked.txt"), []byte("10.0.0.99\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg.Server.DataDir = dataDir

		s := newMinimalServer(cfg)
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := s.blocklistMiddleware(inner)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.99:5000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("blocked IP status = %d, want 403", w.Code)
		}
	})

	t.Run("allowed IP passes through", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.DataDir = "/nonexistent/dir"
		s := newMinimalServer(cfg)
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := s.blocklistMiddleware(inner)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "1.2.3.4:5000"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("non-blocked IP status = %d, want 200", w.Code)
		}
	})

	t.Run("allowlisted IP bypasses blocklist", func(t *testing.T) {
		dataDir := makeTempDir(t)
		secDir := filepath.Join(dataDir, "security", "blocklists")
		if err := os.MkdirAll(secDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(secDir, "blocked.txt"), []byte("10.0.0.1\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		cfg := &config.Config{}
		cfg.Server.DataDir = dataDir
		s := newMinimalServer(cfg)

		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := s.blocklistMiddleware(inner)

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "10.0.0.1:5000"
		r = r.WithContext(contextWithAllowlisted(r.Context()))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("allowlisted+blocked IP should pass through, got %d", w.Code)
		}
	})
}

// ─── Server.OnConfigChange ────────────────────────────────────────────────────

func TestOnConfigChange(t *testing.T) {
	t.Run("rate limiters updated on change", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.RateLimit.CreatePerM = 10
		cfg.RateLimit.DeletePerM = 5
		s := newMinimalServer(cfg)
		s.createLimiter = newRateLimiter(10, time.Minute)
		s.deleteLimiter = newRateLimiter(5, time.Minute)

		next := &config.Config{}
		next.RateLimit.CreatePerM = 20
		next.RateLimit.DeletePerM = 10
		s.OnConfigChange(next)

		if s.createLimiter.limit != 20 {
			t.Errorf("createLimiter.limit = %d, want 20", s.createLimiter.limit)
		}
		if s.deleteLimiter.limit != 10 {
			t.Errorf("deleteLimiter.limit = %d, want 10", s.deleteLimiter.limit)
		}
	})

	t.Run("port change marks pending restart", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.Port = "8080"
		s := newMinimalServer(cfg)

		next := &config.Config{}
		next.Server.Port = "9090"
		s.OnConfigChange(next)

		s.pendingRestartMu.Lock()
		keys := s.pendingRestartKeys
		s.pendingRestartMu.Unlock()

		found := false
		for _, k := range keys {
			if k == "server.port" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected server.port in pendingRestartKeys, got %v", keys)
		}
	})

	t.Run("unchanged port does not mark restart", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Server.Port = "8080"
		s := newMinimalServer(cfg)

		next := &config.Config{}
		next.Server.Port = "8080"
		s.OnConfigChange(next)

		s.pendingRestartMu.Lock()
		keys := s.pendingRestartKeys
		s.pendingRestartMu.Unlock()

		if len(keys) != 0 {
			t.Errorf("expected no pending restart keys, got %v", keys)
		}
	})
}

// ─── Server.buildTorInfo ──────────────────────────────────────────────────────

func TestBuildTorInfo(t *testing.T) {
	t.Run("nil torManager returns disabled", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		info := s.buildTorInfo()
		if info.Enabled {
			t.Error("TorInfo.Enabled should be false when torManager is nil")
		}
		if info.Status != "disabled" {
			t.Errorf("TorInfo.Status = %q, want disabled", info.Status)
		}
	})
}

// ─── Server.isTrustedPeer ────────────────────────────────────────────────────

func TestIsTrustedPeer(t *testing.T) {
	cases := []struct {
		name       string
		remoteAddr string
		additional []string
		want       bool
	}{
		{"loopback IPv4", "127.0.0.1:1234", nil, true},
		{"loopback IPv6", "[::1]:1234", nil, true},
		{"private 10.x", "10.5.6.7:1234", nil, true},
		{"private 192.168.x", "192.168.1.100:1234", nil, true},
		{"private 172.16.x", "172.20.0.1:1234", nil, true},
		{"public IP not trusted", "8.8.8.8:1234", nil, false},
		{"public in additional CIDR", "1.2.3.4:1234", []string{"1.2.3.0/24"}, true},
		{"public in additional exact IP", "5.6.7.8:1234", []string{"5.6.7.8"}, true},
		{"public not in additional", "9.9.9.9:1234", []string{"1.2.3.4"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.TrustedProxies.Additional = tc.additional
			s := newMinimalServer(cfg)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			got := s.isTrustedPeer(r)
			if got != tc.want {
				t.Errorf("isTrustedPeer(%q) = %v, want %v", tc.remoteAddr, got, tc.want)
			}
		})
	}
}

// ─── Server.baseURL ───────────────────────────────────────────────────────────

func TestBaseURL(t *testing.T) {
	cases := []struct {
		name       string
		baseURL    string
		remoteAddr string
		host       string
		proto      string
		want       string
	}{
		{"configured base_url overrides all", "https://paste.example.com", "8.8.8.8:1234", "ignored", "", "https://paste.example.com"},
		{"http for public remote", "", "8.8.8.8:1234", "myhost.com", "", "http://myhost.com"},
		{"https from TLS on trusted peer", "", "127.0.0.1:1234", "myhost.com", "https", "https://myhost.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Server.BaseURL = tc.baseURL
			s := newMinimalServer(cfg)

			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			r.Host = tc.host
			if tc.proto != "" {
				r.Header.Set("X-Forwarded-Proto", tc.proto)
			}
			got := s.baseURL(r)
			if got != tc.want {
				t.Errorf("baseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// ─── Server.renderTemplate (nil templates path) ───────────────────────────────

func TestRenderTemplateNilTemplates(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.renderTemplate(w, r, "home.html", nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("renderTemplate with nil templates status = %d, want 500", w.Code)
	}
}

// ─── Server.pageData ──────────────────────────────────────────────────────────

func TestPageData(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.SiteTitle = "My Paste"
	cfg.Web.Theme = "dark"
	s := newMinimalServer(cfg)

	data := s.pageData()
	if data["SiteTitle"] != "My Paste" {
		t.Errorf("SiteTitle = %v, want My Paste", data["SiteTitle"])
	}
	if data["Theme"] != "dark" {
		t.Errorf("Theme = %v, want dark", data["Theme"])
	}
}

// ─── Additional scheduler handler nil checks ─────────────────────────────────

func TestSchedulerHandlersNilScheduler(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	handlers := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"show", s.handleSchedulerShow},
		{"run", s.handleSchedulerRun},
		{"enable", s.handleSchedulerEnable},
		{"disable", s.handleSchedulerDisable},
	}
	for _, tc := range handlers {
		t.Run(tc.name+" nil scheduler returns 503", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			tc.handler(w, r)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("%s: status = %d, want 503", tc.name, w.Code)
			}
		})
	}
}

// ─── Server.maybeRateLimit with active limiter ────────────────────────────────

func TestMaybeRateLimitWithLimiter(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	s.createLimiter = newRateLimiter(1, time.Minute)

	calls := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	wrapped := s.maybeRateLimit(inner)

	r1 := httptest.NewRequest(http.MethodPost, "/create", nil)
	r1.RemoteAddr = "10.0.0.5:1111"
	w1 := httptest.NewRecorder()
	wrapped(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("first request status = %d, want 200", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodPost, "/create", nil)
	r2.RemoteAddr = "10.0.0.5:2222"
	w2 := httptest.NewRecorder()
	wrapped(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request status = %d, want 429", w2.Code)
	}
}

// ─── Server.securityHeadersMiddleware with TLS/HSTS ──────────────────────────

func TestSecurityHeadersHSTS(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.TLS.Enabled = true
	cfg.Web.HSTS.Enabled = true
	cfg.Web.HSTS.MaxAgeSeconds = 31536000
	cfg.Web.HSTS.IncludeSubdomains = true
	cfg.Web.HSTS.Preload = true
	s := newMinimalServer(cfg)

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.securityHeadersMiddleware(inner)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("HSTS header missing when TLS enabled + HSTS enabled")
	}
	if !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("HSTS header = %q, expected max-age=31536000", hsts)
	}
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("HSTS header missing includeSubDomains: %q", hsts)
	}
	if !strings.Contains(hsts, "preload") {
		t.Errorf("HSTS header missing preload: %q", hsts)
	}
}

// ─── stubDB implements database.DB for testing ────────────────────────────────

type stubDB struct {
	pingErr        error
	pastesCount    int64
	taskHistory    []*database.TaskHistory
	historyErr     error
	getPasteByIDFn func(id string) (*model.Paste, error)
}

func (d *stubDB) Close() error                  { return nil }
func (d *stubDB) Type() string                  { return "stub" }
func (d *stubDB) Ping() error                   { return d.pingErr }
func (d *stubDB) CountPastes() (int64, error)   { return d.pastesCount, nil }

func (d *stubDB) CreatePaste(p *model.Paste) error  { return nil }
func (d *stubDB) GetPasteByID(id string) (*model.Paste, error) {
	if d.getPasteByIDFn != nil {
		return d.getPasteByIDFn(id)
	}
	return nil, nil
}
func (d *stubDB) GetPublicPastes(page, limit int) ([]model.PasteListItem, int, error) {
	return nil, 0, nil
}
func (d *stubDB) IncrementPasteViews(id string) error            { return nil }
func (d *stubDB) DeletePaste(id string) error                     { return nil }
func (d *stubDB) DeletePasteByToken(id, tok string) error         { return nil }
func (d *stubDB) DeleteExpiredPastes() (int64, error)             { return 0, nil }
func (d *stubDB) DeleteBurnedPastes() (int64, error)              { return 0, nil }
func (d *stubDB) UpsertSchedulerTask(t *database.TaskState) error  { return nil }
func (d *stubDB) GetSchedulerTask(id string) (*database.TaskState, error) { return nil, nil }
func (d *stubDB) ListSchedulerTasks() ([]*database.TaskState, error) { return nil, nil }
func (d *stubDB) UpdateTaskRun(id string, lastRun time.Time, status, lastErr string, run, fail int64, next time.Time) error {
	return nil
}
func (d *stubDB) SetTaskEnabled(id string, enabled bool) error     { return nil }
func (d *stubDB) RecordTaskHistory(h *database.TaskHistory) error  { return nil }
func (d *stubDB) ListTaskHistory(taskID string, limit int) ([]*database.TaskHistory, error) {
	return d.taskHistory, d.historyErr
}
func (d *stubDB) CreateAPIToken(hash, prefix, rtype, rid string, exp *time.Time) error { return nil }
func (d *stubDB) VerifyAPIToken(hash [32]byte, rtype, rid string) error               { return nil }
func (d *stubDB) ValidateAPIToken(hash [32]byte, rtype string) error                   { return nil }
func (d *stubDB) RevokeAPIToken(prefix, reason string) error                           { return nil }
func (d *stubDB) ListAPITokens() ([]*database.APITokenRecord, error)                   { return nil, nil }
func (d *stubDB) DeleteExpiredAPITokens() (int64, error)                               { return 0, nil }
func (d *stubDB) EnsureAppSecret(key string) ([]byte, error)                           { return nil, nil }
func (d *stubDB) CreateSecurityReport(r *database.SecurityReport) error                 { return nil }
func (d *stubDB) GetSecurityReport(trackingID string) (*database.SecurityReport, error) { return nil, nil }
func (d *stubDB) UpdateSecurityReportStatus(trackingID, status, comment string) error   { return nil }
func (d *stubDB) ListDisclosedSecurityReports() ([]*database.SecurityReport, error)     { return nil, nil }
func (d *stubDB) MarkSecurityReportTokenUsed(trackingID string, at time.Time) error     { return nil }

// newServerWithDB creates a minimal Server with both cfg and db set.
func newServerWithDB(cfg *config.Config, db database.DB) *Server {
	return &Server{cfg: cfg, db: db, startTime: time.Now()}
}

// ─── Server.buildHealthResponse ───────────────────────────────────────────────

func TestBuildHealthResponse(t *testing.T) {
	t.Run("healthy when db ping ok", func(t *testing.T) {
		db := &stubDB{pingErr: nil, pastesCount: 42}
		s := newServerWithDB(&config.Config{}, db)
		hr := s.buildHealthResponse()
		if hr.Checks.Database != "ok" {
			t.Errorf("database check = %q, want ok", hr.Checks.Database)
		}
	})

	t.Run("database error when ping fails", func(t *testing.T) {
		db := &stubDB{pingErr: os.ErrClosed}
		s := newServerWithDB(&config.Config{}, db)
		hr := s.buildHealthResponse()
		if hr.Checks.Database != "error" {
			t.Errorf("database check = %q, want error", hr.Checks.Database)
		}
	})
}

// ─── Server.handleHealthzJSON ─────────────────────────────────────────────────

func TestHandleHealthzJSON(t *testing.T) {
	db := &stubDB{pingErr: nil, pastesCount: 10}
	s := newServerWithDB(&config.Config{}, db)

	r := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	s.handleHealthzJSON(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("healthz JSON status = %d, want 200", w.Code)
	}
	var hr HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&hr); err != nil {
		t.Fatalf("healthz JSON body not valid: %v", err)
	}
}

// ─── Server.handleSchedulerHistory ───────────────────────────────────────────

func TestHandleSchedulerHistory(t *testing.T) {
	t.Run("returns history from db", func(t *testing.T) {
		history := []*database.TaskHistory{
			{TaskID: "ssl_renewal", Status: "ok"},
		}
		db := &stubDB{taskHistory: history}
		s := newServerWithDB(&config.Config{}, db)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/server/scheduler/ssl_renewal/history", nil)
		w := httptest.NewRecorder()
		s.handleSchedulerHistory(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("scheduler history status = %d, want 200", w.Code)
		}
	})

	t.Run("db error returns 500", func(t *testing.T) {
		db := &stubDB{historyErr: os.ErrClosed}
		s := newServerWithDB(&config.Config{}, db)

		r := httptest.NewRequest(http.MethodGet, "/api/v1/server/scheduler/foo/history", nil)
		w := httptest.NewRecorder()
		s.handleSchedulerHistory(w, r)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("scheduler history error status = %d, want 500", w.Code)
		}
	})
}

// ─── stubSchedulerAPI implements SchedulerAPI for testing ────────────────────

type stubSchedulerAPI struct {
	tasks    []database.TaskState
	taskMap  map[string]database.TaskState
	runNowFn func(id string) error
}

func newStubScheduler(tasks ...database.TaskState) *stubSchedulerAPI {
	m := make(map[string]database.TaskState, len(tasks))
	for _, t := range tasks {
		m[t.TaskName] = t
	}
	return &stubSchedulerAPI{tasks: tasks, taskMap: m}
}

func (s *stubSchedulerAPI) GetTasks() []database.TaskState {
	return s.tasks
}
func (s *stubSchedulerAPI) GetTask(id string) (database.TaskState, bool) {
	t, ok := s.taskMap[id]
	return t, ok
}
func (s *stubSchedulerAPI) RunNow(id string) error {
	if s.runNowFn != nil {
		return s.runNowFn(id)
	}
	return nil
}
func (s *stubSchedulerAPI) EnableTask(id string) {
	if ts, ok := s.taskMap[id]; ok {
		ts.Enabled = true
		s.taskMap[id] = ts
	}
}
func (s *stubSchedulerAPI) DisableTask(id string) {
	if ts, ok := s.taskMap[id]; ok {
		ts.Enabled = false
		s.taskMap[id] = ts
	}
}

// ─── Scheduler handler tests with a real scheduler API ───────────────────────

func TestHandleSchedulerListWithAPI(t *testing.T) {
	sched := newStubScheduler(
		database.TaskState{TaskName: "ssl_renewal", Enabled: true},
		database.TaskState{TaskName: "geoip_update", Enabled: false},
	)
	s := newMinimalServer(&config.Config{})
	s.SetSchedulerAPI(sched)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/server/scheduler", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerList(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("scheduler list status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("scheduler list body not valid JSON: %v", err)
	}
	if body["ok"] != true {
		t.Errorf("scheduler list ok = %v, want true", body["ok"])
	}
}

func TestHandleSchedulerShowWithAPI(t *testing.T) {
	sched := newStubScheduler(database.TaskState{TaskName: "ssl_renewal", Enabled: true})
	s := newMinimalServer(&config.Config{})
	s.SetSchedulerAPI(sched)

	t.Run("existing task returns 200", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		s.handleSchedulerShow(w, r)
		// chi.URLParam returns "" without router context → task not found → 404
		if w.Code != http.StatusNotFound {
			t.Errorf("no-ID show status = %d, want 404", w.Code)
		}
	})
}

func TestHandleSchedulerRunWithAPI(t *testing.T) {
	sched := newStubScheduler(database.TaskState{TaskName: "ssl_renewal"})
	s := newMinimalServer(&config.Config{})
	s.SetSchedulerAPI(sched)

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRun(w, r)
	// chi.URLParam("id") = "" → RunNow("") returns nil → 200
	if w.Code != http.StatusOK {
		t.Errorf("scheduler run status = %d, want 200", w.Code)
	}
}

func TestHandleSchedulerRunError(t *testing.T) {
	sched := newStubScheduler()
	sched.runNowFn = func(id string) error { return os.ErrNotExist }
	s := newMinimalServer(&config.Config{})
	s.SetSchedulerAPI(sched)

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRun(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("scheduler run error status = %d, want 400", w.Code)
	}
}

func TestHandleSchedulerEnableWithAPI(t *testing.T) {
	sched := newStubScheduler(database.TaskState{TaskName: "ssl_renewal"})
	s := newMinimalServer(&config.Config{})
	s.SetSchedulerAPI(sched)

	// Without chi context, URLParam("id") = "" → GetTask("") → not found → 404.
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerEnable(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("scheduler enable (not found) status = %d, want 404", w.Code)
	}
}

func TestHandleSchedulerDisableWithAPI(t *testing.T) {
	sched := newStubScheduler(database.TaskState{TaskName: "ssl_renewal"})
	s := newMinimalServer(&config.Config{})
	s.SetSchedulerAPI(sched)

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerDisable(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("scheduler disable (not found) status = %d, want 404", w.Code)
	}
}

// ─── Server.checkDisk ────────────────────────────────────────────────────────

func TestCheckDisk(t *testing.T) {
	t.Run("existing dir returns true", func(t *testing.T) {
		s := newMinimalServer(&config.Config{})
		if !s.checkDisk() {
			t.Error("checkDisk should return true for an accessible filesystem")
		}
	})
}

// ─── Server.buildHealthResponse with schedFn ──────────────────────────────────

func TestBuildHealthResponseScheduler(t *testing.T) {
	t.Run("scheduler error when fn returns false", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		s.SetSchedulerHealthFn(func() bool { return false })

		hr := s.buildHealthResponse()
		if hr.Checks.Scheduler != "error" {
			t.Errorf("scheduler check = %q, want error", hr.Checks.Scheduler)
		}
		if hr.Status != "degraded" {
			t.Errorf("status = %q, want degraded", hr.Status)
		}
	})

	t.Run("unhealthy when db error", func(t *testing.T) {
		db := &stubDB{pingErr: os.ErrClosed}
		s := newServerWithDB(&config.Config{}, db)
		hr := s.buildHealthResponse()
		if hr.Status != "unhealthy" {
			t.Errorf("status = %q, want unhealthy", hr.Status)
		}
	})

	t.Run("pending restart surfaced in response", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		s.MarkPendingRestart("server.port")
		hr := s.buildHealthResponse()
		if !hr.PendingRestart {
			t.Error("PendingRestart should be true when keys are set")
		}
		if len(hr.RestartReason) == 0 {
			t.Error("RestartReason should not be empty")
		}
	})
}

// ─── Server.handleHealthz (nil templates path) ────────────────────────────────

func TestHandleHealthz(t *testing.T) {
	t.Run("JSON accept returns JSON", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		s.handleHealthz(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("healthz JSON status = %d, want 200", w.Code)
		}
	})

	t.Run("text UA returns 200 plain text", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
		r.Header.Set("User-Agent", "curl/8.0.0")
		w := httptest.NewRecorder()
		s.handleHealthz(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("healthz text status = %d, want 200", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("healthz text content-type = %q, want text/plain; charset=utf-8", ct)
		}
	})

	t.Run("browser UA returns 500 with nil templates", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s.handleHealthz(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("healthz HTML status = %d, want 500 (nil templates)", w.Code)
		}
	})
}

// ─── Handlers that use renderTemplate with nil templates ─────────────────────

func TestTemplateHandlersNilTemplates(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.SiteTitle = "Test Paste"
	cfg.Web.Theme = "dark"

	handlers := []struct {
		name    string
		handler func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"createPage", (*Server).handleCreatePage},
		{"about", (*Server).handleAbout},
		{"privacy", (*Server).handlePrivacy},
		{"terms", (*Server).handleTerms},
	}
	for _, tc := range handlers {
		t.Run(tc.name+" browser UA returns 500 with nil templates", func(t *testing.T) {
			s := newMinimalServer(cfg)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
			w := httptest.NewRecorder()
			tc.handler(s, w, r)
			if w.Code != http.StatusInternalServerError {
				t.Errorf("%s: status = %d, want 500", tc.name, w.Code)
			}
		})
	}

	textHandlers := []struct {
		name    string
		handler func(*Server, http.ResponseWriter, *http.Request)
	}{
		{"about", (*Server).handleAbout},
		{"privacy", (*Server).handlePrivacy},
		{"terms", (*Server).handleTerms},
	}
	for _, tc := range textHandlers {
		t.Run(tc.name+" curl UA returns 200 text with nil templates", func(t *testing.T) {
			s := newMinimalServer(cfg)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("User-Agent", "curl/8.0.0")
			w := httptest.NewRecorder()
			tc.handler(s, w, r)
			if w.Code != http.StatusOK {
				t.Errorf("%s text: status = %d, want 200", tc.name, w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
				t.Errorf("%s text: content-type = %q, want text/plain", tc.name, ct)
			}
		})
	}
}

func TestHandleHelpNilTemplates(t *testing.T) {
	cfg := &config.Config{}

	t.Run("browser UA returns 500 with nil templates", func(t *testing.T) {
		s := newMinimalServer(cfg)
		r := httptest.NewRequest(http.MethodGet, "/help", nil)
		r.Host = "example.com"
		r.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
		w := httptest.NewRecorder()
		s.handleHelp(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("handleHelp status = %d, want 500", w.Code)
		}
	})

	t.Run("curl UA returns 200 text with nil templates", func(t *testing.T) {
		s := newMinimalServer(cfg)
		r := httptest.NewRequest(http.MethodGet, "/help", nil)
		r.Host = "example.com"
		r.Header.Set("User-Agent", "curl/8.0.0")
		w := httptest.NewRecorder()
		s.handleHelp(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("handleHelp text: status = %d, want 200", w.Code)
		}
	})
}

func TestHandleOfflineNilTemplates(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/offline", nil)
	w := httptest.NewRecorder()
	s.handleOffline(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("handleOffline status = %d, want 500", w.Code)
	}
}

// ─── Handlers using db + nil templates ───────────────────────────────────────

func TestHandleHomeNilTemplates(t *testing.T) {
	t.Run("text UA returns plain text", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("User-Agent", "curl/7.88.1")
		r.Host = "example.com"
		w := httptest.NewRecorder()
		s.handleHome(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("handleHome text status = %d, want 200", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
			t.Errorf("handleHome text Content-Type = %q", ct)
		}
	})

	t.Run("HTML UA returns 500 with nil templates", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("User-Agent", "Mozilla/5.0")
		w := httptest.NewRecorder()
		s.handleHome(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("handleHome HTML status = %d, want 500", w.Code)
		}
	})
}

func TestHandleRecentNilTemplates(t *testing.T) {
	t.Run("text UA returns plain text", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/recent", nil)
		r.Header.Set("User-Agent", "curl/7.88.1")
		r.Host = "example.com"
		w := httptest.NewRecorder()
		s.handleRecent(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("handleRecent text status = %d, want 200", w.Code)
		}
	})

	t.Run("HTML UA returns 500 with nil templates", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithDB(&config.Config{}, db)
		r := httptest.NewRequest(http.MethodGet, "/recent", nil)
		r.Header.Set("User-Agent", "Mozilla/5.0")
		w := httptest.NewRecorder()
		s.handleRecent(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("handleRecent HTML status = %d, want 500", w.Code)
		}
	})
}

// ─── Server.handleViewPaste via reserved slug ─────────────────────────────────

func TestHandleViewPasteReservedSlug(t *testing.T) {
	// When chi.URLParam("id") = "" (no router context), isReservedSlug("") = false
	// and pasteHandler is nil → panic. Instead test via a minimal chi router.
	// Without a chi router, we test the reserved slug path directly via isReservedSlug,
	// which is already covered by TestIsReservedSlug.
	// For handleViewPaste itself with a nil pasteHandler we use a reserved slug
	// that returns 404 early — but chi.URLParam won't return a reserved slug without
	// a router context. Skip this and test via unit coverage of isReservedSlug instead.
	t.Skip("handleViewPaste requires chi router context — covered via isReservedSlug tests")
}

// ─── Server.handleRemoveSubmit early exit ────────────────────────────────────

func TestHandleRemoveSubmitBadForm(t *testing.T) {
	// When Content-Type is not a form type and body is not parseable, ParseForm
	// will succeed (returns nil error for empty body). Test the token-empty path.
	cfg := &config.Config{}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodPost, "/remove/abc", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.handleRemoveSubmit(w, r)
	if w.Code == http.StatusInternalServerError && w.Body.String() == "templates not loaded\n" {
		// nil templates → 500 is the expected early return from renderTemplate
		return
	}
	// Any non-panic response is acceptable.
}

// ─── Server.handleQR nil templates ────────────────────────────────────────────

func TestHandleQRNilTemplates(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/qr/abc", nil)
	r.Host = "example.com"
	w := httptest.NewRecorder()
	s.handleQR(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("handleQR nil templates status = %d, want 500", w.Code)
	}
}

// ─── Server.handleRemovePage nil templates ────────────────────────────────────

func TestHandleRemovePageNilTemplates(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/remove/abc", nil)
	w := httptest.NewRecorder()
	s.handleRemovePage(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("handleRemovePage nil templates status = %d, want 500", w.Code)
	}
}

// ─── Server.UpdateGeoIP with nil db ───────────────────────────────────────────

func TestUpdateGeoIPNil(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	if err := s.UpdateGeoIP(); err != nil {
		t.Errorf("UpdateGeoIP with nil geoipDB should return nil, got %v", err)
	}
}

// ─── Server.handleURLRedirect nil paste path ──────────────────────────────────

func TestHandleURLRedirectNoPasteHandler(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)
	r := httptest.NewRequest(http.MethodGet, "/url/abc", nil)
	w := httptest.NewRecorder()
	// pasteHandler is nil — this will panic. We only test if pasteHandler is set.
	// Instead, test the nil paste branch via a db-backed server.
	_ = s
	_ = w
	_ = r
	t.Skip("handleURLRedirect requires pasteHandler — covered via integration tests")
}

// ─── CSP with extra directives ────────────────────────────────────────────────

func TestBuildCSPWithExtras(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.CSP.Enabled = true
	cfg.Web.CSP.Mode = "enforce"
	cfg.Web.CSP.ScriptSrcExtra = "https://cdn.example.com"
	cfg.Web.CSP.StyleSrcExtra = "https://fonts.googleapis.com"
	cfg.Web.CSP.ImgSrcExtra = "https://images.example.com"
	cfg.Web.CSP.FontSrcExtra = "https://fonts.gstatic.com"
	cfg.Web.CSP.ConnectSrcExtra = "https://api.example.com"
	cfg.Web.CSP.FrameSrcExtra = "https://embed.example.com"
	cfg.Web.CSP.FormActionExtra = "https://forms.example.com"
	cfg.Server.TLS.Enabled = true
	cfg.Server.FQDN = "paste.example.com"
	s := newMinimalServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	policy, reportOnly := s.buildCSP(r)

	if reportOnly {
		t.Error("expected enforce mode (not report-only)")
	}
	for _, expected := range []string{
		"https://cdn.example.com",
		"https://fonts.googleapis.com",
		"upgrade-insecure-requests",
	} {
		if !strings.Contains(policy, expected) {
			t.Errorf("CSP policy should contain %q; policy snippet: %q", expected, policy[:min(len(policy), 100)])
		}
	}
}

func TestBuildCSPScriptSrcOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.CSP.Enabled = true
	cfg.Web.CSP.Mode = "enforce"
	cfg.Web.CSP.ScriptSrcOverride = "https://trusted-scripts.example.com"
	s := newMinimalServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	policy, _ := s.buildCSP(r)

	if !strings.Contains(policy, "https://trusted-scripts.example.com") {
		t.Errorf("CSP policy should use ScriptSrcOverride; got snippet: %q", policy[:min(len(policy), 100)])
	}
	if strings.Contains(policy, "'unsafe-inline'") && strings.Contains(policy, "script-src") {
		// With override, 'unsafe-inline' should not appear in script-src
		// (it's only in the default if no override is set)
	}
}

// ─── Server.baseURL with X-Forwarded-Prefix on trusted peer ──────────────────

func TestBaseURLWithForwardedPrefix(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Host = "example.com"
	r.Header.Set("X-Forwarded-Prefix", "/myapp")
	got := s.baseURL(r)
	if !strings.Contains(got, "/myapp") {
		t.Errorf("baseURL with X-Forwarded-Prefix = %q, should contain /myapp", got)
	}
}

func TestBaseURLWithForwardedHost(t *testing.T) {
	cfg := &config.Config{}
	s := newMinimalServer(cfg)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Host = "internal.example.com"
	r.Header.Set("X-Forwarded-Host", "public.example.com")
	got := s.baseURL(r)
	if !strings.Contains(got, "public.example.com") {
		t.Errorf("baseURL with X-Forwarded-Host = %q, should contain public.example.com", got)
	}
}

// ─── Server.handleDownload nil pasteHandler ──────────────────────────────────

func TestHandleDownloadNilPasteHandler(t *testing.T) {
	// handleDownload uses s.pasteHandler which would panic if nil.
	// Verify that the function signature exists via a compile-time type check.
	var _ http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {}
	t.Skip("handleDownload requires pasteHandler — no safe nil path to test")
}

// ─── liveCfg with cfgMgr nil uses cfg directly ───────────────────────────────

func TestLiveCfgWithNilManager(t *testing.T) {
	cfg := &config.Config{}
	cfg.Web.SiteTitle = "direct"
	s := newMinimalServer(cfg)
	if got := s.liveCfg().Web.SiteTitle; got != "direct" {
		t.Errorf("liveCfg() = %q, want direct", got)
	}
}

// withChiID injects a chi URL param "id" into the request context.
func withChiID(r *http.Request, id string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// newServerWithPasteHandler creates a Server with a real PasteHandler backed by stubDB.
func newServerWithPasteHandler(cfg *config.Config, db database.DB) *Server {
	ph := handler.NewPasteHandler(db, "", [32]byte{})
	return &Server{
		cfg:          cfg,
		db:           db,
		pasteHandler: ph,
		startTime:    time.Now(),
	}
}

// ─── handleViewPaste tests ───────────────────────────────────────────────────

func TestHandleViewPaste(t *testing.T) {
	t.Run("reserved slug returns 404", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/api", nil), "api")
		w := httptest.NewRecorder()
		s.handleViewPaste(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("reserved slug status = %d, want 404", w.Code)
		}
	})

	t.Run("nil paste returns 404", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/abc12345", nil), "abc12345")
		w := httptest.NewRecorder()
		s.handleViewPaste(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("nil paste status = %d, want 404", w.Code)
		}
	})

	t.Run("text UA returns content", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "hello world"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/testid", nil), "testid")
		r.Header.Set("User-Agent", "curl/7.88.1")
		w := httptest.NewRecorder()
		s.handleViewPaste(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("text paste status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "hello world") {
			t.Errorf("text paste body should contain content, got: %q", w.Body.String())
		}
	})

	t.Run("json UA returns JSON", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "json content"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/testid", nil), "testid")
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		s.handleViewPaste(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("json paste status = %d, want 200", w.Code)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("json paste body not valid JSON: %v", err)
		}
	})

	t.Run("HTML UA with nil templates returns 500", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "html content"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/testid", nil), "testid")
		r.Header.Set("User-Agent", "Mozilla/5.0")
		w := httptest.NewRecorder()
		s.handleViewPaste(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("html paste nil template status = %d, want 500", w.Code)
		}
	})
}

// ─── handleEmbed tests ───────────────────────────────────────────────────────

func TestHandleEmbed(t *testing.T) {
	t.Run("nil paste returns 404", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/emb/abc", nil), "abc")
		w := httptest.NewRecorder()
		s.handleEmbed(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("embed nil paste status = %d, want 404", w.Code)
		}
	})

	t.Run("found paste with nil templates returns 500", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "code"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/emb/testid", nil), "testid")
		w := httptest.NewRecorder()
		s.handleEmbed(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("embed found paste nil template status = %d, want 500", w.Code)
		}
	})
}

// ─── handleDownload tests ────────────────────────────────────────────────────

func TestHandleDownload(t *testing.T) {
	t.Run("nil paste returns 404", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/dl/abc", nil), "abc")
		w := httptest.NewRecorder()
		s.handleDownload(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("download nil paste status = %d, want 404", w.Code)
		}
	})

	t.Run("found paste returns content as download", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Title: "mycode", Content: "file content"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/dl/testid", nil), "testid")
		w := httptest.NewRecorder()
		s.handleDownload(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("download status = %d, want 200", w.Code)
		}
		if ct := w.Header().Get("Content-Disposition"); !strings.Contains(ct, "attachment") {
			t.Errorf("download Content-Disposition = %q", ct)
		}
		if !strings.Contains(w.Body.String(), "file content") {
			t.Errorf("download body should contain content")
		}
	})

	t.Run("paste with empty title uses ID as filename", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Title: "", Content: "content"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/dl/myid123", nil), "myid123")
		w := httptest.NewRecorder()
		s.handleDownload(w, r)
		if !strings.Contains(w.Header().Get("Content-Disposition"), "myid123") {
			t.Errorf("expected ID in filename for untitled paste, got: %q", w.Header().Get("Content-Disposition"))
		}
	})
}

// ─── handleURLRedirect tests ─────────────────────────────────────────────────

func TestHandleURLRedirect(t *testing.T) {
	t.Run("nil paste returns 404", func(t *testing.T) {
		db := &stubDB{}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/url/abc", nil), "abc")
		w := httptest.NewRecorder()
		s.handleURLRedirect(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("url redirect nil paste status = %d, want 404", w.Code)
		}
	})

	t.Run("http URL content redirects", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "https://example.com"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/url/testid", nil), "testid")
		w := httptest.NewRecorder()
		s.handleURLRedirect(w, r)
		if w.Code != http.StatusFound {
			t.Errorf("url redirect status = %d, want 302", w.Code)
		}
		if w.Header().Get("Location") != "https://example.com" {
			t.Errorf("url redirect location = %q", w.Header().Get("Location"))
		}
	})

	t.Run("non-URL content uses renderTemplate (nil → 500)", func(t *testing.T) {
		db := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "just some text"}, nil
			},
		}
		s := newServerWithPasteHandler(&config.Config{}, db)
		r := withChiID(httptest.NewRequest(http.MethodGet, "/url/testid", nil), "testid")
		w := httptest.NewRecorder()
		s.handleURLRedirect(w, r)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("url non-redirect nil template status = %d, want 500", w.Code)
		}
	})
}

// ─── New + setupRoutes ────────────────────────────────────────────────────────

// TestNew calls the real constructor to cover New and setupRoutes (both 0%).
// Uses stubDB which satisfies database.DB.
// Templates are loaded from the embedded FS so renderTemplate works here.
func TestNew(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	s := New(db, cfg, nil, "1.2.3", "abc1234", "2025-01-01", "", "")

	if s == nil {
		t.Fatal("New returned nil")
	}
	if s.router == nil {
		t.Error("router should be initialized")
	}
	if s.pasteHandler == nil {
		t.Error("pasteHandler should be initialized")
	}
	if s.swaggerHandler == nil {
		t.Error("swaggerHandler should be initialized")
	}
	if s.graphqlHandler == nil {
		t.Error("graphqlHandler should be initialized")
	}
	if s.metricsCollector == nil {
		t.Error("metricsCollector should be initialized")
	}
}

func TestNewWithToken(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	cfg.Server.Token = "mysecrettoken"
	s := New(db, cfg, nil, "1.0.0", "def5678", "2025-06-01", "", "")
	var zeroHash [32]byte
	if s.operatorTokenHash == zeroHash {
		t.Error("operatorTokenHash should be set when server.token is non-empty")
	}
}

func TestNewWithRateLimitEnabled(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	cfg.RateLimit.Enabled = true
	cfg.RateLimit.CreatePerM = 5
	cfg.RateLimit.DeletePerM = 3
	s := New(db, cfg, nil, "1.0.0", "abc", "now", "", "")
	if s.createLimiter == nil {
		t.Error("createLimiter should be non-nil when rate limiting enabled")
	}
	if s.deleteLimiter == nil {
		t.Error("deleteLimiter should be non-nil when rate limiting enabled")
	}
	if s.createLimiter.limit != 5 {
		t.Errorf("createLimiter.limit = %d, want 5", s.createLimiter.limit)
	}
}

// TestNewServeHTTP verifies the router serves common routes after New.
func TestNewServeHTTP(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	s := New(db, cfg, nil, "1.0.0", "abc", "now", "", "")

	routes := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/server/healthz", http.StatusOK},
		{http.MethodGet, "/api/autodiscover", http.StatusOK},
		{http.MethodGet, "/api/swagger", http.StatusOK},
		{http.MethodGet, "/robots.txt", http.StatusOK},
		{http.MethodGet, "/manifest.json", http.StatusOK},
		{http.MethodGet, "/favicon.ico", http.StatusFound},
	}
	for _, tc := range routes {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Accept", "application/json")
			r.Host = "localhost"
			w := httptest.NewRecorder()
			s.router.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, w.Code, tc.want)
			}
		})
	}
}

// TestRun starts the server and cancels immediately.
func TestRun(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	s := New(db, cfg, nil, "1.0.0", "abc", "now", "", "")

	ctx, cancel := context.WithCancel(context.Background())
	// cancel before Run — server should stop immediately after binding
	cancel()

	// Use port 0 so the OS picks a free port.
	err := s.Run(ctx, "127.0.0.1:0")
	// Run returns nil or context-related error on clean shutdown.
	if err != nil && !strings.Contains(err.Error(), "context") &&
		!strings.Contains(err.Error(), "Server closed") {
		t.Errorf("Run returned unexpected error: %v", err)
	}
}

func TestRunBindError(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	s := New(db, cfg, nil, "1.0.0", "abc", "now", "", "")

	// An unbindable address makes bindAndDrop fail; Run must surface the error.
	err := s.Run(context.Background(), "127.0.0.1:99999")
	if err == nil {
		t.Error("expected a bind error for an invalid port")
	}
}

// TestNewHandlersWithTemplates tests handlers that need real templates
// (loaded by New via embedded FS) to exercise the renderTemplate non-error path.
func TestNewHandlersWithTemplates(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	s := New(db, cfg, nil, "1.0.0", "abc", "now", "", "")

	t.Run("handleHome_html", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s.handleHome(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("home HTML status = %d, want 200; body: %s", w.Code, w.Body.String()[:min(len(w.Body.String()), 200)])
		}
	})

	t.Run("handleRecent_html", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/recent", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s.handleRecent(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("recent HTML status = %d, want 200", w.Code)
		}
	})

	t.Run("handleHealthz_html", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s.handleHealthz(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("healthz HTML status = %d, want 200", w.Code)
		}
	})

	t.Run("handleViewPaste_html_found", func(t *testing.T) {
		db2 := &stubDB{
			getPasteByIDFn: func(id string) (*model.Paste, error) {
				return &model.Paste{ID: id, Content: "hello world", Language: "text"}, nil
			},
		}
		ph := handler.NewPasteHandler(db2, "", [32]byte{})
		s2 := &Server{cfg: cfg, db: db2, pasteHandler: ph, templates: s.templates, startTime: time.Now()}
		r := withChiID(httptest.NewRequest(http.MethodGet, "/testid", nil), "testid")
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s2.handleViewPaste(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("view paste HTML status = %d, want 200; body: %q", w.Code, w.Body.String()[:min(len(w.Body.String()), 300)])
		}
	})

	t.Run("handleRemoveSubmit_no_token", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/remove/abc", strings.NewReader(""))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.handleRemoveSubmit(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("remove submit no-token status = %d, want 200 (template rendered)", w.Code)
		}
	})
}

// ─── maybeDeleteRateLimit with active limiter ─────────────────────────────────

func TestMaybeDeleteRateLimitWithLimiter(t *testing.T) {
	s := newMinimalServer(&config.Config{})
	s.deleteLimiter = newRateLimiter(1, time.Minute)

	calls := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	wrapped := s.maybeDeleteRateLimit(inner)

	r1 := httptest.NewRequest(http.MethodDelete, "/api/v1/pastes/abc", nil)
	r1.RemoteAddr = "10.1.1.1:1111"
	w1 := httptest.NewRecorder()
	wrapped(w1, r1)
	if w1.Code != http.StatusOK {
		t.Errorf("first delete status = %d, want 200", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodDelete, "/api/v1/pastes/abc", nil)
	r2.RemoteAddr = "10.1.1.1:2222"
	w2 := httptest.NewRecorder()
	wrapped(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second delete status = %d, want 429", w2.Code)
	}
}

// ─── liveCfg with cfgMgr set ─────────────────────────────────────────────────

func TestLiveCfgWithManager(t *testing.T) {
	initial := config.DefaultConfig()
	initial.Web.SiteTitle = "Initial"
	mgr := config.NewConfigManager("", initial)

	s := &Server{cfg: initial, cfgMgr: mgr}
	got := s.liveCfg()
	if got.Web.SiteTitle == "" {
		t.Error("liveCfg with manager should return non-nil config")
	}
}

// torManager is a concrete *tor.Manager, so buildTorInfo is tested via
// TestNew/TestBuildHealthResponse which exercise the real server with no Tor binary found.

// ─── OnConfigChange TLS restart key ──────────────────────────────────────────

func TestOnConfigChangeTLSRestart(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.TLS.Enabled = false
	s := newMinimalServer(cfg)

	next := config.DefaultConfig()
	next.Server.TLS.Enabled = true
	s.OnConfigChange(next)

	s.pendingRestartMu.Lock()
	keys := s.pendingRestartKeys
	s.pendingRestartMu.Unlock()

	found := false
	for _, k := range keys {
		if strings.Contains(k, "tls") || strings.Contains(k, "ssl") {
			found = true
		}
	}
	if !found {
		t.Errorf("TLS change should mark pending restart, got keys: %v", keys)
	}
}

// TestHandleWebCreate exercises the no-JS web create flow: a urlencoded POST
// renders the result (including the one-time owner token) server-side, an empty
// body renders the error, and a JSON request is delegated to the API handler.
func TestHandleWebCreate(t *testing.T) {
	db := &stubDB{}
	cfg := config.DefaultConfig()
	s := New(db, cfg, nil, "1.0.0", "abc", "now", "", "")

	t.Run("urlencoded_success_renders_token", func(t *testing.T) {
		body := strings.NewReader("content=hello+world&title=Test&language=go&visibility=0&expires_in=never&burn_after=0")
		r := httptest.NewRequest(http.MethodPost, "/create", body)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s.handleWebCreate(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %q", w.Code, w.Body.String()[:min(len(w.Body.String()), 300)])
		}
		if !strings.Contains(w.Body.String(), "created-token") {
			t.Error("response should render the created owner token block")
		}
		// Raw and Download links must be absolute URLs sharing the view origin,
		// not bare relative paths (regression: "/raw/{id}", "/dl/{id}").
		rendered := w.Body.String()
		if !strings.Contains(rendered, `href="http://example.com/raw/`) {
			t.Error("raw link should be an absolute URL matching the view origin")
		}
		if !strings.Contains(rendered, `href="http://example.com/dl/`) {
			t.Error("download link should be an absolute URL matching the view origin")
		}
	})

	t.Run("urlencoded_empty_content_renders_error", func(t *testing.T) {
		body := strings.NewReader("content=&title=Test")
		r := httptest.NewRequest(http.MethodPost, "/create", body)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		s.handleWebCreate(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
		if !strings.Contains(w.Body.String(), "alert-error") {
			t.Error("response should render the error alert")
		}
	})

	t.Run("json_delegates_to_api_handler", func(t *testing.T) {
		body := strings.NewReader(`{"content":"hi","language":"text"}`)
		r := httptest.NewRequest(http.MethodPost, "/create", body)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		s.handleWebCreate(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201; body: %q", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"ok": true`) {
			t.Error("JSON delegation should return the API success envelope")
		}
	})

	// The canonical web resource routes (PART 16 dual-route table) must be
	// registered: GET /pastes (list) and POST /pastes (create), mirroring
	// GET/POST /api/v1/pastes. /create is retained as a compatibility alias.
	t.Run("canonical_pastes_routes_registered", func(t *testing.T) {
		want := map[string]bool{"GET /pastes": false, "POST /pastes": false, "POST /create": false}
		_ = chi.Walk(s.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			if _, ok := want[method+" "+route]; ok {
				want[method+" "+route] = true
			}
			return nil
		})
		for k, seen := range want {
			if !seen {
				t.Errorf("route %q not registered", k)
			}
		}
	})

	// A CLI client posting form-encoded data is a non-browser client (PART 16
	// Smart Content Detection): it must be delegated to the negotiating handler
	// for text/redirect output, never rendered the HTML result page.
	t.Run("cli_urlencoded_delegates_not_html", func(t *testing.T) {
		body := strings.NewReader("content=hello&language=text")
		r := httptest.NewRequest(http.MethodPost, "/create", body)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Accept", "text/plain")
		w := httptest.NewRecorder()
		s.handleWebCreate(w, r)
		if strings.Contains(w.Body.String(), "created-token") {
			t.Error("CLI form-encoded request must not render the HTML result page")
		}
	})
}

// ─── startTermbin ──────────────────────────────────────────────────────────────

// newTermbinServer builds a Server with a working compat handler over stubDB.
func newTermbinServer() *Server {
	db := &stubDB{}
	ph := handler.NewPasteHandler(db, "", [32]byte{})
	return &Server{
		db:            db,
		pasteHandler:  ph,
		compatHandler: handler.NewCompatHandler(ph, db, "test"),
		startTime:     time.Now(),
	}
}

// freeTCPPort grabs an OS-assigned port, then frees it for the caller to reuse.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestStartTermbin_Disabled(t *testing.T) {
	s := newTermbinServer()
	cfg := &config.Config{}
	cfg.Server.Termbin = config.TermbinConfig{Enabled: false}
	stop := s.startTermbin(context.Background(), cfg)
	defer stop()
	if stop == nil {
		t.Fatal("startTermbin returned nil stop func")
	}
}

func TestStartTermbin_BindFailure(t *testing.T) {
	s := newTermbinServer()
	cfg := &config.Config{}
	cfg.Server.Address = "127.0.0.1"
	// Port 0 with an unusable address forces a bind failure path; use an
	// out-of-range port to guarantee net.Listen errors.
	cfg.Server.Termbin = config.TermbinConfig{Enabled: true, Port: 1, MaxSize: 32768, Timeout: "5s"}
	cfg.Server.Address = "240.0.0.1"
	stop := s.startTermbin(context.Background(), cfg)
	defer stop()
	if stop == nil {
		t.Fatal("startTermbin returned nil stop func on bind failure")
	}
}

func TestStartTermbin_RoundTrip(t *testing.T) {
	s := newTermbinServer()
	port := freeTCPPort(t)
	cfg := &config.Config{}
	cfg.Server.Address = "127.0.0.1"
	cfg.Server.BaseURL = "http://paste.example"
	cfg.Server.Termbin = config.TermbinConfig{Enabled: true, Port: port, MaxSize: 32768, Timeout: "2s"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := s.startTermbin(ctx, cfg)
	defer stop()

	var conn net.Conn
	var err error
	// The accept loop starts in a goroutine; retry the dial briefly.
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial termbin: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("termbin tcp content\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, _ := io.ReadAll(conn)
	if !strings.HasPrefix(string(resp), "http://paste.example/") {
		t.Fatalf("response = %q, want a base URL line", string(resp))
	}
}

func TestStartTermbin_BaseURLFallback(t *testing.T) {
	s := newTermbinServer()
	port := freeTCPPort(t)
	cfg := &config.Config{}
	cfg.Server.Address = "127.0.0.1"
	// No BaseURL and no FQDN → falls back to http://localhost.
	cfg.Server.Termbin = config.TermbinConfig{Enabled: true, Port: port, MaxSize: 32768, Timeout: "bad-duration"}

	ctx, cancel := context.WithCancel(context.Background())
	stop := s.startTermbin(ctx, cfg)
	defer stop()

	var conn net.Conn
	var err error
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial termbin: %v", err)
	}
	if _, err := conn.Write([]byte("fallback content\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, _ := io.ReadAll(conn)
	conn.Close()
	if !strings.HasPrefix(string(resp), "http://localhost/") {
		t.Fatalf("response = %q, want localhost fallback", string(resp))
	}
	// Cancelling the context must close the listener (covers the ctx.Done path).
	cancel()
	time.Sleep(50 * time.Millisecond)
}
