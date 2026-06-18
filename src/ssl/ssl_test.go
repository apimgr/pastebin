package ssl_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/pastebin/src/ssl"
)

// ─── ParseChallenge ───────────────────────────────────────────────────────────

func TestParseChallenge(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// http-01 variants
		{"http-01", "http-01"},
		{"http01", "http-01"},
		{"http", "http-01"},
		{"HTTP-01", "http-01"},
		{"  HTTP01  ", "http-01"},
		// tls-alpn-01 variants
		{"tls-alpn-01", "tls-alpn-01"},
		{"tlsalpn01", "tls-alpn-01"},
		{"tls-alpn", "tls-alpn-01"},
		{"tls", "tls-alpn-01"},
		{"TLS-ALPN-01", "tls-alpn-01"},
		// dns-01 variants
		{"dns-01", "dns-01"},
		{"dns01", "dns-01"},
		{"dns", "dns-01"},
		{"DNS-01", "dns-01"},
		// unknown defaults to http-01
		{"", "http-01"},
		{"smtp", "http-01"},
		{"unknown-challenge", "http-01"},
	}
	for _, tc := range cases {
		got := ssl.ParseChallenge(tc.input)
		if got != tc.want {
			t.Errorf("ParseChallenge(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

// ─── NewManager ───────────────────────────────────────────────────────────────

func TestNewManager_NotNil(t *testing.T) {
	m := ssl.NewManager(ssl.Config{})
	if m == nil {
		t.Error("NewManager returned nil")
	}
}

// ─── GetTLSConfig — disabled ──────────────────────────────────────────────────

func TestGetTLSConfig_Disabled_ReturnsNil(t *testing.T) {
	m := ssl.NewManager(ssl.Config{Enabled: false})
	cfg, err := m.GetTLSConfig([]string{"example.com"})
	if err != nil {
		t.Errorf("GetTLSConfig (disabled) error: %v", err)
	}
	if cfg != nil {
		t.Error("GetTLSConfig (disabled) should return nil TLS config")
	}
}

// ─── GetHTTPHandler ───────────────────────────────────────────────────────────

func TestGetHTTPHandler_NoCertManager_ReturnsFallback(t *testing.T) {
	m := ssl.NewManager(ssl.Config{})
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fallback"))
	})

	handler := m.GetHTTPHandler(fallback)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GetHTTPHandler fallback: status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "fallback") {
		t.Errorf("GetHTTPHandler fallback: body %q; want 'fallback'", rec.Body.String())
	}
}

// ─── ChallengeServer ─────────────────────────────────────────────────────────

func TestNewChallengeServer_NotNil(t *testing.T) {
	cs := ssl.NewChallengeServer()
	if cs == nil {
		t.Error("NewChallengeServer returned nil")
	}
}

func TestChallengeServer_SetAndServe(t *testing.T) {
	cs := ssl.NewChallengeServer()
	cs.SetToken("abc123", "abc123.XXXXXXXXXXXXXXXX")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/abc123", nil)
	rec := httptest.NewRecorder()
	consumed := cs.ServeHTTP(rec, req)

	if !consumed {
		t.Error("ServeHTTP should return true for challenge path")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("ServeHTTP token found: status = %d; want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc123.XXXXXXXXXXXXXXXX") {
		t.Errorf("ServeHTTP: body %q; want auth value", rec.Body.String())
	}
}

func TestChallengeServer_TokenNotFound_Returns404(t *testing.T) {
	cs := ssl.NewChallengeServer()

	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/missing", nil)
	rec := httptest.NewRecorder()
	consumed := cs.ServeHTTP(rec, req)

	if !consumed {
		t.Error("ServeHTTP should return true even when token not found")
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("ServeHTTP missing token: status = %d; want 404", rec.Code)
	}
}

func TestChallengeServer_NonChallengePath_NotConsumed(t *testing.T) {
	cs := ssl.NewChallengeServer()

	req := httptest.NewRequest(http.MethodGet, "/other/path", nil)
	rec := httptest.NewRecorder()
	consumed := cs.ServeHTTP(rec, req)

	if consumed {
		t.Error("ServeHTTP should return false for non-challenge path")
	}
}

func TestChallengeServer_ClearToken(t *testing.T) {
	cs := ssl.NewChallengeServer()
	cs.SetToken("tok1", "auth1")
	cs.ClearToken("tok1")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok1", nil)
	rec := httptest.NewRecorder()
	cs.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("after ClearToken: status = %d; want 404", rec.Code)
	}
}

// ─── GetTLSConfig — enabled, no certs, LE disabled ───────────────────────────

func TestGetTLSConfig_EnabledNoCertsNoLE_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	m := ssl.NewManager(ssl.Config{
		Enabled: true,
		FQDN:    "example.com",
		CertDir: dir,
		LetsEncrypt: ssl.LetsEncryptConfig{
			Enabled: false,
		},
	})
	_, err := m.GetTLSConfig([]string{"example.com"})
	if err == nil {
		t.Error("GetTLSConfig with no certs and LE disabled should return an error")
	}
}

// ─── ChallengeServer — multi-token ───────────────────────────────────────────

func TestChallengeServer_MultipleTokens(t *testing.T) {
	cs := ssl.NewChallengeServer()
	cs.SetToken("t1", "auth1")
	cs.SetToken("t2", "auth2")

	for _, tc := range []struct{ token, auth string }{{"t1", "auth1"}, {"t2", "auth2"}} {
		req := httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/"+tc.token, nil)
		rec := httptest.NewRecorder()
		cs.ServeHTTP(rec, req)
		if !strings.Contains(rec.Body.String(), tc.auth) {
			t.Errorf("token %s: body %q; want %q", tc.token, rec.Body.String(), tc.auth)
		}
	}
}
