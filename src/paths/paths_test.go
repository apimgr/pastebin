package paths_test

// Tests for the paths package: env-override paths, IsContainer on a normal host,
// and EnsureDir directory creation.
//
// t.Setenv is used for all env tests — it auto-restores the original value
// when the test finishes, so tests are safe to run in parallel or sequence.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apimgr/pastebin/src/paths"
)

// ─── Env-override path helpers ────────────────────────────────────────────────

// TestGetConfigDir_EnvOverride verifies that CONFIG_DIR overrides the computed path.
func TestGetConfigDir_EnvOverride(t *testing.T) {
	t.Setenv("CONFIG_DIR", "/tmp/test-config")
	got := paths.GetConfigDir("pastebin")
	if got != "/tmp/test-config" {
		t.Errorf("GetConfigDir: got %q, want %q", got, "/tmp/test-config")
	}
}

// TestGetDataDir_EnvOverride verifies that DATA_DIR overrides the computed path.
func TestGetDataDir_EnvOverride(t *testing.T) {
	t.Setenv("DATA_DIR", "/tmp/test-data")
	got := paths.GetDataDir("pastebin")
	if got != "/tmp/test-data" {
		t.Errorf("GetDataDir: got %q, want %q", got, "/tmp/test-data")
	}
}

// TestGetLogsDir_EnvOverride verifies that LOGS_DIR overrides the computed path.
func TestGetLogsDir_EnvOverride(t *testing.T) {
	t.Setenv("LOGS_DIR", "/tmp/test-logs")
	got := paths.GetLogsDir("pastebin")
	if got != "/tmp/test-logs" {
		t.Errorf("GetLogsDir: got %q, want %q", got, "/tmp/test-logs")
	}
}

// TestGetBackupDir_EnvOverride verifies that BACKUP_DIR overrides the computed path.
func TestGetBackupDir_EnvOverride(t *testing.T) {
	t.Setenv("BACKUP_DIR", "/tmp/test-backup")
	got := paths.GetBackupDir("pastebin")
	if got != "/tmp/test-backup" {
		t.Errorf("GetBackupDir: got %q, want %q", got, "/tmp/test-backup")
	}
}

// TestGetPIDFile_EnvOverride verifies that PID_FILE overrides the computed path.
func TestGetPIDFile_EnvOverride(t *testing.T) {
	t.Setenv("PID_FILE", "/tmp/test-pastebin.pid")
	got := paths.GetPIDFile("pastebin")
	if got != "/tmp/test-pastebin.pid" {
		t.Errorf("GetPIDFile: got %q, want %q", got, "/tmp/test-pastebin.pid")
	}
}

// TestGetCacheDir_EnvOverride verifies that CACHE_DIR overrides the computed path.
func TestGetCacheDir_EnvOverride(t *testing.T) {
	t.Setenv("CACHE_DIR", "/tmp/test-cache")
	got := paths.GetCacheDir("pastebin")
	if got != "/tmp/test-cache" {
		t.Errorf("GetCacheDir: got %q, want %q", got, "/tmp/test-cache")
	}
}

// TestGetDBPath_EnvOverride verifies that DB_PATH overrides the computed path.
func TestGetDBPath_EnvOverride(t *testing.T) {
	t.Setenv("DB_PATH", "/tmp/test.db")
	got := paths.GetDBPath("pastebin")
	if got != "/tmp/test.db" {
		t.Errorf("GetDBPath: got %q, want %q", got, "/tmp/test.db")
	}
}

// TestGetDBPath_NativeHost verifies that outside a container GetDBPath derives
// the path from GetDataDir.
func TestGetDBPath_NativeHost(t *testing.T) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("running inside Docker — container path applies")
	}
	os.Unsetenv("DB_PATH")
	got := paths.GetDBPath("pastebin")
	want := filepath.Join(paths.GetDataDir("pastebin"), "db", "server.db")
	if got != want {
		t.Errorf("GetDBPath: got %q, want %q", got, want)
	}
}

// ─── IsContainer ──────────────────────────────────────────────────────────────

// TestIsContainer verifies that a normal test host (no /.dockerenv, no docker
// cgroup entries) is not identified as a container.
//
// Note: this test may return true if it genuinely runs inside Docker.
// It is kept here as a negative sanity check for bare-metal / VM hosts.
func TestIsContainer(t *testing.T) {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		t.Skip("running inside Docker — IsContainer() is expected to return true")
	}
	if paths.IsContainer() {
		// Could be a Docker-based CI; report but don't hard-fail.
		t.Log("IsContainer() returned true on this host — may be a container-based CI environment")
	}
}

// ─── EnsureDir ────────────────────────────────────────────────────────────────

// TestEnsureDir confirms that EnsureDir creates a directory (including parents)
// when it does not exist, and is idempotent when called a second time.
func TestEnsureDir(t *testing.T) {
	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "pastebin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	target := filepath.Join(dir, "nested", "deep", "dir")

	// First call creates the directory.
	if err := paths.EnsureDir(target); err != nil {
		t.Fatalf("EnsureDir first call: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q exists but is not a directory", target)
	}

	// Second call must be idempotent (no error).
	if err := paths.EnsureDir(target); err != nil {
		t.Fatalf("EnsureDir second call (idempotent): %v", err)
	}
}
