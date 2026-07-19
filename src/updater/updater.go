// Package updater implements self-update logic against GitHub Releases.
package updater

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
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

	// 256 MiB cap
	lr := io.LimitReader(resp.Body, 256<<20)
	if _, err := io.Copy(tmpFile, lr); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing download: %w", err)
	}
	tmpFile.Close()

	// Verify SHA256 checksum against the release's checksums.txt (MANDATORY)
	expectedHash, err := fetchExpectedChecksum(ctx, client, release, assetName)
	if err != nil {
		return fmt.Errorf("failed to fetch checksum: %w", err)
	}
	if err := verifyChecksum(tmpPath, expectedHash); err != nil {
		return err
	}

	// Set executable bit (Unix).
	if runtime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, 0o755); err != nil {
			return fmt.Errorf("chmod: %w", err)
		}
	}

	return replaceBinary(currentPath, tmpPath)
}

// fetchExpectedChecksum downloads the release's checksums.txt asset and
// returns the SHA256 hash recorded for assetName. Every published release
// MUST ship a checksums.txt — refusing an unverified binary prevents
// installing a tampered or MITM-substituted download (fail closed).
func fetchExpectedChecksum(ctx context.Context, client *http.Client, release *Release, assetName string) (string, error) {
	var checksumsURL string
	for _, a := range release.Assets {
		if a.Name == "checksums.txt" {
			checksumsURL = a.BrowserDownloadURL
			break
		}
	}
	if checksumsURL == "" {
		return "", fmt.Errorf("release has no checksums.txt asset; refusing to install unverified update")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	// Each line is "{sha256}  {filename}" (sha256sum output format).
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s in checksums.txt; refusing to install unverified update", assetName)
}

// verifyChecksum computes the SHA256 of the file at path and compares it
// (constant-time) to expectedHash (hex-encoded).
func verifyChecksum(path, expectedHash string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(h.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) != 1 {
		return fmt.Errorf("checksum mismatch: downloaded binary failed verification")
	}
	return nil
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

// matchesBranch implements cumulative channels: each channel also accepts
// every release from all more-stable channels.
func matchesBranch(r Release, branch string) bool {
	// stable releases match every channel
	if !r.Prerelease {
		return true
	}
	isBeta := strings.HasSuffix(r.TagName, "-beta")
	// Daily builds are timestamps: YYYYMMDDHHMMSS (14 chars, no dots).
	isDaily := len(r.TagName) == 14 && !strings.Contains(r.TagName, ".")
	switch branch {
	case "beta":
		return isBeta
	case "daily":
		return isBeta || isDaily
	default:
		return false
	}
}
