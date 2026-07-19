package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/apimgr/pastebin/src/config"
)

// LetsEncryptStagingURL is the ACME directory URL for the Let's Encrypt staging CA.
const LetsEncryptStagingURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

// Config holds SSL/TLS configuration.
type Config struct {
	Enabled bool
	// CertDir is the {config_dir}/ssl/ base directory.
	CertDir string
	// FQDN is the primary domain.
	FQDN        string
	LetsEncrypt LetsEncryptConfig
}

// LetsEncryptConfig holds Let's Encrypt ACME settings.
type LetsEncryptConfig struct {
	Enabled bool
	Email   string
	// Challenge is the ACME challenge type: http-01, tls-alpn-01, or dns-01.
	Challenge       string
	DNSProviderType string
	// DNSCredentials holds decrypted provider credentials.
	DNSCredentials map[string]string
	// Staging selects the LE staging CA.
	Staging bool
}

// Manager handles SSL/TLS certificates.
type Manager struct {
	config      Config
	certManager *autocert.Manager
	mu          sync.RWMutex
}

// NewManager creates a new SSL manager.
func NewManager(cfg Config) *Manager {
	return &Manager{config: cfg}
}

// GetTLSConfig returns a *tls.Config for the given domains.
// Certificate lookup follows the PART 15 priority order:
//  1. /etc/letsencrypt/live/domain/ (system certbot, literal "domain" dir)
//  2. /etc/letsencrypt/live/{fqdn}/ (system certbot, FQDN-named dir)
//  3. {cert_dir}/letsencrypt/{fqdn}/ (app-managed LE certs)
//  4. {cert_dir}/local/{fqdn}/ (user-provided / self-signed)
//
// If no cert is found and Let's Encrypt is enabled, a new cert is requested
// via autocert and saved to {cert_dir}/letsencrypt/{fqdn}/.
func (m *Manager) GetTLSConfig(domains []string) (*tls.Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.config.Enabled {
		return nil, nil
	}

	fqdn := m.config.FQDN
	if fqdn == "" && len(domains) > 0 {
		fqdn = domains[0]
	}

	// Priority 1: /etc/letsencrypt/live/domain/ (literal "domain" directory).
	if cert, key := findCertPair("/etc/letsencrypt/live/domain"); cert != "" {
		if err := validateCertFile(cert, fqdn); err == nil {
			log.Printf("ssl: using system cert /etc/letsencrypt/live/domain/")
			return tlsConfigFromFiles(cert, key)
		}
	}

	// Priority 2: /etc/letsencrypt/live/{fqdn}/.
	if fqdn != "" {
		dir := filepath.Join("/etc/letsencrypt/live", fqdn)
		if cert, key := findCertPair(dir); cert != "" {
			if err := validateCertFile(cert, fqdn); err == nil {
				log.Printf("ssl: using system cert %s/", dir)
				return tlsConfigFromFiles(cert, key)
			}
		}
	}

	// Priority 3: {cert_dir}/letsencrypt/{fqdn}/ (app-managed).
	if m.config.CertDir != "" && fqdn != "" {
		dir := filepath.Join(m.config.CertDir, "letsencrypt", fqdn)
		if cert, key := findCertPair(dir); cert != "" {
			if err := validateCertFile(cert, fqdn); err == nil {
				log.Printf("ssl: using app-managed cert %s/", dir)
				return tlsConfigFromFiles(cert, key)
			}
			log.Printf("ssl: app-managed cert at %s/ failed validation; will request new cert", dir)
		}
	}

	// Priority 4: {cert_dir}/local/{fqdn}/ (user-provided).
	if m.config.CertDir != "" && fqdn != "" {
		dir := filepath.Join(m.config.CertDir, "local", fqdn)
		if cert, key := findCertPair(dir); cert != "" {
			if err := validateCertFile(cert, fqdn); err == nil {
				log.Printf("ssl: using local cert %s/", dir)
				return tlsConfigFromFiles(cert, key)
			}
		}
	}

	// No existing cert found — request via Let's Encrypt if enabled. Every
	// domain MUST pass production-grade SSL host validation (PART 12,
	// "SSL/Let's Encrypt FQDN Requirements"): dev-only TLDs, project TLDs,
	// bare IPs, and .onion hosts can never receive a publicly trusted cert.
	// Invalid domains are dropped with a warning rather than failing the
	// whole request; if none remain, LE is skipped and SSL falls through to
	// the overlay/self-signed or "no certificate" paths below.
	if m.config.LetsEncrypt.Enabled {
		validDomains := make([]string, 0, len(domains))
		for _, d := range domains {
			if config.IsValidSSLHost(d) {
				validDomains = append(validDomains, d)
			} else {
				log.Printf("ssl: domain %q is not eligible for Let's Encrypt (dev-only TLD, IP, or overlay host); skipping", d)
			}
		}
		if len(validDomains) > 0 {
			return m.getLetsEncryptTLSConfig(validDomains)
		}
		log.Printf("ssl: no domains passed SSL host validation for %q; skipping Let's Encrypt request", fqdn)
	}

	// Overlay networks (.onion/.i2p/.exit) cannot use Let's Encrypt, so HTTPS on
	// an overlay host falls back to a cached self-signed cert in
	// {cert_dir}/local/{fqdn}/ (PART 15). Clearnet hosts NEVER self-sign.
	if isOverlayHost(fqdn) && m.config.CertDir != "" {
		dir := filepath.Join(m.config.CertDir, "local", fqdn)
		certPath, keyPath, err := ensureSelfSignedCert(dir, fqdn)
		if err != nil {
			return nil, fmt.Errorf("ssl: overlay self-signed cert generation failed for %q: %w", fqdn, err)
		}
		log.Printf("ssl: using self-signed cert for overlay host %s at %s/", fqdn, dir)
		return tlsConfigFromFiles(certPath, keyPath)
	}

	return nil, fmt.Errorf("ssl: no certificates available for %q and Let's Encrypt not enabled", fqdn)
}

