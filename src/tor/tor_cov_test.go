package tor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ─── FindBinary with PATH override ───────────────────────────────────────────

// TestFindBinary_FoundInPath verifies FindBinary picks up a binary placed in
// a temp directory that is prepended to PATH (empty configuredPath branch).
func TestFindBinary_FoundInPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH separator differences on Windows")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "tor")
	if err := os.WriteFile(bin, []byte("#!/bin/sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	got := FindBinary("")
	if got != bin {
		t.Errorf("FindBinary(\"\") = %q; want %q", got, bin)
	}
}

// TestFindBinary_EmptyPATH_NoCommonPath verifies FindBinary returns "" when
// PATH is empty and no common OS path exists in a temp-only environment.
func TestFindBinary_EmptyPATH_NoCommonPath(t *testing.T) {
	t.Setenv("PATH", "")
	got := FindBinary("")
	if got != "" {
		if _, err := os.Stat(got); err != nil {
			t.Errorf("FindBinary returned non-existent path %q", got)
		}
	}
}

// ─── findInPath edge cases ────────────────────────────────────────────────────

// TestFindInPath_IsDir verifies that a directory with the same name as the
// binary is not returned (must be a regular file, not a dir).
func TestFindInPath_IsDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows-only skip")
	}
	tmp := t.TempDir()
	dirEntry := filepath.Join(tmp, "tor")
	if err := os.Mkdir(dirEntry, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	_, err := findInPath("tor")
	if err == nil {
		t.Error("expected error when only a directory named 'tor' exists in PATH")
	}
}

// TestFindInPath_MultiplePathEntries verifies the function scans all PATH
// entries and returns the first match.
func TestFindInPath_MultiplePathEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows-only skip")
	}
	tmp1 := t.TempDir()
	tmp2 := t.TempDir()
	bin := filepath.Join(tmp2, "myapp")
	if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp1+":"+tmp2)

	got, err := findInPath("myapp")
	if err != nil {
		t.Fatalf("findInPath: unexpected error: %v", err)
	}
	if got != bin {
		t.Errorf("got %q; want %q", got, bin)
	}
}

// ─── ensureTorDirs ────────────────────────────────────────────────────────────

// TestEnsureTorDirs_PermissionsAre0700 verifies that all created directories
// carry 0700 permissions on non-Windows platforms.
func TestEnsureTorDirs_PermissionsAre0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not enforced on Windows")
	}
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	if err := ensureTorDirs(configDir, dataDir); err != nil {
		t.Fatalf("ensureTorDirs: %v", err)
	}

	expectedDirs := []string{
		filepath.Join(configDir, "tor"),
		filepath.Join(dataDir, "tor"),
		filepath.Join(dataDir, "tor", "site"),
	}
	for _, d := range expectedDirs {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("stat %s: %v", d, err)
			continue
		}
		perm := info.Mode().Perm()
		if perm != 0o700 {
			t.Errorf("dir %s has perm %o; want 0700", d, perm)
		}
	}
}

// ─── saveKey error path ───────────────────────────────────────────────────────

// TestSaveKey_UnwritableParent verifies saveKey returns an error when the
// parent directory is not writable (chmod 000 applied to grandparent).
func TestSaveKey_UnwritableParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission enforcement differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	tmp := t.TempDir()
	locked := filepath.Join(tmp, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	keyPath := filepath.Join(locked, "sub", "key")
	err := saveKey(keyPath, fakeKey("test-key"))
	if err == nil {
		t.Error("expected error writing key into unwritable directory")
	}
}

// TestSaveKey_OverwritesExistingKey verifies that saveKey overwrites an
// already-existing key file without error.
func TestSaveKey_OverwritesExistingKey(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "key")

	if err := saveKey(path, fakeKey("first-key")); err != nil {
		t.Fatalf("first saveKey: %v", err)
	}
	if err := saveKey(path, fakeKey("second-key")); err != nil {
		t.Fatalf("second saveKey: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "second-key" {
		t.Errorf("expected 'second-key', got %q", got)
	}
}

// TestSaveKey_FilePermissions verifies the key file is written with 0600
// permissions (owner read/write only).
func TestSaveKey_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not enforced on Windows")
	}
	tmp := t.TempDir()
	path := filepath.Join(tmp, "key")
	if err := saveKey(path, fakeKey("key-data")); err != nil {
		t.Fatalf("saveKey: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("key file perm = %o; want 0600", info.Mode().Perm())
	}
}

// ─── writeIfChanged error path ────────────────────────────────────────────────

// TestWriteIfChanged_UnwritablePath verifies writeIfChanged returns an error
// when the target path is inside a non-writable directory.
func TestWriteIfChanged_UnwritablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission enforcement differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	tmp := t.TempDir()
	locked := filepath.Join(tmp, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	path := filepath.Join(locked, "torrc")
	err := writeIfChanged(path, []byte("content"), 0o600)
	if err == nil {
		t.Error("expected error writing to unwritable path")
	}
}

// ─── getTorConfig additional branches ────────────────────────────────────────

