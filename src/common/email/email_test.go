package email_test

import (
	"testing"

	"github.com/apimgr/pastebin/src/common/email"
	"github.com/apimgr/pastebin/src/config"
)

// ─── New + Enabled ────────────────────────────────────────────────────────────

func TestNew_ReturnsMailer(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: true, SMTP: config.SMTPConfig{Host: "smtp.example.com", Port: 587}}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")
	if m == nil {
		t.Fatal("New returned nil")
	}
}

func TestEnabled_True(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: true, SMTP: config.SMTPConfig{Host: "smtp.example.com", Port: 587}}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")
	if !m.Enabled() {
		t.Error("expected Enabled()=true when SMTP host is set and enabled=true")
	}
}

func TestEnabled_FalseWhenDisabled(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: false, SMTP: config.SMTPConfig{Host: "smtp.example.com"}}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")
	if m.Enabled() {
		t.Error("expected Enabled()=false when Enabled=false")
	}
}

func TestEnabled_FalseWhenNoHost(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: true, SMTP: config.SMTPConfig{Host: ""}}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")
	if m.Enabled() {
		t.Error("expected Enabled()=false when SMTP host is empty")
	}
}

// ─── Send when disabled ───────────────────────────────────────────────────────

func TestSend_Disabled_ReturnsNil(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: false}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")

	if err := m.Send("user@example.com", "paste_created", map[string]string{
		"paste_id": "abc123",
	}); err != nil {
		t.Errorf("Send when disabled: expected nil, got %v", err)
	}
}

func TestSend_NoHost_ReturnsNil(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: true, SMTP: config.SMTPConfig{Host: ""}}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")

	if err := m.Send("user@example.com", "paste_created", nil); err != nil {
		t.Errorf("Send with no SMTP host: expected nil, got %v", err)
	}
}

// ─── TestSMTP when no host ────────────────────────────────────────────────────

func TestTestSMTP_NoHost_ReturnsError(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: false, SMTP: config.SMTPConfig{Host: ""}}
	m := email.New(cfg, "pastebin", "https://example.com", "example.com")

	if err := m.TestSMTP(); err == nil {
		t.Error("TestSMTP with no host: expected error, got nil")
	}
}
