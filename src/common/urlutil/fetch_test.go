package urlutil

import "testing"

func TestValidateRemoteURLSchemeRejection(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	err := ValidateRemoteURL("http://8.8.8.8/logo.png", cfg)
	if err == nil {
		t.Fatal("expected error for non-https scheme, got nil")
	}
}

func TestValidateRemoteURLInvalidURL(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	err := ValidateRemoteURL("://not-a-url", cfg)
	if err == nil {
		t.Fatal("expected error for malformed URL, got nil")
	}
}

func TestValidateRemoteURLPublicIPAccepted(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	if err := ValidateRemoteURL("https://8.8.8.8/logo.png", cfg); err != nil {
		t.Fatalf("expected public IP literal to be accepted, got: %v", err)
	}
}

func TestValidateRemoteURLPrivateIPRejected(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	cases := []string{
		"https://127.0.0.1/logo.png",
		"https://10.0.0.5/logo.png",
		"https://172.16.0.5/logo.png",
		"https://192.168.1.5/logo.png",
		"https://169.254.1.1/logo.png",
		"https://[::1]/logo.png",
		"https://[fe80::1]/logo.png",
	}
	for _, rawURL := range cases {
		if err := ValidateRemoteURL(rawURL, cfg); err == nil {
			t.Errorf("expected private/local IP %q to be rejected, got nil error", rawURL)
		}
	}
}

func TestValidateRemoteURLLocalhostHostnameRejected(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	cases := []string{
		"https://localhost/logo.png",
		"https://printer.local/logo.png",
		"https://router.internal/logo.png",
	}
	for _, rawURL := range cases {
		if err := ValidateRemoteURL(rawURL, cfg); err == nil {
			t.Errorf("expected internal hostname %q to be rejected, got nil error", rawURL)
		}
	}
}

func TestValidateRemoteURLMissingHost(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	if err := ValidateRemoteURL("https:///logo.png", cfg); err == nil {
		t.Fatal("expected error for missing host, got nil")
	}
}

func TestValidateNotPrivateIPUnspecified(t *testing.T) {
	if err := validateNotPrivateIP("0.0.0.0"); err == nil {
		t.Fatal("expected unspecified address to be rejected, got nil")
	}
}

func TestDefaultFetchRemoteImageConfig(t *testing.T) {
	cfg := DefaultFetchRemoteImageConfig()
	if cfg.MaxSize != 10*1024*1024 {
		t.Errorf("expected default MaxSize 10MB, got %d", cfg.MaxSize)
	}
	if len(cfg.AllowedSchemes) != 1 || cfg.AllowedSchemes[0] != "https" {
		t.Errorf("expected https-only default scheme allowlist, got %v", cfg.AllowedSchemes)
	}
	if len(cfg.AllowedTypes) == 0 {
		t.Error("expected non-empty default allowed content types")
	}
}
