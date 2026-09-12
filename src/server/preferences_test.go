package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// TestParseAppPref verifies the pastebin_pref_{key} name/value allowlist
// (AI.md 23482/23496) accepts well-formed pairs and rejects everything else.
func TestParseAppPref(t *testing.T) {
	cases := []struct {
		name    string
		pname   string
		pvalue  string
		wantKey string
		wantOK  bool
	}{
		{"valid", "pastebin_pref_view_mode", "compact", "view_mode", true},
		{"wrong prefix", "theme", "dark", "", false},
		{"unrelated cookie", "pastebin_build", "1.0.0-abc123", "", false},
		{"empty key", "pastebin_pref_", "x", "", false},
		{"uppercase key rejected", "pastebin_pref_ViewMode", "x", "", false},
		{"key starting with digit rejected", "pastebin_pref_1abc", "x", "", false},
		{"empty value rejected", "pastebin_pref_view_mode", "", "", false},
		{"value with space rejected", "pastebin_pref_view_mode", "a b", "", false},
		{"value with semicolon rejected", "pastebin_pref_view_mode", "a;b", "", false},
		{"value too long rejected", "pastebin_pref_view_mode", string(make([]byte, 65)), "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, value, ok := parseAppPref(tc.pname, tc.pvalue)
			if ok != tc.wantOK {
				t.Fatalf("parseAppPref(%q, %q) ok = %v, want %v", tc.pname, tc.pvalue, ok, tc.wantOK)
			}
			if ok && (key != tc.wantKey || value != tc.pvalue) {
				t.Fatalf("parseAppPref(%q, %q) = (%q, %q), want (%q, %q)", tc.pname, tc.pvalue, key, value, tc.wantKey, tc.pvalue)
			}
		})
	}
}

// TestAppPreferencesFromRequest verifies only pastebin_pref_* cookies are
// surfaced, non-preference cookies (theme, cookie_consent, pastebin_build)
// are excluded, and malformed values are dropped rather than applied.
func TestAppPreferencesFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/server/preferences", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})
	req.AddCookie(&http.Cookie{Name: "cookie_consent", Value: `{"essential":true}`})
	req.AddCookie(&http.Cookie{Name: "ccpa_opt_out", Value: "true"})
	req.AddCookie(&http.Cookie{Name: "pastebin_build", Value: "1.0.0-abc123"})
	req.AddCookie(&http.Cookie{Name: "pastebin_pref_view_mode", Value: "compact"})
	req.AddCookie(&http.Cookie{Name: "pastebin_pref_bad value", Value: "x"})

	got := appPreferencesFromRequest(req)
	if len(got) != 1 {
		t.Fatalf("appPreferencesFromRequest() = %v, want exactly 1 entry", got)
	}
	if got["view_mode"] != "compact" {
		t.Fatalf("appPreferencesFromRequest()[\"view_mode\"] = %q, want %q", got["view_mode"], "compact")
	}
}

// TestExtractAppPrefs mirrors TestAppPreferencesFromRequest but for the
// import path's url.Values source (query params and decoded `code`).
func TestExtractAppPrefs(t *testing.T) {
	values := url.Values{
		"theme":                   {"dark"},
		"lang":                    {"fr"},
		"code":                    {"abc"},
		"pastebin_pref_view_mode": {"compact"},
		"pastebin_pref_invalid!":  {"x"},
	}

	got := extractAppPrefs(values)
	if len(got) != 1 {
		t.Fatalf("extractAppPrefs() = %v, want exactly 1 entry", got)
	}
	if got["view_mode"] != "compact" {
		t.Fatalf("extractAppPrefs()[\"view_mode\"] = %q, want %q", got["view_mode"], "compact")
	}
}

// TestPreferencesExportQuery verifies theme/lang always round-trip and every
// app-specific pastebin_pref_* entry is appended in stable (sorted) order
// (AI.md 23502/23504).
func TestPreferencesExportQuery(t *testing.T) {
	got := preferencesExportQuery("dark", "fr", nil)
	want := "theme=dark&lang=fr"
	if got != want {
		t.Fatalf("preferencesExportQuery(nil) = %q, want %q", got, want)
	}

	got = preferencesExportQuery("dark", "fr", map[string]string{
		"view_mode": "compact",
		"per_page":  "50",
	})
	want = "theme=dark&lang=fr&pastebin_pref_per_page=50&pastebin_pref_view_mode=compact"
	if got != want {
		t.Fatalf("preferencesExportQuery(extra) = %q, want %q", got, want)
	}
}