// TestGetTorConfig_AllowUserPreference sets AllowUserPreference=true and
// verifies SocksPort is set to auto.
func TestGetTorConfig_AllowUserPreference(t *testing.T) {
	cfg := &Config{
		AllowUserPreference: true,
		BandwidthRate:       "1 MB",
		BandwidthBurst:      "2 MB",
	}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "SocksPort auto") {
		t.Errorf("expected 'SocksPort auto' when AllowUserPreference=true, got:\n%s", out)
	}
}

// TestGetTorConfig_UnlimitedBandwidth verifies no accounting block is
// generated when MaxMonthlyBandwidth is "unlimited".
func TestGetTorConfig_UnlimitedBandwidth(t *testing.T) {
	cfg := &Config{
		BandwidthRate:       "1 MB",
		BandwidthBurst:      "2 MB",
		MaxMonthlyBandwidth: "unlimited",
	}
	out := getTorConfig(cfg)
	if strings.Contains(out, "AccountingMax") {
		t.Errorf("expected no AccountingMax when MaxMonthlyBandwidth='unlimited', got:\n%s", out)
	}
}

// TestGetTorConfig_EmptyBandwidth verifies no accounting block is generated
// when MaxMonthlyBandwidth is empty.
func TestGetTorConfig_EmptyBandwidth(t *testing.T) {
	cfg := &Config{
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	out := getTorConfig(cfg)
	if strings.Contains(out, "AccountingMax") {
		t.Errorf("expected no AccountingMax when MaxMonthlyBandwidth='', got:\n%s", out)
	}
}

// TestGetTorConfig_ExitPolicyPresent verifies the config always disables
// relay and exit traffic.
func TestGetTorConfig_ExitPolicyPresent(t *testing.T) {
	cfg := &Config{BandwidthRate: "512 KB", BandwidthBurst: "1 MB"}
	out := getTorConfig(cfg)
	for _, required := range []string{"ORPort 0", "DirPort 0", "ExitRelay 0", "ExitPolicy reject *:*"} {
		if !strings.Contains(out, required) {
			t.Errorf("missing %q in torrc:\n%s", required, out)
		}
	}
}

// TestGetTorConfig_BandwidthValuesPresent verifies BandwidthRate and
// BandwidthBurst appear verbatim in the config output.
func TestGetTorConfig_BandwidthValuesPresent(t *testing.T) {
	cfg := &Config{BandwidthRate: "2 MB", BandwidthBurst: "4 MB"}
	out := getTorConfig(cfg)
	if !strings.Contains(out, "BandwidthRate 2 MB") {
		t.Errorf("expected 'BandwidthRate 2 MB' in config, got:\n%s", out)
	}
	if !strings.Contains(out, "BandwidthBurst 4 MB") {
		t.Errorf("expected 'BandwidthBurst 4 MB' in config, got:\n%s", out)
	}
}

// TestGetTorConfig_StartupOptimizationFlags verifies early-directory fetch
// and debugger-attachment directives are present.
func TestGetTorConfig_StartupOptimizationFlags(t *testing.T) {
	cfg := &Config{BandwidthRate: "1 MB", BandwidthBurst: "2 MB"}
	out := getTorConfig(cfg)
	for _, flag := range []string{
		"FetchDirInfoEarly 1",
		"FetchDirInfoExtraEarly 1",
		"DisableDebuggerAttachment 1",
	} {
		if !strings.Contains(out, flag) {
			t.Errorf("expected %q in torrc, got:\n%s", flag, out)
		}
	}
}

// ─── Manager state after Close ────────────────────────────────────────────────

// TestManager_DoubleClose verifies that calling Close twice does not panic.
func TestManager_DoubleClose(t *testing.T) {
	m := NewManager(context.Background(), 9000, Config{})
	m.Close()
	m.Close()
}

// TestManager_GetHTTPClient_UseTorNoSvc verifies that requesting a Tor client
// when no service is running returns a direct client with a non-zero timeout.
func TestManager_GetHTTPClient_UseTorNoSvc(t *testing.T) {
	m := NewManager(context.Background(), 9000, Config{})
	c := m.GetHTTPClient(true)
	if c == nil {
		t.Fatal("GetHTTPClient returned nil")
	}
	if c.Timeout == 0 {
		t.Error("expected non-zero timeout on fallback direct client")
	}
}

// TestManager_GetHTTPClient_DirectTimeout verifies the direct client uses
// a 30-second timeout exactly.
func TestManager_GetHTTPClient_DirectTimeout(t *testing.T) {
	m := NewManager(context.Background(), 9000, Config{})
	c := m.GetHTTPClient(false)
	if c.Timeout.Seconds() != 30 {
		t.Errorf("direct client timeout = %v; want 30s", c.Timeout)
	}
}

// ─── Monitor exits on context cancellation ────────────────────────────────────

// TestManager_Monitor_ExitsOnCancel verifies the Monitor goroutine exits when
// the manager's context is cancelled. No Tor binary is needed — the goroutine
// wakes on ctx.Done() and returns without touching the service.
func TestManager_Monitor_ExitsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(ctx, 9000, Config{})

	done := make(chan struct{})
	go func() {
		m.Monitor()
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not exit after context cancellation within 2 seconds")
	}
}

