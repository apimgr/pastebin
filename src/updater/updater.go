// Package updater implements self-update logic against GitHub Releases.
package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	orgName     = "apimgr"
	projectName = "pastebin"
	apiBase     = "https://api.github.com/repos/" + orgName + "/" + projectName + "/releases"
)

// Release represents a single GitHub release.
type Release struct {
	TagName     string    `json:"tag_name"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset is a downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckForUpdate queries GitHub Releases for a newer version on the given
// branch ("stable", "beta", or "daily").  Returns nil, nil when already
// up to date or when no release is found.
func CheckForUpdate(ctx context.Context, currentVersion, branch string) (*Release, error) {
	var apiURL string
	switch branch {
	case "stable", "":
		apiURL = apiBase + "/latest"
	default:
		apiURL = apiBase
	}
	return CheckForUpdateURL(ctx, currentVersion, branch, apiURL)
}

// CheckForUpdateURL is the testable core of CheckForUpdate.  It queries the
// given apiURL (so tests can inject an httptest server) and otherwise behaves
// identically to CheckForUpdate.
func CheckForUpdateURL(ctx context.Context, currentVersion, branch, apiURL string) (*Release, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// no updates available
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API: HTTP %d", resp.StatusCode)
	}

	// 4 MiB cap
	lr := io.LimitReader(resp.Body, 4<<20)

	if branch == "stable" || branch == "" {
		var rel Release
		if err := json.NewDecoder(lr).Decode(&rel); err != nil {
			return nil, fmt.Errorf("decode release: %w", err)
		}
		if rel.TagName == currentVersion {
			return nil, nil
		}
		return &rel, nil
	}

	var releases []Release
	if err := json.NewDecoder(lr).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	for _, r := range releases {
		if matchesBranch(r, branch) && r.TagName != currentVersion {
			return &r, nil
		}
	}
	return nil, nil
}

// DoUpdate downloads the release binary, verifies it, and replaces the
// running binary. The caller must restart the process afterwards.
func DoUpdate(ctx context.Context, release *Release) error {
	assetName := binaryAssetName()
	var downloadURL string
	for _, a := range release.Assets {
		if a.Name == assetName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return fmt.Errorf("no binary for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// Download to a temp file inside the same directory as the binary so
	// os.Rename stays on the same filesystem (atomic on Unix).
	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}
	currentPath, err = filepath.EvalSymlinks(currentPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	tmpDir := filepath.Dir(currentPath)
	tmpFile, err := os.CreateTemp(tmpDir, projectName+"-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	// best-effort cleanup on error
	defer os.Remove(tmpPath)

	client := &http.Client{Timeout: 10 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		tmpFile.Close()
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()

	h := sha256.New()
	// 256 MiB cap
	lr := io.LimitReader(resp.Body, 256<<20)
	if _, err := io.Copy(io.MultiWriter(tmpFile, h), lr); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing download: %w", err)
	}
	tmpFile.Close()

	// Set executable bit (Unix).
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return fmt.Errorf("chmod: %w", err)
		}
	}

	// Look for a matching checksum asset (e.g. pastebin-linux-amd64.sha256).
	checksumAsset := assetName + ".sha256"
	var expectedHash string
	for _, a := range release.Assets {
		if a.Name == checksumAsset {
			expectedHash, err = fetchChecksum(ctx, client, a.BrowserDownloadURL, assetName)
			if err != nil {
				return fmt.Errorf("fetching checksum: %w", err)
			}
			break
		}
	}
	// Fail closed: every published release MUST ship a SHA-256 checksum.
	// Refusing an unverified binary prevents installing a tampered or
	// MITM-substituted download.
	if expectedHash == "" {
		return fmt.Errorf("no SHA-256 checksum published for %s; refusing to install unverified update", assetName)
	}
	actualHash := hex.EncodeToString(h.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return replaceBinary(currentPath, tmpPath)
}

// fetchChecksum retrieves the checksum file and extracts the hash for
// the named asset.  Checksum files may be in "hash  filename" format.
func fetchChecksum(ctx context.Context, client *http.Client, url, assetName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == assetName {
			return parts[0], nil
		}
	}
	// Single-hash file.
	trimmed := strings.TrimSpace(string(data))
	if len(trimmed) == 64 {
		return trimmed, nil
	}
	return "", nil
}

// binaryAssetName returns the expected GitHub release asset name for the
// current platform (e.g. "pastebin-linux-amd64").
func binaryAssetName() string {
	name := projectName + "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func matchesBranch(r Release, branch string) bool {
	switch branch {
	case "beta":
		return strings.HasSuffix(r.TagName, "-beta")
	case "daily":
		// Daily builds are timestamps: YYYYMMDDHHMMSS (14 chars, no dots).
		return len(r.TagName) == 14 && !strings.Contains(r.TagName, ".")
	default:
		return !r.Prerelease
	}
}
