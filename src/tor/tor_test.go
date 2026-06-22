package tor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cretz/bine/control"
)

// ─── FindBinary ───────────────────────────────────────────────────────────────

func TestFindBinary_ConfiguredPathExisting(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "tor")
	if err := os.WriteFile(bin, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := FindBinary(bin)
	if got != bin {
		t.Errorf("FindBinary(%q) = %q; want %q", bin, got, bin)
	}
}

func TestFindBinary_ConfiguredPathMissing(t *testing.T) {
	got := FindBinary("/nonexistent/path/to/tor")
	if got != "" {
		t.Errorf("FindBinary of missing path returned %q; want empty", got)
	}
}

func TestFindBinary_EmptyPath_ReturnsSomethingOrEmpty(t *testing.T) {
	// When configured path is empty, FindBinary searches PATH and common locs.
	// We cannot guarantee tor is installed; just verify it doesn't panic and
	// returns a non-empty string only when the path actually exists.
	got := FindBinary("")
	if got != "" {
		if _, err := os.Stat(got); err != nil {
			t.Errorf("FindBinary returned %q but file does not exist: %v", got, err)
		}
	}
}

// ─── findInPath ───────────────────────────────────────────────────────────────

func TestFindInPath_NotFound(t *testing.T) {
	// Save PATH, then override to an empty temp directory.
	orig := os.Getenv("PATH")
	tmp := t.TempDir()
	os.Setenv("PATH", tmp)
	defer os.Setenv("PATH", orig)

	_, err := findInPath("definitely-not-a-real-binary-xyzzy")
	if err == nil {
		t.Error("expected error when binary is not in PATH")
	}
}

func TestFindInPath_Found(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH sep and .exe suffix differ on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "mytor")
	if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := os.Getenv("PATH")
	os.Setenv("PATH", tmp+":"+orig)
	defer os.Setenv("PATH", orig)

	got, err := findInPath("mytor")
	if err != nil {
		t.Fatalf("findInPath returned error: %v", err)
	}
	if got != bin {
		t.Errorf("findInPath = %q; want %q", got, bin)
	}
}

func TestFindInPath_EmptyPATH(t *testing.T) {
	orig := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", orig)

	_, err := findInPath("tor")
	if err == nil {
		t.Error("expected error when PATH is empty")
	}
}

// ─── NewManager ───────────────────────────────────────────────────────────────

func TestNewManager_ReturnsManager(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		SafeLogging:      true,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		VirtualPort:      80,
		BootstrapTimeout: 60,
	}
	m := NewManager(ctx, 8080, cfg)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.Running() {
		t.Error("newly created manager should not be running")
	}
	if m.OnionAddress() != "" {
		t.Errorf("newly created manager OnionAddress should be empty, got %q", m.OnionAddress())
	}
}

// ─── Running / OnionAddress / GetHTTPClient ───────────────────────────────────

func TestManager_RunningFalseWhenNotStarted(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})
	if m.Running() {
		t.Error("Running() should be false before Start()")
	}
}

func TestManager_OnionAddressEmptyWhenNotStarted(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})
	if addr := m.OnionAddress(); addr != "" {
		t.Errorf("OnionAddress() = %q; want empty string", addr)
	}
}

func TestManager_GetHTTPClient_Direct(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})
	c := m.GetHTTPClient(false)
	if c == nil {
		t.Fatal("GetHTTPClient returned nil")
	}
	if c.Timeout == 0 {
		t.Error("expected non-zero timeout on direct HTTP client")
	}
}

func TestManager_GetHTTPClient_TorNotRunning(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})
	// Tor not started — requesting Tor client falls back to direct.
	c := m.GetHTTPClient(true)
	if c == nil {
		t.Fatal("GetHTTPClient returned nil")
	}
}

func TestManager_Close_NoOp(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})
	// Close on an unstarted manager must not panic.
	m.Close()
	if m.Running() {
		t.Error("Running() should be false after Close()")
	}
}

// ─── Start without a real Tor binary ─────────────────────────────────────────

func TestManager_Start_NoTorBinary(t *testing.T) {
	// Point to a non-existent binary; Start should return nil (graceful disable).
	cfg := Config{
		Binary:           "/nonexistent/tor",
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
	}
	m := NewManager(context.Background(), 8080, cfg)
	err := m.Start()
	if err != nil {
		t.Errorf("Start() with missing tor binary returned error: %v", err)
	}
	if m.Running() {
		t.Error("manager should not be running when Tor binary is missing")
	}
}

// ─── getTorConfig ─────────────────────────────────────────────────────────────

func TestGetTorConfig_SafeLoggingEnabled(t *testing.T) {
	cfg := &Config{
		SafeLogging:   true,
		BandwidthRate: "1 MB",
		BandwidthBurst: "2 MB",
	}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "SafeLogging 1") {
		t.Errorf("expected 'SafeLogging 1' in config, got:\n%s", out)
	}
}

