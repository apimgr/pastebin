package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
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
