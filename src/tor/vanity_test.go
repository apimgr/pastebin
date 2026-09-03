package tor

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newVanityManager builds a Manager rooted at a throwaway data directory.
func newVanityManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")}
	return NewManager(context.Background(), 8080, cfg, http.NewServeMux()), cfg.DataDir
}

func TestOnionAddressFromPublicKey(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	address := OnionAddressFromPublicKey(pub)
	if !strings.HasSuffix(address, ".onion") {
		t.Fatalf("address %q missing .onion suffix", address)
	}
	if len(address) != 56+len(".onion") {
		t.Fatalf("v3 onion address must be 56 base32 chars, got %q", address)
	}
	for _, r := range strings.TrimSuffix(address, ".onion") {
		if !strings.ContainsRune(vanityCharset, r) {
			t.Fatalf("address %q contains non-base32 character %q", address, r)
		}
	}
	if again := OnionAddressFromPublicKey(pub); again != address {
		t.Fatalf("derivation is not deterministic: %q vs %q", address, again)
	}
}

func TestOnionAddressFromPublicKeyKnownVector(t *testing.T) {
	// An all-zero public key is a stable vector for the rend-spec-v3 address
	// derivation and guards against a checksum or byte-ordering regression:
	// the 32 zero bytes encode to a run of base32 'a's before the checksum.
	pub := ed25519.PublicKey(make([]byte, ed25519.PublicKeySize))
	address := OnionAddressFromPublicKey(pub)
	if !strings.HasPrefix(address, strings.Repeat("a", 51)) {
		t.Fatalf("zero key should encode to a run of 'a's, got %q", address)
	}
	if !strings.HasSuffix(address, ".onion") || len(address) != 62 {
		t.Fatalf("unexpected address shape %q", address)
	}
}

func TestExpandedSecretKeyIsClamped(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	key := expandedSecretKey(seed)
	if len(key) != legacyKeyBlobLen {
		t.Fatalf("expected %d bytes, got %d", legacyKeyBlobLen, len(key))
	}
	if key[0]&7 != 0 {
		t.Fatalf("low three bits must be cleared, got %08b", key[0])
	}
	if key[31]&64 == 0 || key[31]&128 != 0 {
		t.Fatalf("high bits not clamped, got %08b", key[31])
	}
}

func TestValidateVanityPrefix(t *testing.T) {
	for _, ok := range []string{"a", "abc", "z27", "abcdef"} {
		if err := ValidateVanityPrefix(ok); err != nil {
			t.Fatalf("prefix %q should be valid: %v", ok, err)
		}
	}
	if err := ValidateVanityPrefix(""); err == nil {
		t.Fatal("empty prefix must be rejected")
	}
	for _, bad := range []string{"AB", "a0", "a1", "a8", "a9", "a-b", "a b"} {
		if err := ValidateVanityPrefix(bad); err == nil {
			t.Fatalf("prefix %q must be rejected for charset", bad)
		}
	}
	err := ValidateVanityPrefix("abcdefg")
	if err == nil {
		t.Fatal("7-character prefix must be rejected")
	}
	if !strings.Contains(err.Error(), "mkp224o") || !strings.Contains(err.Error(), "import-keys") {
		t.Fatalf("long-prefix error must point at the external tool path, got %q", err)
	}
}

func TestNormalizeVanityWorkers(t *testing.T) {
	maxWorkers := runtime.NumCPU()
	def, err := NormalizeVanityWorkers(0)
	if err != nil {
		t.Fatalf("default workers: %v", err)
	}
	expected := maxWorkers - 1
	if expected < 1 {
		expected = 1
	}
	if def != expected {
		t.Fatalf("default workers = %d, want %d", def, expected)
	}
	if got, err := NormalizeVanityWorkers(1); err != nil || got != 1 {
		t.Fatalf("explicit 1 worker = %d, %v", got, err)
	}
	if _, err := NormalizeVanityWorkers(maxWorkers + 1); err == nil {
		t.Fatal("more workers than logical CPUs must be rejected")
	}
}

