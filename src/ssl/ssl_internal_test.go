package ssl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── findCertPair ─────────────────────────────────────────────────────────────

func TestFindCertPair_NotFound(t *testing.T) {
	dir := t.TempDir()
	cert, key := findCertPair(dir)
	if cert != "" || key != "" {
		t.Errorf("findCertPair empty dir: got (%q, %q); want (\"\", \"\")", cert, key)
	}
}

func TestFindCertPair_FullchainAndPrivkey(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "privkey.pem")
	if err := os.WriteFile(certPath, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	gotCert, gotKey := findCertPair(dir)
	if gotCert != certPath {
		t.Errorf("cert = %q; want %q", gotCert, certPath)
	}
	if gotKey != keyPath {
		t.Errorf("key = %q; want %q", gotKey, keyPath)
	}
}

func TestFindCertPair_CertAndKey(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	gotCert, gotKey := findCertPair(dir)
	if gotCert != certPath {
		t.Errorf("cert = %q; want %q", gotCert, certPath)
	}
	if gotKey != keyPath {
		t.Errorf("key = %q; want %q", gotKey, keyPath)
	}
}

// ─── validateCertFile ─────────────────────────────────────────────────────────

func TestValidateCertFile_Nonexistent(t *testing.T) {
	err := validateCertFile("/nonexistent/cert.pem", "example.com")
	if err == nil {
		t.Error("expected error for nonexistent cert file")
	}
}

func TestValidateCertFile_NoPEMBlock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(p, []byte("not pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateCertFile(p, "")
	if err == nil {
		t.Error("expected error for file with no PEM block")
	}
}

func TestValidateCertFile_ValidLocalhost(t *testing.T) {
	certPEM, _ := generateSelfSignedCert(t, "localhost", time.Now().Add(24*time.Hour))
	dir := t.TempDir()
	p := filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(p, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	// localhost FQDN skips hostname validation — should succeed.
	if err := validateCertFile(p, "localhost"); err != nil {
		t.Errorf("validateCertFile (localhost): unexpected error: %v", err)
	}
}

func TestValidateCertFile_Expired(t *testing.T) {
	certPEM, _ := generateSelfSignedCert(t, "example.com", time.Now().Add(-time.Hour))
	dir := t.TempDir()
	p := filepath.Join(dir, "cert.pem")
	if err := os.WriteFile(p, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateCertFile(p, "example.com")
	if err == nil {
		t.Error("expected error for expired certificate")
	}
}

// ─── fileReadable ─────────────────────────────────────────────────────────────

func TestFileReadable_Exists(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !fileReadable(p) {
		t.Error("fileReadable: expected true for existing file")
	}
}

func TestFileReadable_Missing(t *testing.T) {
	if fileReadable("/nonexistent/path.pem") {
		t.Error("fileReadable: expected false for missing file")
	}
}

// generateSelfSignedCert generates a minimal self-signed PEM certificate valid
// until notAfter (or expired if notAfter is in the past).
func generateSelfSignedCert(t *testing.T, cn string, notAfter time.Time) (certPEM []byte, keyPEM []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	notBefore := notAfter.Add(-24 * time.Hour)
	if notAfter.Before(time.Now()) {
		notBefore = notAfter.Add(-24 * time.Hour)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		DNSNames:     []string{cn},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	privDer, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDer})
	return certPEM, keyPEM
}
