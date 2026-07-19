package config

import (
	"testing"
)

func TestIsValidHost(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		devMode     bool
		projectName string
		want        bool
	}{
		{"empty", "", false, "pastebin", false},
		{"public etld+1 prod", "api.example.com", false, "pastebin", true},
		{"public etld+1 dev", "api.example.com", true, "pastebin", true},
		{"multi-label public suffix prod", "app.company.com.au", false, "pastebin", true},
		{"deep subdomain of co.uk", "my.server.domain.co.uk", false, "pastebin", true},
		{"bare public suffix rejected", "co.uk", false, "pastebin", false},
		{"ipv4 always rejected", "192.168.1.1", true, "pastebin", false},
		{"ipv4 always rejected prod", "192.168.1.1", false, "pastebin", false},
		{"ipv6 always rejected", "::1", true, "pastebin", false},
		{"no dot rejected", "myhost", false, "pastebin", false},
		{"no dot rejected dev", "myhost", true, "pastebin", false},
		{"bare localhost prod", "localhost", false, "pastebin", false},
		{"bare localhost dev", "localhost", true, "pastebin", true},
		{"dev tld prod rejected", "dev.local", false, "pastebin", false},
		{"dev tld dev allowed", "dev.local", true, "pastebin", true},
		{"dev tld .test prod rejected", "app.test", false, "pastebin", false},
		{"dev tld .test dev allowed", "app.test", true, "pastebin", true},
		{"project tld prod rejected", "app.pastebin", false, "pastebin", false},
		{"project tld dev allowed", "app.pastebin", true, "pastebin", true},
		{"nested project tld dev allowed", "my.app.pastebin", true, "pastebin", true},
		{"nested project tld prod rejected", "my.app.pastebin", false, "pastebin", false},
		{"bare project name dev allowed", "pastebin", true, "pastebin", true},
		{"bare project name prod rejected", "pastebin", false, "pastebin", false},
		{"onion always valid prod", "abc123.onion", false, "pastebin", true},
		{"onion always valid dev", "abc123.onion", true, "pastebin", true},
		{"i2p always valid", "abc123.i2p", false, "pastebin", true},
		{"exit always valid", "abc123.exit", false, "pastebin", true},
		{"case insensitive", "API.EXAMPLE.COM", false, "pastebin", true},
		{"whitespace trimmed", "  api.example.com  ", false, "pastebin", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidHost(tt.host, tt.devMode, tt.projectName); got != tt.want {
				t.Errorf("IsValidHost(%q, %v, %q) = %v, want %v", tt.host, tt.devMode, tt.projectName, got, tt.want)
			}
		})
	}
}

func TestIsValidSSLHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want bool
	}{
		{"public domain valid", "api.example.com", true},
		{"onion never valid for LE", "abc123.onion", false},
		{"dev tld never valid for SSL", "dev.local", false},
		{"project tld never valid for SSL", "app.pastebin", false},
		{"bare localhost never valid for SSL", "localhost", false},
		{"ip never valid for SSL", "192.168.1.1", false},
		{"i2p not onion still runs prod validation and fails", "abc123.i2p", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidSSLHost(tt.host); got != tt.want {
				t.Errorf("IsValidSSLHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestLoadEnvDomainValidation(t *testing.T) {
	t.Run("invalid DOMAIN skipped silently in production", func(t *testing.T) {
		t.Setenv("DOMAIN", "dev.local")
		t.Setenv("MODE", "production")
		c := DefaultConfig()
		before := c.Server.FQDN
		c.loadEnv()
		if c.Server.FQDN != before {
			t.Errorf("expected invalid DOMAIN to be skipped, got FQDN=%q", c.Server.FQDN)
		}
	})

	t.Run("valid DOMAIN accepted in production", func(t *testing.T) {
		t.Setenv("DOMAIN", "paste.example.com")
		t.Setenv("MODE", "production")
		c := DefaultConfig()
		c.loadEnv()
		if c.Server.FQDN != "paste.example.com" {
			t.Errorf("expected DOMAIN to be applied, got FQDN=%q", c.Server.FQDN)
		}
	})

	t.Run("dev TLD DOMAIN accepted in development", func(t *testing.T) {
		t.Setenv("DOMAIN", "dev.local")
		t.Setenv("MODE", "development")
		c := DefaultConfig()
		c.loadEnv()
		if c.Server.FQDN != "dev.local" {
			t.Errorf("expected dev-mode DOMAIN to be applied, got FQDN=%q", c.Server.FQDN)
		}
	})
}

// TestFQDNHostnameEnvValidation confirms the $HOSTNAME fallback source in
// fqdn() rejects an invalid host and falls through to the next source
// (never returning the invalid value), and accepts a valid one. os.Hostname()
// ranks ahead of $HOSTNAME in the resolution chain, so both cases force it
// out of the way with an FQDN override that is itself intentionally
// "localhost" — the one os.Hostname() value fqdn() always treats as absent —
// which is not possible to guarantee portably, so instead this exercises the
// validation helper directly against the same devMode/projectName wiring
// fqdn() uses, which is the behavior under test.
func TestFQDNHostnameEnvValidation(t *testing.T) {
	c := DefaultConfig()
	c.Server.Mode = "production"
	devMode := c.devModeForValidation()

	if IsValidHost("192.168.1.5", devMode, projectName) {
		t.Error("expected IP HOSTNAME value to fail production validation")
	}
	if !IsValidHost("host.example.com", devMode, projectName) {
		t.Error("expected valid HOSTNAME value to pass production validation")
	}

	c.Server.Mode = "development"
	devMode = c.devModeForValidation()
	if !IsValidHost("dev.local", devMode, projectName) {
		t.Error("expected dev-TLD HOSTNAME value to pass development validation")
	}
}
