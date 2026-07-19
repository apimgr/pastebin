package config

import (
	"net"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// devOnlyTLDs are hostnames/suffixes that are only valid in development mode
// (PART 12, "FQDN Validation Rules"). They never resolve on the public
// internet and must never be accepted for production DOMAIN/HOSTNAME values
// or Let's Encrypt certificate requests.
var devOnlyTLDs = map[string]bool{
	"localhost": true, "test": true, "example": true, "invalid": true,
	"local": true, "lan": true, "internal": true, "home": true,
	"localdomain": true, "home.arpa": true, "intranet": true,
	"corp": true, "private": true,
}

// IsValidHost reports whether host is an acceptable FQDN for the given mode
// (PART 12, "FQDN Validation Rules"). In production, only a publicly
// resolvable ICANN eTLD+1 (or a subdomain of one) is accepted — no IP
// addresses, no bare "localhost", no dev-only TLDs. In development mode,
// "localhost", the static dev-only TLDs, and the dynamic project-specific
// TLD ("{projectName}", e.g. "app.jokes") are additionally accepted.
// Overlay-network hosts (.onion/.i2p/.exit) are always valid — they are
// app-managed and not set via the DOMAIN environment variable.
func IsValidHost(host string, devMode bool, projectName string) bool {
	lower := strings.ToLower(strings.TrimSpace(host))

	// Reject empty
	if lower == "" {
		return false
	}

	// Reject IP addresses always
	if net.ParseIP(lower) != nil {
		return false
	}

	// Handle localhost
	if lower == "localhost" {
		return devMode
	}

	// Must contain at least one dot
	if !strings.Contains(lower, ".") {
		return false
	}

	// Overlay network TLDs - valid but app-managed (not set via DOMAIN)
	// These are checked here for internal validation, not for DOMAIN env var
	if strings.HasSuffix(lower, ".onion") ||
		strings.HasSuffix(lower, ".i2p") ||
		strings.HasSuffix(lower, ".exit") {
		return true
	}

	// Check dynamic project-specific TLD (e.g., app.jokes, dev.quotes, quotes, jokes, {project_name})
	if projectName != "" && strings.HasSuffix(lower, "."+strings.ToLower(projectName)) {
		// Project TLDs only valid in dev mode
		return devMode
	}

	// Get the public suffix (TLD or eTLD like co.uk)
	suffix, icann := publicsuffix.PublicSuffix(lower)

	// Check if it's a dev-only TLD
	if devOnlyTLDs[suffix] {
		// Dev TLDs only valid in dev mode
		return devMode
	}

	// In production, require valid ICANN TLD
	if !devMode && !icann {
		return false
	}

	// Verify we have at least eTLD+1 (not just the suffix itself)
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(lower)
	if err != nil {
		return false
	}

	// Host must be at least eTLD+1 (e.g., "domain.co.uk" not just "co.uk")
	return len(etldPlusOne) > 0
}

// IsValidSSLHost reports whether host is acceptable for a Let's Encrypt
// certificate request (PART 12, "SSL/Let's Encrypt FQDN Requirements").
// SSL always requires production-valid validation regardless of the running
// mode — dev-only TLDs and project TLDs are never eligible for a publicly
// trusted certificate. ".onion" hosts cannot use Let's Encrypt at all (they
// are not publicly resolvable; Tor already provides end-to-end encryption).
func IsValidSSLHost(host string) bool {
	lower := strings.ToLower(host)

	// .onion addresses cannot use Let's Encrypt (not publicly resolvable)
	// Tor provides end-to-end encryption, so SSL is optional for .onion
	if strings.HasSuffix(lower, ".onion") {
		return false
	}

	// SSL always requires production-valid host (devMode=false);
	// projectName is irrelevant here since project TLDs are dev-only
	return IsValidHost(host, false, "")
}
