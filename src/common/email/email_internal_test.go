package email

import (
	"os"
	"strings"
	"testing"

	"github.com/apimgr/pastebin/src/config"
)

// ─── parseTemplate ────────────────────────────────────────────────────────────

func TestParseTemplate_Valid(t *testing.T) {
	raw := "Subject: Hello World\n---\nThis is the body.\nSecond line."
	subject, body, ok := parseTemplate(raw)
	if !ok {
		t.Fatal("parseTemplate: expected ok")
	}
	if subject != "Hello World" {
		t.Errorf("subject = %q; want %q", subject, "Hello World")
	}
	if !strings.Contains(body, "This is the body.") {
		t.Errorf("body does not contain expected text: %q", body)
	}
}

func TestParseTemplate_MissingSubjectLine(t *testing.T) {
	raw := "From: someone@example.com\n---\nbody"
	_, _, ok := parseTemplate(raw)
	if ok {
		t.Error("expected ok=false for missing Subject: prefix")
	}
}

func TestParseTemplate_MissingSeparator(t *testing.T) {
	raw := "Subject: Hi\nNo separator here\nbody"
	_, _, ok := parseTemplate(raw)
	if ok {
		t.Error("expected ok=false when no --- separator")
	}
}

func TestParseTemplate_TooFewLines(t *testing.T) {
	raw := "Subject: Hi"
	_, _, ok := parseTemplate(raw)
	if ok {
		t.Error("expected ok=false for too few lines")
	}
}

func TestParseTemplate_SubjectTrimmed(t *testing.T) {
	raw := "Subject:   Spaced Subject   \n---\nbody"
	subject, _, ok := parseTemplate(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if subject != "Spaced Subject" {
		t.Errorf("subject not trimmed: %q", subject)
	}
}

// ─── buildMessage ─────────────────────────────────────────────────────────────

func TestBuildMessage_Basic(t *testing.T) {
	msg := buildMessage("Sender", "from@example.com", "to@example.com", "Test Subject", "Hello body", "")
	s := string(msg)

	checks := []string{
		"From: Sender <from@example.com>",
		"To: to@example.com",
		"Subject: Test Subject",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Hello body",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Errorf("buildMessage: missing %q in output:\n%s", want, s)
		}
	}
}

func TestBuildMessage_WithReplyTo(t *testing.T) {
	msg := buildMessage("Name", "from@example.com", "to@example.com", "Subj", "Body", "reply@example.com")
	s := string(msg)
	if !strings.Contains(s, "Reply-To: reply@example.com") {
		t.Errorf("expected Reply-To header, got:\n%s", s)
	}
}

func TestBuildMessage_NoReplyTo(t *testing.T) {
	msg := buildMessage("Name", "from@example.com", "to@example.com", "Subj", "Body", "")
	if strings.Contains(string(msg), "Reply-To:") {
		t.Error("unexpected Reply-To header when replyTo is empty")
	}
}

// ─── renderTemplate ───────────────────────────────────────────────────────────

func TestRenderTemplate_EmbeddedTest(t *testing.T) {
	cfg := &config.EmailConfig{Enabled: true, SMTP: config.SMTPConfig{Host: "smtp.example.com"}}
	m := &Mailer{
		cfg:     cfg,
		appName: "TestApp",
		appURL:  "https://test.example.com",
		fqdn:    "test.example.com",
	}

	body, err := m.renderTemplate("test", map[string]string{"extra_key": "extra_value"})
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if body == "" {
		t.Error("expected non-empty rendered template")
	}
	// Global variables should be substituted.
	if strings.Contains(body, "{app_name}") {
		t.Error("renderTemplate: {app_name} not substituted")
	}
	if !strings.Contains(body, "TestApp") {
		t.Errorf("renderTemplate: expected 'TestApp' in output, got:\n%s", body)
	}
}

func TestRenderTemplate_NonexistentTemplate(t *testing.T) {
	cfg := &config.EmailConfig{}
	m := &Mailer{cfg: cfg, appName: "App", appURL: "https://a.com", fqdn: "a.com"}

	_, err := m.renderTemplate("nonexistent_template_xyz", nil)
	if err == nil {
		t.Error("expected error for nonexistent template, got nil")
	}
}

func TestRenderTemplate_CustomDir(t *testing.T) {
	dir := t.TempDir()
	// Write a custom template that overrides the embedded default.
	content := "Subject: Custom Subject\n---\nCustom body for {app_name}"
	if err := os.WriteFile(dir+"/custom.txt", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.EmailConfig{TemplateDir: dir}
	m := &Mailer{cfg: cfg, appName: "MyApp", appURL: "https://b.com", fqdn: "b.com"}

	body, err := m.renderTemplate("custom", nil)
	if err != nil {
		t.Fatalf("renderTemplate custom: %v", err)
	}
	if !strings.Contains(body, "MyApp") {
		t.Errorf("expected 'MyApp' in custom template output, got:\n%s", body)
	}
}
