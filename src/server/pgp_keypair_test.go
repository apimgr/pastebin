package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/pgp"
)

func newPGPTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		configDir:     t.TempDir(),
		installSecret: []byte("0123456789abcdef0123456789abcdef"),
	}
}

func TestPGPWrapKeyDeterministic(t *testing.T) {
	s := newPGPTestServer(t)
	k1, err := s.pgpWrapKey()
	if err != nil {
		t.Fatalf("pgpWrapKey: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("wrap key length = %d, want 32", len(k1))
	}
	k2, _ := s.pgpWrapKey()
	if string(k1) != string(k2) {
		t.Error("wrap key is not deterministic for the same installation_secret")
	}
	// A different installation_secret must derive a different key.
	s.installSecret = []byte("ffffffffffffffffffffffffffffffff")
	k3, _ := s.pgpWrapKey()
	if string(k1) == string(k3) {
		t.Error("wrap key did not change with installation_secret")
	}
}

func TestPrivateKeyRoundTripOnDisk(t *testing.T) {
	s := newPGPTestServer(t)
	kp, err := pgp.Generate("Pastebin", "security@example.com", time.Now(), keypairValidity)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := s.writePrivateKey(s.pgpPrivateKeyPath(), kp.PrivateArmored); err != nil {
		t.Fatalf("writePrivateKey: %v", err)
	}
	// On-disk file must be sealed, never plaintext-armored.
	raw, err := os.ReadFile(s.pgpPrivateKeyPath())
	if err != nil {
		t.Fatalf("read sealed key: %v", err)
	}
	if len(raw) == 0 || string(raw) == kp.PrivateArmored {
		t.Fatal("private key was not encrypted at rest")
	}
	got, err := s.readPrivateKey(s.pgpPrivateKeyPath())
	if err != nil {
		t.Fatalf("readPrivateKey: %v", err)
	}
	if got != kp.PrivateArmored {
		t.Error("private key round trip mismatch")
	}
}

func TestEncryptSecurityReportPrefersPGP(t *testing.T) {
	s := newPGPTestServer(t)
	kp, err := pgp.Generate("Pastebin", "security@example.com", time.Now(), keypairValidity)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.pgpPublicKeyPath()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(s.pgpPublicKeyPath(), []byte(kp.PublicArmored), 0o644); err != nil {
		t.Fatalf("write public key: %v", err)
	}
	if err := s.writePrivateKey(s.pgpPrivateKeyPath(), kp.PrivateArmored); err != nil {
		t.Fatalf("writePrivateKey: %v", err)
	}

	aesKey := []byte("0123456789abcdef0123456789abcdef")
	body := []byte("sensitive vulnerability report body")
	sealed, method, err := s.encryptSecurityReport(body, aesKey)
	if err != nil {
		t.Fatalf("encryptSecurityReport: %v", err)
	}
	if method != "pgp" {
		t.Fatalf("enc method = %q, want pgp", method)
	}
	rec := &database.SecurityReport{TrackingID: "sec_test", EncMethod: method, EncryptedBody: sealed}
	plain, err := s.decryptSecurityReport(rec, aesKey)
	if err != nil {
		t.Fatalf("decryptSecurityReport: %v", err)
	}
	if string(plain) != string(body) {
		t.Errorf("round trip = %q, want %q", plain, body)
	}
}

func TestEncryptSecurityReportAESFallback(t *testing.T) {
	s := newPGPTestServer(t)
	aesKey := []byte("0123456789abcdef0123456789abcdef")
	body := []byte("report with no project key present")
	sealed, method, err := s.encryptSecurityReport(body, aesKey)
	if err != nil {
		t.Fatalf("encryptSecurityReport: %v", err)
	}
	if method != "aes-256-gcm" {
		t.Fatalf("enc method = %q, want aes-256-gcm", method)
	}
	rec := &database.SecurityReport{TrackingID: "sec_test", EncMethod: method, EncryptedBody: sealed}
	plain, err := s.decryptSecurityReport(rec, aesKey)
	if err != nil {
		t.Fatalf("decryptSecurityReport: %v", err)
	}
	if string(plain) != string(body) {
		t.Errorf("round trip = %q, want %q", plain, body)
	}
}

func TestMergeKeyserverPublishesDeduplicates(t *testing.T) {
	prior := []database.KeyserverPublish{
		{URL: "https://keys.openpgp.org", PublishedAt: time.Unix(1000, 0)},
	}
	fresh := []database.KeyserverPublish{
		{URL: "https://keys.openpgp.org", PublishedAt: time.Unix(2000, 0)},
		{URL: "https://keyserver.ubuntu.com", PublishedAt: time.Unix(2000, 0)},
	}
	merged := mergeKeyserverPublishes(prior, fresh)
	if len(merged) != 2 {
		t.Fatalf("merged length = %d, want 2", len(merged))
	}
	for _, p := range merged {
		if p.URL == "https://keys.openpgp.org" && !p.PublishedAt.Equal(time.Unix(2000, 0)) {
			t.Error("newer publish timestamp did not win")
		}
	}
}