func TestGetTorConfig_SafeLoggingDisabled(t *testing.T) {
	cfg := &Config{
		SafeLogging:    false,
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "SafeLogging 0") {
		t.Errorf("expected 'SafeLogging 0' in config, got:\n%s", out)
	}
}

func TestGetTorConfig_SocksPortZeroByDefault(t *testing.T) {
	cfg := &Config{BandwidthRate: "1 MB", BandwidthBurst: "2 MB"}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "SocksPort 0") {
		t.Errorf("expected 'SocksPort 0' when UseNetwork=false, got:\n%s", out)
	}
}

func TestGetTorConfig_SocksPortAutoWhenNetworkEnabled(t *testing.T) {
	cfg := &Config{
		UseNetwork:     true,
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "SocksPort auto") {
		t.Errorf("expected 'SocksPort auto' when UseNetwork=true, got:\n%s", out)
	}
}

func TestGetTorConfig_MonthlyBandwidthLimit(t *testing.T) {
	cfg := &Config{
		BandwidthRate:       "1 MB",
		BandwidthBurst:      "2 MB",
		MaxMonthlyBandwidth: "100 GB",
	}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "AccountingMax 100 GB") {
		t.Errorf("expected 'AccountingMax 100 GB' in config, got:\n%s", out)
	}
}

func TestGetTorConfig_NoDefaultPorts(t *testing.T) {
	cfg := &Config{BandwidthRate: "1 MB", BandwidthBurst: "2 MB"}
	out := getTorConfig(cfg)
	// ControlPort must use auto (never a hardcoded port number).
	if !strings.Contains(out, "ControlPort 127.0.0.1:auto") {
		t.Errorf("expected 'ControlPort 127.0.0.1:auto' in config, got:\n%s", out)
	}
	// SocksPort must be 0 (hidden-service-only mode) or auto — never hardcoded.
	if strings.Contains(out, "SocksPort 9050") || strings.Contains(out, "SocksPort 9051") {
		t.Errorf("torrc must not bind SocksPort to default ports 9050/9051, got:\n%s", out)
	}
}

func TestGetTorConfig_NoRelayOrExit(t *testing.T) {
	cfg := &Config{BandwidthRate: "1 MB", BandwidthBurst: "2 MB"}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "ExitRelay 0") {
		t.Errorf("expected 'ExitRelay 0' in config, got:\n%s", out)
	}
	if !strings.Contains(out, "ExitPolicy reject *:*") {
		t.Errorf("expected reject exit policy in config, got:\n%s", out)
	}
}

// ─── ensureTorDirs ────────────────────────────────────────────────────────────

func TestEnsureTorDirs_CreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("ensureTorDirs error: %v", err)
	}

	expectedDirs := []string{
		filepath.Join(configDir, "tor"),
		filepath.Join(dataDir, "tor"),
		filepath.Join(dataDir, "tor", "site"),
	}
	for _, d := range expectedDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

func TestEnsureTorDirs_Idempotent(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	// Call twice; second call should succeed without error.
	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("second call error: %v", err)
	}
}

// ─── writeIfChanged ───────────────────────────────────────────────────────────

func TestWriteIfChanged_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "torrc")
	content := []byte("# torrc content\n")

	if err := writeIfChanged(path, content, 0o600); err != nil {
		t.Fatalf("writeIfChanged error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("got %q; want %q", got, content)
	}
}

func TestWriteIfChanged_NoWriteWhenUnchanged(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "torrc")
	content := []byte("# torrc content\n")

	if err := writeIfChanged(path, content, 0o600); err != nil {
		t.Fatalf("first write error: %v", err)
	}

	info1, _ := os.Stat(path)

	// Second write of identical content — file mod time should be identical
	// (writeIfChanged skips the write).
	if err := writeIfChanged(path, content, 0o600); err != nil {
		t.Fatalf("second write error: %v", err)
	}

	info2, _ := os.Stat(path)
	if info1.ModTime() != info2.ModTime() {
		t.Error("writeIfChanged should skip write when content is unchanged")
	}
}

func TestWriteIfChanged_UpdatesWhenChanged(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "torrc")

	if err := writeIfChanged(path, []byte("v1"), 0o600); err != nil {
		t.Fatalf("first write error: %v", err)
	}
	if err := writeIfChanged(path, []byte("v2"), 0o600); err != nil {
		t.Fatalf("second write error: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "v2" {
		t.Errorf("expected 'v2' after update, got %q", got)
	}
}

// ─── saveKey ──────────────────────────────────────────────────────────────────

func TestSaveKey_CreatesParentDirs(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "dir", "key")

	// fakeKey satisfies control.Key interface with a Blob() method returning a string.
	err := saveKey(path, fakeKey("fake-key-data"))
	if err != nil {
		t.Fatalf("saveKey error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(got) != "fake-key-data" {
		t.Errorf("got %q; want 'fake-key-data'", got)
	}
}

// fakeKey is a test helper implementing control.Key (Type + Blob).
type fakeKey string

func (f fakeKey) Blob() string {
	return string(f)
}

func (f fakeKey) Type() control.KeyType {
	return control.KeyTypeED25519V3
}