// getLetsEncryptTLSConfig configures autocert for Let's Encrypt.
// Certificates are cached in {cert_dir}/letsencrypt/{fqdn}/.
func (m *Manager) getLetsEncryptTLSConfig(domains []string) (*tls.Config, error) {
	cacheDir := filepath.Join(m.config.CertDir, "letsencrypt", m.config.FQDN)
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("ssl: failed to create cert cache dir %s: %w", cacheDir, err)
	}

	acmeMgr := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Cache:      autocert.DirCache(cacheDir),
		Email:      m.config.LetsEncrypt.Email,
	}

	// Use staging CA when requested (e.g., for testing).
	if m.config.LetsEncrypt.Staging {
		acmeMgr.Client = &acme.Client{
			DirectoryURL: LetsEncryptStagingURL,
		}
	}

	m.certManager = acmeMgr
	return acmeMgr.TLSConfig(), nil
}

// GetHTTPHandler wraps fallback with the ACME HTTP-01 challenge handler.
// Only active when autocert is managing certificates.
func (m *Manager) GetHTTPHandler(fallback http.Handler) http.Handler {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.certManager != nil {
		return m.certManager.HTTPHandler(fallback)
	}
	return fallback
}

// ParseChallenge normalises a challenge type string to its canonical form.
func ParseChallenge(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "http-01", "http01", "http":
		return "http-01"
	case "tls-alpn-01", "tlsalpn01", "tls-alpn", "tls":
		return "tls-alpn-01"
	case "dns-01", "dns01", "dns":
		return "dns-01"
	default:
		return "http-01"
	}
}

// findCertPair looks for fullchain.pem+privkey.pem or cert.pem+key.pem in dir.
func findCertPair(dir string) (certPath, keyPath string) {
	pairs := [][2]string{
		{filepath.Join(dir, "fullchain.pem"), filepath.Join(dir, "privkey.pem")},
		{filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")},
	}
	for _, p := range pairs {
		if fileReadable(p[0]) && fileReadable(p[1]) {
			return p[0], p[1]
		}
	}
	return "", ""
}

// validateCertFile loads the PEM certificate at certPath, parses the first
// certificate block, and verifies:
//   - not expired
//   - covers fqdn (CN or SAN match), when fqdn is not empty or "localhost"
func validateCertFile(certPath, fqdn string) error {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("cannot read cert: %w", err)
	}

	var cert *x509.Certificate
	for rest := data; len(rest) > 0; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("cannot parse certificate: %w", err)
		}
		break
	}
	if cert == nil {
		return fmt.Errorf("no CERTIFICATE PEM block found in %s", certPath)
	}

	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("certificate expired at %s", cert.NotAfter.Format(time.RFC3339))
	}
	if fqdn == "" || fqdn == "localhost" {
		return nil
	}
	return cert.VerifyHostname(fqdn)
}

// tlsConfigFromFiles loads a TLS certificate and key from disk.
func tlsConfigFromFiles(cert, key string) (*tls.Config, error) {
	tlsCert, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("ssl: failed to load key pair (%s, %s): %w", cert, key, err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// fileReadable reports whether path exists and can be opened for reading.
func fileReadable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}
