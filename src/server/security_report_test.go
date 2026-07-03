package server

import (
	"net"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.1.2.3", "192.168.0.5", "172.16.9.9",
		"169.254.1.1", "0.0.0.0", "224.0.0.1", "fc00::1", "fe80::1",
	}
	for _, s := range blocked {
		if isPublicIP(net.ParseIP(s)) {
			t.Errorf("isPublicIP(%s) = true; want false", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"}
	for _, s := range allowed {
		if !isPublicIP(net.ParseIP(s)) {
			t.Errorf("isPublicIP(%s) = false; want true", s)
		}
	}
	if isPublicIP(nil) {
		t.Error("isPublicIP(nil) = true; want false")
	}
}

func TestFetchResearcherPubKeyRejectsNonHTTPS(t *testing.T) {
	if _, err := fetchResearcherPubKey("http://keys.example.com/k.asc"); err == nil {
		t.Error("expected non-https url to be rejected")
	}
	if _, err := fetchResearcherPubKey("ftp://keys.example.com/k.asc"); err == nil {
		t.Error("expected non-https scheme to be rejected")
	}
}

func TestFetchResearcherPubKeyEmpty(t *testing.T) {
	if _, err := fetchResearcherPubKey("   "); err == nil {
		t.Error("expected empty key to be rejected")
	}
}

func TestFetchResearcherPubKeyRejectsInvalidPastedBlock(t *testing.T) {
	bogus := pgpPublicKeyHeader + "\nnot-a-real-key\n-----END PGP PUBLIC KEY BLOCK-----"
	if _, err := fetchResearcherPubKey(bogus); err == nil {
		t.Error("expected structurally invalid pasted key to be rejected")
	}
}

func TestSanitizeComponent(t *testing.T) {
	cases := map[string]string{
		"auth":                 "auth",
		"  API  ":              "api",
		"Login Form":           "login-form",
		"api/v1/pastes":        "api/v1/pastes",
		"drop table users; --": "drop-table-users",
		"<script>":             "script",
		"":                     "unspecified",
		"---":                  "unspecified",
		"e-mail_service.v2":    "e-mail_service.v2",
	}
	for in, want := range cases {
		if got := sanitizeComponent(in); got != want {
			t.Errorf("sanitizeComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeComponentCaps64(t *testing.T) {
	got := sanitizeComponent(strings.Repeat("a", 200))
	if len(got) > 64 {
		t.Errorf("sanitizeComponent length %d exceeds 64", len(got))
	}
}

func TestRandomHexLengthAndUniqueness(t *testing.T) {
	for _, n := range []int{1, 16, 32, 33} {
		a, err := randomHex(n)
		if err != nil {
			t.Fatalf("randomHex(%d): %v", n, err)
		}
		if len(a) != n {
			t.Errorf("randomHex(%d) length = %d", n, len(a))
		}
		for _, r := range a {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
				t.Errorf("randomHex(%d) has non-hex char %q", n, r)
			}
		}
	}
	a, _ := randomHex(32)
	b, _ := randomHex(32)
	if a == b {
		t.Error("two randomHex(32) calls collided")
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/server/contact", nil)
	r.RemoteAddr = "203.0.113.7:54321"
	if got := clientIP(r); got != "203.0.113.7" {
		t.Errorf("clientIP host = %q, want 203.0.113.7", got)
	}
	r.RemoteAddr = "not-a-host-port"
	if got := clientIP(r); got != "not-a-host-port" {
		t.Errorf("clientIP fallback = %q", got)
	}
}

func TestBuildReportBody(t *testing.T) {
	body := buildReportBody(reportFields{
		trackingID:     "sec_deadbeef",
		timestamp:      "2026-07-02T12:00:00Z",
		name:           "Jane Researcher",
		email:          "jane@example.com",
		gpg:            "0xABC",
		component:      "auth",
		endpoint:       "/api/v1/pastes",
		severity:       "High",
		summary:        "sample summary",
		steps:          "step 1",
		impact:         "account takeover",
		suggestedFix:   "validate token",
		cveRequested:   true,
		disclosureDays: 45,
		creditPref:     "name",
		creditName:     "Jane",
		userAgent:      "curl/8",
		remoteIP:       "203.0.113.7",
		appVersion:     "1.2.3",
		commitHash:     "abc123",
	})
	for _, want := range []string{
		"Tracking ID: sec_deadbeef",
		"Reporter: Jane Researcher <jane@example.com>",
		"Reporter PGP: 0xABC",
		"Credit preference: name (Jane)",
		"Severity (self-assessed): High",
		"Affected component: auth",
		"Affected endpoint: /api/v1/pastes",
		"CVE requested: yes",
		"Requested disclosure window: 45 days",
		"Summary:\nsample summary",
		"Steps to reproduce:\nstep 1",
		"Impact:\naccount takeover",
		"Suggested fix:\nvalidate token",
		"app_version: 1.2.3",
		"commit_hash: abc123",
		"request_ip: 203.0.113.7",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("report body missing %q\n---\n%s", want, body)
		}
	}
}

func TestBuildReportBodyOmitsOptional(t *testing.T) {
	body := buildReportBody(reportFields{
		trackingID: "sec_x", timestamp: "t", name: "n", email: "e",
		component: "api", severity: "Low", summary: "s", steps: "st",
		impact: "i", creditPref: "anonymous", disclosureDays: 90,
	})
	if strings.Contains(body, "Reporter PGP:") {
		t.Error("empty gpg should not render a PGP line")
	}
	if strings.Contains(body, "Suggested fix:") {
		t.Error("empty suggested fix should not render a section")
	}
	if strings.Contains(body, "Affected endpoint:") {
		t.Error("empty endpoint should not render a line")
	}
	if !strings.Contains(body, "CVE requested: no") {
		t.Error("cveRequested=false should render 'no'")
	}
}
