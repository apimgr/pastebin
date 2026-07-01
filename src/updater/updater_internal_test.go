package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─── matchesBranch ────────────────────────────────────────────────────────────

func TestMatchesBranch(t *testing.T) {
	cases := []struct {
		branch string
		tag    string
		pre    bool
		want   bool
	}{
		// stable branch (default) — non-prerelease only
		{"stable", "v1.0.0", false, true},
		{"stable", "v1.0.0", true, false},
		{"", "v1.0.0", false, true},
		{"", "v1.0.0-beta", true, false},
		// beta branch — tag ends in "-beta"
		{"beta", "v1.0.0-beta", false, true},
		{"beta", "v1.0.0", false, false},
		{"beta", "v1.0.0-rc1", false, false},
		// daily branch — 14-char tag with no dots
		{"daily", "20250115120000", false, true},
		{"daily", "2025011512000", false, false},  // too short (13)
		{"daily", "202501151200000", false, false}, // too long (15)
		{"daily", "v1.0.20250115", false, false},   // contains dot
		// unknown branch behaves like stable
		{"unknown", "v2.0.0", false, true},
		{"unknown", "v2.0.0", true, false},
	}
	for _, tc := range cases {
		r := Release{TagName: tc.tag, Prerelease: tc.pre}
		got := matchesBranch(r, tc.branch)
		if got != tc.want {
			t.Errorf("matchesBranch(%q, tag=%q, pre=%v) = %v, want %v",
				tc.branch, tc.tag, tc.pre, got, tc.want)
		}
	}
}

// ─── fetchChecksum ────────────────────────────────────────────────────────────

func TestFetchChecksum_HashAndFilenameFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc123def456abc1  pastebin-linux-amd64\n"))
	}))
	defer srv.Close()

	client := &http.Client{}
	hash, err := fetchChecksum(context.Background(), client, srv.URL, "pastebin-linux-amd64")
	if err != nil {
		t.Fatalf("fetchChecksum error: %v", err)
	}
	if hash != "abc123def456abc1" {
		t.Errorf("got %q; want abc123def456abc1", hash)
	}
}

func TestFetchChecksum_SingleHash(t *testing.T) {
	// Some checksum files contain only the hash with no filename column.
	const h64 = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(h64))
	}))
	defer srv.Close()

	client := &http.Client{}
	hash, err := fetchChecksum(context.Background(), client, srv.URL, "pastebin-linux-amd64")
	if err != nil {
		t.Fatalf("fetchChecksum error: %v", err)
	}
	if hash != h64 {
		t.Errorf("got %q; want %q", hash, h64)
	}
}

func TestFetchChecksum_NoMatch_ReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("abc  other-binary\n"))
	}))
	defer srv.Close()

	client := &http.Client{}
	hash, err := fetchChecksum(context.Background(), client, srv.URL, "pastebin-linux-amd64")
	if err != nil {
		t.Fatalf("fetchChecksum error: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty hash for no-match, got %q", hash)
	}
}

// ─── binaryAssetName ─────────────────────────────────────────────────────────

func TestBinaryAssetName(t *testing.T) {
	name := binaryAssetName()
	if name == "" {
		t.Error("binaryAssetName returned empty string")
	}
	// Must start with the project name.
	if len(name) < len(projectName) || name[:len(projectName)] != projectName {
		t.Errorf("binaryAssetName %q does not start with %q", name, projectName)
	}
}

// ─── replaceBinary ────────────────────────────────────────────────────────────

func TestReplaceBinary_ReplacesFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replaceBinary on Windows uses a different codepath")
	}
	tmp := t.TempDir()
	current := filepath.Join(tmp, "current")
	newBin := filepath.Join(tmp, "new")

	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatalf("write current: %v", err)
	}
	if err := os.WriteFile(newBin, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new: %v", err)
	}

	if err := replaceBinary(current, newBin); err != nil {
		t.Fatalf("replaceBinary error: %v", err)
	}

	got, err := os.ReadFile(current)
	if err != nil {
		t.Fatalf("ReadFile after replace: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("after replace, content = %q; want 'new'", got)
	}
	// The temp file should have been renamed away (no longer exists at original path).
	if _, err := os.Stat(newBin); err == nil {
		t.Error("temp binary should not exist after successful replace")
	}
}

func TestReplaceBinary_MissingCurrent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows path")
	}
	tmp := t.TempDir()
	newBin := filepath.Join(tmp, "new")
	if err := os.WriteFile(newBin, []byte("new"), 0o755); err != nil {
		t.Fatalf("write new: %v", err)
	}
	err := replaceBinary(filepath.Join(tmp, "nonexistent"), newBin)
	if err == nil {
		t.Error("expected error when current binary does not exist")
	}
}

// ─── DoUpdate ────────────────────────────────────────────────────────────────

func TestDoUpdate_NoBinaryForPlatform(t *testing.T) {
	rel := Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: "other-binary-other-arch", BrowserDownloadURL: "http://example.com/other"},
		},
	}
	err := DoUpdate(context.Background(), &rel)
	if err == nil {
		t.Error("expected error when no binary matches current platform")
	}
}