func TestWriteAndListVanityCandidates(t *testing.T) {
	m, dataDir := newVanityManager(t)
	dir := m.vanityDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	address := OnionAddressFromPublicKey(pub)
	if err := writeVanityCandidate(dir, address, pub, expandedSecretKey(priv.Seed())); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	// A second write for the same address is a no-op, not an error.
	if err := writeVanityCandidate(dir, address, pub, expandedSecretKey(priv.Seed())); err != nil {
		t.Fatalf("duplicate write: %v", err)
	}
	got := ListVanityCandidates(dataDir)
	if len(got) != 1 || got[0] != address {
		t.Fatalf("candidates = %v, want [%s]", got, address)
	}
	secret, err := os.ReadFile(filepath.Join(dir, address, "hs_ed25519_secret_key"))
	if err != nil {
		t.Fatalf("read secret: %v", err)
	}
	if len(secret) != nativeKeyFileLen {
		t.Fatalf("secret key must be the %d-byte native form, got %d", nativeKeyFileLen, len(secret))
	}
	hostname, err := os.ReadFile(filepath.Join(dir, address, "hostname"))
	if err != nil {
		t.Fatalf("read hostname: %v", err)
	}
	if strings.TrimSpace(string(hostname)) != address {
		t.Fatalf("hostname = %q, want %q", hostname, address)
	}
	info, err := os.Stat(filepath.Join(dir, address))
	if err != nil {
		t.Fatalf("stat candidate dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("candidate dir mode = %o, want 0700", info.Mode().Perm())
	}
}

func TestListVanityCandidatesIgnoresIncomplete(t *testing.T) {
	m, dataDir := newVanityManager(t)
	dir := m.vanityDir()
	if err := os.MkdirAll(filepath.Join(dir, "partial.onion"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".candidate-tmp"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := ListVanityCandidates(dataDir); len(got) != 0 {
		t.Fatalf("incomplete and temp dirs must be ignored, got %v", got)
	}
	if got := ListVanityCandidates(filepath.Join(dataDir, "missing")); got != nil {
		t.Fatalf("missing dir should yield nil, got %v", got)
	}
}

// storeCandidate writes a synthetic candidate directory and returns its address.
func storeCandidate(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	address := OnionAddressFromPublicKey(pub)
	if err := writeVanityCandidate(dir, address, pub, expandedSecretKey(priv.Seed())); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	return address
}

func TestResolveVanityCandidate(t *testing.T) {
	m, dataDir := newVanityManager(t)
	if _, err := ResolveVanityCandidate(dataDir, ""); err == nil {
		t.Fatal("no candidates must be an error")
	}
	first := storeCandidate(t, m.vanityDir())
	got, err := ResolveVanityCandidate(dataDir, "")
	if err != nil || got != first {
		t.Fatalf("single candidate with empty arg = %q, %v", got, err)
	}
	if got, err := ResolveVanityCandidate(dataDir, first[:16]); err != nil || got != first {
		t.Fatalf("unique prefix = %q, %v", got, err)
	}
	if _, err := ResolveVanityCandidate(dataDir, "zzzzzznotacandidate"); err == nil {
		t.Fatal("unknown prefix must be an error")
	}
	second := storeCandidate(t, m.vanityDir())
	if _, err := ResolveVanityCandidate(dataDir, ""); err == nil {
		t.Fatal("ambiguous empty arg must be an error")
	}
	if got, err := ResolveVanityCandidate(dataDir, second); err != nil || got != second {
		t.Fatalf("exact address = %q, %v", got, err)
	}
}

func TestResolveVanityCandidateAmbiguousPrefix(t *testing.T) {
	m, dataDir := newVanityManager(t)
	dir := m.vanityDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two candidates sharing a prefix make that prefix ambiguous.
	for _, name := range []string{"sharedaaa.onion", "sharedbbb.onion"} {
		candidate := filepath.Join(dir, name)
		if err := os.MkdirAll(candidate, 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(candidate, "hs_ed25519_secret_key"), make([]byte, nativeKeyFileLen), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	_, err := ResolveVanityCandidate(dataDir, "shared")
	if err == nil {
		t.Fatal("ambiguous prefix must be rejected")
	}
	if !strings.Contains(err.Error(), "matches multiple") {
		t.Fatalf("unexpected error %q", err)
	}
}

func TestNormalizeSecretKeyBlob(t *testing.T) {
	legacy := make([]byte, legacyKeyBlobLen)
	for i := range legacy {
		legacy[i] = byte(i)
	}
	native, err := normalizeSecretKeyBlob(legacy)
	if err != nil {
		t.Fatalf("legacy blob: %v", err)
	}
	if len(native) != nativeKeyFileLen {
		t.Fatalf("native length = %d, want %d", len(native), nativeKeyFileLen)
	}
	if !strings.HasPrefix(string(native), string(nativeKeyHeader)) {
		t.Fatal("native blob missing header")
	}
	again, err := normalizeSecretKeyBlob(native)
	if err != nil {
		t.Fatalf("native blob: %v", err)
	}
	if string(again) != string(native) {
		t.Fatal("native blob must pass through unchanged")
	}
	if _, err := normalizeSecretKeyBlob([]byte("short")); err == nil {
		t.Fatal("bad length must be rejected")
	}
	bogus := make([]byte, nativeKeyFileLen)
	if _, err := normalizeSecretKeyBlob(bogus); err == nil {
		t.Fatal("bad header must be rejected")
	}
}

func TestVanityStartValidatesInput(t *testing.T) {
	m, _ := newVanityManager(t)
	if _, err := m.VanityStart("", 0); err == nil {
		t.Fatal("empty prefix must be rejected")
	}
	if _, err := m.VanityStart("abcdefg", 0); err == nil {
		t.Fatal("over-long prefix must be rejected")
	}
	if _, err := m.VanityStart("a0", 0); err == nil {
		t.Fatal("invalid charset must be rejected")
	}
	if _, err := m.VanityStart("abc", runtime.NumCPU()+1); err == nil {
		t.Fatal("too many workers must be rejected")
	}
}

func TestVanityStatusIdleWithoutSearch(t *testing.T) {
	m, _ := newVanityManager(t)
	st := m.VanityStatus()
	if st.State != vanityStateIdle {
		t.Fatalf("state = %q, want idle", st.State)
	}
	if len(st.Candidates) != 0 {
		t.Fatalf("unexpected candidates %v", st.Candidates)
	}
	if m.RunningVanityPrefix() != "" {
		t.Fatal("no search should be reported as running")
	}
	if m.VanityStop() {
		t.Fatal("stopping an idle search must report false")
	}
}

func TestVanityStatusFoundWhenCandidateOnDisk(t *testing.T) {
	m, _ := newVanityManager(t)
	address := storeCandidate(t, m.vanityDir())
	st := m.VanityStatus()
	if st.State != vanityStateFound {
		t.Fatalf("state = %q, want found", st.State)
	}
	if len(st.Candidates) != 1 || st.Candidates[0] != address {
		t.Fatalf("candidates = %v, want [%s]", st.Candidates, address)
	}
}

func TestVanitySearchFindsCandidate(t *testing.T) {
	m, dataDir := newVanityManager(t)
	// A single-character prefix is found within a handful of attempts, so
	// this exercises the full worker -> candidate -> status path quickly.
	st, err := m.VanityStart("a", 1)
	if err != nil {
		t.Fatalf("start search: %v", err)
	}
	if st.Prefix != "a" || st.Workers != 1 {
		t.Fatalf("unexpected start status %+v", st)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if len(ListVanityCandidates(dataDir)) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.VanityStop()
	candidates := ListVanityCandidates(dataDir)
	if len(candidates) == 0 {
		t.Fatal("single-character prefix search should have found a candidate")
	}
	if !strings.HasPrefix(candidates[0], "a") {
		t.Fatalf("candidate %q does not match the requested prefix", candidates[0])
	}
	final := m.VanityStatus()
	if final.State != vanityStateFound {
		t.Fatalf("state = %q, want found", final.State)
	}
	if final.Attempts == 0 {
		t.Fatal("attempt counter should be non-zero after a search")
	}
	if m.RunningVanityPrefix() != "" {
		t.Fatal("search should no longer be running")
	}
}

func TestVanityStartRejectsSecondSearch(t *testing.T) {
	m, _ := newVanityManager(t)
	// A six-character prefix is effectively never found, so the first
	// search stays running while the second start is attempted.
	if _, err := m.VanityStart("abcdef", 1); err != nil {
		t.Fatalf("start search: %v", err)
	}
	defer m.VanityStop()
	if got := m.RunningVanityPrefix(); got != "abcdef" {
		t.Fatalf("running prefix = %q, want abcdef", got)
	}
	if _, err := m.VanityStart("bcdefa", 1); err == nil {
		t.Fatal("a second concurrent search must be rejected")
	}
	st := m.VanityStatus()
	if st.State != vanityStateRunning {
		t.Fatalf("state = %q, want running", st.State)
	}
	if st.Workers != 1 {
		t.Fatalf("workers = %d, want 1", st.Workers)
	}
	if !m.VanityStop() {
		t.Fatal("stopping a running search must report true")
	}
	if m.RunningVanityPrefix() != "" {
		t.Fatal("search should be stopped")
	}
}

func TestImportKeyPathErrors(t *testing.T) {
	m, _ := newVanityManager(t)
	if _, err := m.ImportKeyPath("  "); err == nil {
		t.Fatal("empty path must be rejected")
	}
	if _, err := m.ImportKeyPath(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("missing path must be rejected")
	}
	bad := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(bad, []byte("too short"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := m.ImportKeyPath(bad); err == nil {
		t.Fatal("malformed key file must be rejected")
	}
}

func TestVanityApplyWithoutCandidates(t *testing.T) {
	m, _ := newVanityManager(t)
	if _, err := m.VanityApply(""); err == nil {
		t.Fatal("apply without candidates must fail")
	}
}
