package config_test

// Tests for the config package: defaults, save/load round-trip, env overrides,
// and the three ResolvePort paths (container, already set, random first-run).

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/apimgr/pastebin/src/config"
)

// tempConfigPath returns a path inside a fresh temp directory that does not exist yet.
func tempConfigPath(t *testing.T) string {
	t.Helper()
	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "pastebin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "server.yml")
}

// ─── DefaultConfig ────────────────────────────────────────────────────────────

// TestDefaultConfig verifies key documented defaults from PART 5.
func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if cfg.Server.Port != "" {
		t.Errorf("Server.Port: got %q, want empty string (first-run randomisation pending)", cfg.Server.Port)
	}
	if cfg.Server.Address != "0.0.0.0" {
		t.Errorf("Server.Address: got %q, want %q", cfg.Server.Address, "0.0.0.0")
	}
	if cfg.Paste.MaxSizeBytes != 10<<20 {
		t.Errorf("Paste.MaxSizeBytes: got %d, want %d", cfg.Paste.MaxSizeBytes, 10<<20)
	}
}

// ─── Save / Load round-trip ───────────────────────────────────────────────────

// TestSaveAndLoad persists a config to a file and reloads it, verifying fields survive.
func TestSaveAndLoad(t *testing.T) {
	path := tempConfigPath(t)

	orig := config.DefaultConfig()
	orig.Server.Port = "12345"
	orig.Web.SiteTitle = "RoundTripTest"
	orig.Paste.MaxSizeBytes = 5 << 20

	if err := config.Save(path, orig); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Server.Port != "12345" {
		t.Errorf("Port: got %q, want %q", loaded.Server.Port, "12345")
	}
	if loaded.Web.SiteTitle != "RoundTripTest" {
		t.Errorf("SiteTitle: got %q, want %q", loaded.Web.SiteTitle, "RoundTripTest")
	}
	if loaded.Paste.MaxSizeBytes != 5<<20 {
		t.Errorf("MaxSizeBytes: got %d, want %d", loaded.Paste.MaxSizeBytes, 5<<20)
	}
}

// TestLoad_CreatesFileIfAbsent verifies that Load creates the file when it is missing.
func TestLoad_CreatesFileIfAbsent(t *testing.T) {
	path := tempConfigPath(t)

	// path does not exist yet.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected file to not exist before Load")
	}

	_, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load on absent file: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected Load to create the file, but it is still absent")
	}
}

// ─── Env overrides ────────────────────────────────────────────────────────────

// TestLoadEnv_PORT verifies that $PORT overrides the config file value.
func TestLoadEnv_PORT(t *testing.T) {
	t.Setenv("PORT", "9999")

	path := tempConfigPath(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != "9999" {
		t.Errorf("Server.Port: got %q, want %q", cfg.Server.Port, "9999")
	}
}

// TestLoadEnv_SITE_TITLE verifies that $SITE_TITLE overrides the config file value.
func TestLoadEnv_SITE_TITLE(t *testing.T) {
	t.Setenv("SITE_TITLE", "MyPaste")

	path := tempConfigPath(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Web.SiteTitle != "MyPaste" {
		t.Errorf("SiteTitle: got %q, want %q", cfg.Web.SiteTitle, "MyPaste")
	}
}

// TestLoadEnv_MAX_SIZE_BYTES verifies that $MAX_SIZE_BYTES overrides the config file value.
func TestLoadEnv_MAX_SIZE_BYTES(t *testing.T) {
	t.Setenv("MAX_SIZE_BYTES", "5242880")

	path := tempConfigPath(t)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Paste.MaxSizeBytes != 5242880 {
		t.Errorf("MaxSizeBytes: got %d, want 5242880", cfg.Paste.MaxSizeBytes)
	}
}

// ─── ResolvePort ──────────────────────────────────────────────────────────────

// TestResolvePort_Container confirms that the container path always yields port 80.
func TestResolvePort_Container(t *testing.T) {
	cfg := config.DefaultConfig()
	// cfgPath is irrelevant in the container branch.
	if err := config.ResolvePort("/dev/null", cfg, true); err != nil {
		t.Fatalf("ResolvePort container: %v", err)
	}
	if cfg.Server.Port != "80" {
		t.Errorf("Port: got %q, want %q", cfg.Server.Port, "80")
	}
}

// TestResolvePort_AlreadySet confirms that an already-configured port is left unchanged.
func TestResolvePort_AlreadySet(t *testing.T) {
	path := tempConfigPath(t)
	cfg := config.DefaultConfig()
	cfg.Server.Port = "8080"

	if err := config.ResolvePort(path, cfg, false); err != nil {
		t.Fatalf("ResolvePort already-set: %v", err)
	}
	if cfg.Server.Port != "8080" {
		t.Errorf("Port: got %q, want %q", cfg.Server.Port, "8080")
	}
}

// TestResolvePort_RandomPort verifies that the first-run path selects a port in
// [64000, 64999] and persists it to the config file.
func TestResolvePort_RandomPort(t *testing.T) {
	path := tempConfigPath(t)
	cfg := config.DefaultConfig()
	// Leave port empty to trigger random selection.

	if err := config.ResolvePort(path, cfg, false); err != nil {
		t.Fatalf("ResolvePort random: %v", err)
	}

	port, err := strconv.Atoi(cfg.Server.Port)
	if err != nil {
		t.Fatalf("resolved port %q is not an integer: %v", cfg.Server.Port, err)
	}
	if port < 64000 || port > 64999 {
		t.Errorf("port %d is outside [64000, 64999]", port)
	}

	// Config file must have been created with the chosen port.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected config file to be created after port selection")
	}

	// Re-loading must return the same port.
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load after ResolvePort: %v", err)
	}
	if loaded.Server.Port != cfg.Server.Port {
		t.Errorf("persisted port %q != resolved port %q", loaded.Server.Port, cfg.Server.Port)
	}
}