func TestDoUpdate_DownloadsAsset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replaceBinary Windows path differs")
	}

	assetName := binaryAssetName()
	served := []byte("fake binary v2")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(served)
	}))
	defer srv.Close()

	rel := Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: srv.URL + "/bin"},
		},
	}

	// DoUpdate calls os.Executable() to find the binary to replace.
	// In test context it will attempt to replace the test binary itself, which
	// may or may not succeed depending on permissions; we only assert the error
	// is NOT about a missing/undownloaded asset.
	err := DoUpdate(context.Background(), &rel)
	if err != nil {
		if strings.Contains(err.Error(), "no binary") {
			t.Errorf("should have found asset %q: %v", assetName, err)
		}
		if strings.Contains(err.Error(), "downloading update") {
			t.Errorf("download should have succeeded: %v", err)
		}
	}
}

func TestDoUpdate_InvalidDownloadURL(t *testing.T) {
	assetName := binaryAssetName()
	rel := Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: "http://127.0.0.1:1/nonexistent"},
		},
	}
	err := DoUpdate(context.Background(), &rel)
	if err == nil {
		t.Error("expected error when download URL is unreachable")
	}
}

// ─── CheckForUpdate (URL dispatch) ───────────────────────────────────────────

func TestCheckForUpdate_Stable_HitsLatestEndpoint(t *testing.T) {
	latestCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/latest" {
			latestCalled = true
		}
		json.NewEncoder(w).Encode(Release{TagName: "v2.0.0"})
	}))
	defer srv.Close()

	_, _ = CheckForUpdateURL(context.Background(), "v1.0.0", "stable", srv.URL+"/releases/latest")
	if !latestCalled {
		t.Error("stable branch should hit /releases/latest endpoint")
	}
}

func TestCheckForUpdateURL_BetaAlreadyUpToDate(t *testing.T) {
	releases := []Release{
		{TagName: "v1.0.0-beta", Prerelease: false},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	// Current version matches the only beta release → should return nil, nil.
	rel, err := CheckForUpdateURL(context.Background(), "v1.0.0-beta", "beta", srv.URL+"/releases")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != nil {
		t.Errorf("expected nil (already up to date), got %+v", rel)
	}
}

func TestCheckForUpdateURL_NonStableInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not a json array"))
	}))
	defer srv.Close()

	_, err := CheckForUpdateURL(context.Background(), "v1.0.0", "beta", srv.URL+"/releases")
	if err == nil {
		t.Error("expected error for invalid JSON release list, got nil")
	}
}

func TestDoUpdate_WithChecksumAsset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replaceBinary Windows path differs")
	}

	assetName := binaryAssetName()
	checksumName := assetName + ".sha256"
	fakeContent := []byte("hello from fake binary")

	// We'll compute the expected sha256.
	h := sha256.New()
	h.Write(fakeContent)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			w.Write([]byte(expectedHash + "  " + assetName + "\n"))
		} else {
			w.Write(fakeContent)
		}
	}))
	defer srv.Close()

	rel := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: srv.URL + "/bin"},
			{Name: checksumName, BrowserDownloadURL: srv.URL + "/bin.sha256"},
		},
	}

	// DoUpdate may fail at replaceBinary (test binary permissions), but must NOT
	// fail at the checksum verification step.
	err := DoUpdate(context.Background(), rel)
	if err != nil {
		if strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("checksum should match: %v", err)
		}
	}
}

func TestDoUpdate_ChecksumMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replaceBinary Windows path differs")
	}

	assetName := binaryAssetName()
	checksumName := assetName + ".sha256"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			// Return a 64-char wrong hash.
			w.Write([]byte("0000000000000000000000000000000000000000000000000000000000000000"))
		} else {
			w.Write([]byte("some binary content"))
		}
	}))
	defer srv.Close()

	rel := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: srv.URL + "/bin"},
			{Name: checksumName, BrowserDownloadURL: srv.URL + "/bin.sha256"},
		},
	}

	err := DoUpdate(context.Background(), rel)
	if err == nil {
		t.Error("expected checksum mismatch error, got nil")
	} else if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("expected checksum error, got: %v", err)
	}
}

// TestDoUpdate_NoChecksumRefuses verifies the fail-closed policy: a release
// that ships the binary but no matching .sha256 sidecar must be refused
// rather than installed unverified.
func TestDoUpdate_NoChecksumRefuses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replaceBinary Windows path differs")
	}

	assetName := binaryAssetName()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("unverified binary content"))
	}))
	defer srv.Close()

	rel := &Release{
		TagName: "v9.9.9",
		Assets: []Asset{
			{Name: assetName, BrowserDownloadURL: srv.URL + "/bin"},
		},
	}

	err := DoUpdate(context.Background(), rel)
	if err == nil {
		t.Fatal("expected refusal when no checksum is published, got nil")
	}
	if !strings.Contains(err.Error(), "no SHA-256 checksum") {
		t.Errorf("expected unverified-update refusal, got: %v", err)
	}
}
