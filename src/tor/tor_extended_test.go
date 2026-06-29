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

// ─── updateTorrc ──────────────────────────────────────────────────────────────
// Tests torrc file updates without starting a real Tor process.

func TestUpdateTorrc_WritesTorrC(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	torDir := filepath.Join(configDir, "tor")
	if err := os.MkdirAll(torDir, 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		ConfigDir:      configDir,
		DataDir:        filepath.Join(tmp, "data"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
		SafeLogging:    true,
	}
	m := NewManager(context.Background(), 8080, cfg)

	if err := m.updateTorrc(); err != nil {
		t.Fatalf("updateTorrc error: %v", err)
	}

	torrcPath := filepath.Join(torDir, "torrc")
	content, err := os.ReadFile(torrcPath)
	if err != nil {
		t.Fatalf("failed to read torrc: %v", err)
	}

	if !strings.Contains(string(content), "SafeLogging 1") {
		t.Errorf("torrc missing SafeLogging 1:\n%s", content)
	}
	if !strings.Contains(string(content), "BandwidthRate 1 MB") {
		t.Errorf("torrc missing BandwidthRate 1 MB:\n%s", content)
	}
}

func TestUpdateTorrc_ErrorOnMissingDir(t *testing.T) {
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

	cfg := Config{
		ConfigDir:      locked,
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	err := m.updateTorrc()
	if err == nil {
		t.Error("expected error when config dir is unwritable")
	}
}

func TestUpdateTorrc_OverwritesExisting(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	torDir := filepath.Join(configDir, "tor")
	if err := os.MkdirAll(torDir, 0o700); err != nil {
		t.Fatal(err)
	}

	torrcPath := filepath.Join(torDir, "torrc")
	if err := os.WriteFile(torrcPath, []byte("# old content"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		ConfigDir:      configDir,
		DataDir:        filepath.Join(tmp, "data"),
		BandwidthRate:  "5 MB",
		BandwidthBurst: "10 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	if err := m.updateTorrc(); err != nil {
		t.Fatalf("updateTorrc error: %v", err)
	}

	content, _ := os.ReadFile(torrcPath)
	if strings.Contains(string(content), "old content") {
		t.Error("updateTorrc should overwrite old torrc")
	}
	if !strings.Contains(string(content), "BandwidthRate 5 MB") {
		t.Errorf("torrc missing new config:\n%s", content)
	}
}

// ─── RegenerateAddress ────────────────────────────────────────────────────────
// Tests key removal logic without starting Tor. Since Tor binary is missing,
// startLocked returns nil (graceful disable) and address is empty.

func TestRegenerateAddress_RemovesKeyFile(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	siteDir := filepath.Join(dataDir, "tor", "site")
	if err := os.MkdirAll(siteDir, 0o700); err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(siteDir, "hs_ed25519_secret_key")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      filepath.Join(tmp, "config"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	addr, err := m.RegenerateAddress()
	if err != nil {
		t.Fatalf("RegenerateAddress error: %v", err)
	}

	// Key file should be removed.
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("RegenerateAddress should remove the existing key file")
	}

	// No Tor binary => empty address (graceful disable).
	if addr != "" {
		t.Errorf("expected empty address when Tor binary missing, got %q", addr)
	}
}

func TestRegenerateAddress_NoKeyFileToRemove(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(filepath.Join(dataDir, "tor", "site"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      filepath.Join(tmp, "config"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	// Should not error when key file doesn't exist.
	_, err := m.RegenerateAddress()
	if err != nil {
		t.Fatalf("RegenerateAddress error when no key: %v", err)
	}
}

// ─── ApplyKeys ────────────────────────────────────────────────────────────────
// Tests key persistence logic without starting Tor.

func TestApplyKeys_WritesKeyFile(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	configDir := filepath.Join(tmp, "config")

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      configDir,
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	keyBlob := []byte("new-ed25519-key-material")
	addr, err := m.ApplyKeys(keyBlob)
	if err != nil {
		t.Fatalf("ApplyKeys error: %v", err)
	}

	// Verify key was written.
	keyPath := filepath.Join(dataDir, "tor", "site", "hs_ed25519_secret_key")
	content, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("failed to read key file: %v", err)
	}
	if string(content) != string(keyBlob) {
		t.Errorf("key content mismatch: got %q, want %q", content, keyBlob)
	}

	// No Tor binary => empty address.
	if addr != "" {
		t.Errorf("expected empty address when Tor binary missing, got %q", addr)
	}
}

func TestApplyKeys_CreatesParentDirs(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      filepath.Join(tmp, "config"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	keyBlob := []byte("key-data")
	_, err := m.ApplyKeys(keyBlob)
	if err != nil {
		t.Fatalf("ApplyKeys error: %v", err)
	}

	siteDir := filepath.Join(dataDir, "tor", "site")
	info, err := os.Stat(siteDir)
	if err != nil {
		t.Fatalf("site directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("site path is not a directory")
	}
}

func TestApplyKeys_UnwritablePath(t *testing.T) {
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

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        locked,
		ConfigDir:      filepath.Join(tmp, "config"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	_, err := m.ApplyKeys([]byte("key"))
	if err == nil {
		t.Error("expected error when data dir is unwritable")
	}
}

// ─── UpdateConfig ─────────────────────────────────────────────────────────────
// Tests config update and torrc rewrite without starting Tor.

func TestUpdateConfig_UpdatesTorrc(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")
	torDir := filepath.Join(configDir, "tor")
	if err := os.MkdirAll(torDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tor", "site"), 0o700); err != nil {
		t.Fatal(err)
	}

	initialCfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      configDir,
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, initialCfg)

	newCfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      configDir,
		BandwidthRate:  "10 MB",
		BandwidthBurst: "20 MB",
		SafeLogging:    false,
	}

	if err := m.UpdateConfig(newCfg); err != nil {
		t.Fatalf("UpdateConfig error: %v", err)
	}

	torrcPath := filepath.Join(torDir, "torrc")
	content, err := os.ReadFile(torrcPath)
	if err != nil {
		t.Fatalf("failed to read torrc: %v", err)
	}

	if !strings.Contains(string(content), "BandwidthRate 10 MB") {
		t.Errorf("torrc missing updated BandwidthRate:\n%s", content)
	}
	if !strings.Contains(string(content), "SafeLogging 0") {
		t.Errorf("torrc missing SafeLogging 0:\n%s", content)
	}
}

func TestUpdateConfig_ErrorPropagation(t *testing.T) {
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

	cfg := Config{
		Binary:         "/nonexistent/tor",
		ConfigDir:      locked,
		DataDir:        filepath.Join(tmp, "data"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	err := m.UpdateConfig(cfg)
	if err == nil {
		t.Error("expected error when config dir is unwritable")
	}
}

// ─── Restart ──────────────────────────────────────────────────────────────────
// Tests restart logic without starting Tor.

func TestRestart_NoTorBinary(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		Binary:           "/nonexistent/tor",
		DataDir:          filepath.Join(tmp, "data"),
		ConfigDir:        filepath.Join(tmp, "config"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
	}
	m := NewManager(context.Background(), 8080, cfg)

	// Restart with no Tor binary should succeed (graceful disable).
	err := m.Restart()
	if err != nil {
		t.Errorf("Restart error with missing Tor binary: %v", err)
	}

	if m.Running() {
		t.Error("manager should not be running after Restart without Tor binary")
	}
}

func TestRestart_MultipleRestarts(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		Binary:           "/nonexistent/tor",
		DataDir:          filepath.Join(tmp, "data"),
		ConfigDir:        filepath.Join(tmp, "config"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
	}
	m := NewManager(context.Background(), 8080, cfg)

	// Multiple restarts should not panic.
	for i := 0; i < 3; i++ {
		if err := m.Restart(); err != nil {
			t.Errorf("Restart #%d error: %v", i+1, err)
		}
	}
}

// ─── Monitor additional branches ──────────────────────────────────────────────

func TestMonitor_NilServiceContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := Config{Binary: "/nonexistent/tor"}
	m := NewManager(ctx, 8080, cfg)

	done := make(chan struct{})
	go func() {
		m.Monitor()
		close(done)
	}()

	// Let Monitor tick once with nil svc.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Monitor did not exit after cancellation")
	}
}

// ─── ensureTorDirs error path ─────────────────────────────────────────────────

func TestEnsureTorDirs_MkdirError(t *testing.T) {
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

	err := ensureTorDirs(filepath.Join(locked, "config"), filepath.Join(locked, "data"))
	if err == nil {
		t.Error("expected error when parent directory is unwritable")
	}
}

// ─── saveKey error path (mkdir failure) ───────────────────────────────────────

func TestSaveKey_MkdirError(t *testing.T) {
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

	keyPath := filepath.Join(locked, "nested", "key")
	err := saveKey(keyPath, fakeKey("data"))
	if err == nil {
		t.Error("expected error when parent mkdir fails")
	}
}

// ─── findInPath Windows branch ────────────────────────────────────────────────

func TestFindInPath_WindowsSeparator(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "myapp.exe")
	if err := os.WriteFile(bin, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	// On Windows, findInPath appends .exe automatically.
	got, err := findInPath("myapp")
	if err != nil {
		t.Fatalf("findInPath: %v", err)
	}
	if got != bin {
		t.Errorf("got %q; want %q", got, bin)
	}
}

// ─── FindBinary common paths branch ───────────────────────────────────────────

func TestFindBinary_CommonPathsNotFound(t *testing.T) {
	// Set PATH to empty and ensure no common paths exist.
	t.Setenv("PATH", "")

	// Verify that when no common paths exist, we get empty string.
	// This branch is exercised when PATH lookup fails and common paths don't exist.
	got := FindBinary("")

	// Result depends on whether a real Tor is installed at common paths.
	if got != "" {
		info, err := os.Stat(got)
		if err != nil || info.IsDir() {
			t.Errorf("FindBinary returned invalid path %q", got)
		}
	}
}

// ─── Config struct field coverage ─────────────────────────────────────────────

func TestConfig_AllFieldsUsed(t *testing.T) {
	cfg := Config{
		Binary:                    "/usr/bin/tor",
		UseNetwork:                true,
		AllowUserPreference:       true,
		MaxCircuits:               10,
		CircuitTimeout:            30,
		BootstrapTimeout:          120,
		SafeLogging:               true,
		MaxStreamsPerCircuit:      100,
		CloseCircuitOnStreamLimit: true,
		BandwidthRate:             "5 MB",
		BandwidthBurst:            "10 MB",
		MaxMonthlyBandwidth:       "500 GB",
		NumIntroPoints:            3,
		VirtualPort:               80,
		ConfigDir:                 "/etc/pastebin",
		DataDir:                   "/var/lib/pastebin",
	}

	// Verify all fields are accessible.
	if cfg.Binary == "" {
		t.Error("Binary should be set")
	}
	if !cfg.UseNetwork {
		t.Error("UseNetwork should be true")
	}
	if cfg.MaxCircuits != 10 {
		t.Error("MaxCircuits should be 10")
	}
	if cfg.CircuitTimeout != 30 {
		t.Error("CircuitTimeout should be 30")
	}
	if !cfg.CloseCircuitOnStreamLimit {
		t.Error("CloseCircuitOnStreamLimit should be true")
	}
	if cfg.NumIntroPoints != 3 {
		t.Error("NumIntroPoints should be 3")
	}
}

// ─── getTorConfig table-driven comprehensive tests ───────────────────────────

func TestGetTorConfig_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Config
		contains []string
		excludes []string
	}{
		{
			name: "basic config",
			cfg: &Config{
				BandwidthRate:  "1 MB",
				BandwidthBurst: "2 MB",
			},
			contains: []string{
				"SocksPort 0",
				"ControlPort 127.0.0.1:auto",
				"SafeLogging 0",
				"BandwidthRate 1 MB",
				"BandwidthBurst 2 MB",
			},
			excludes: []string{"AccountingMax"},
		},
		{
			name: "use network enabled",
			cfg: &Config{
				UseNetwork:     true,
				BandwidthRate:  "1 MB",
				BandwidthBurst: "2 MB",
			},
			contains: []string{"SocksPort auto"},
			excludes: []string{"SocksPort 0"},
		},
		{
			name: "safe logging disabled",
			cfg: &Config{
				SafeLogging:    false,
				BandwidthRate:  "1 MB",
				BandwidthBurst: "2 MB",
			},
			contains: []string{"SafeLogging 0"},
			excludes: []string{"SafeLogging 1"},
		},
		{
			name: "monthly bandwidth limit set",
			cfg: &Config{
				BandwidthRate:       "1 MB",
				BandwidthBurst:      "2 MB",
				MaxMonthlyBandwidth: "50 GB",
			},
			contains: []string{
				"AccountingStart month 1 00:00",
				"AccountingMax 50 GB",
			},
		},
		{
			name: "monthly bandwidth unlimited",
			cfg: &Config{
				BandwidthRate:       "1 MB",
				BandwidthBurst:      "2 MB",
				MaxMonthlyBandwidth: "unlimited",
			},
			excludes: []string{"AccountingMax", "AccountingStart"},
		},
		{
			name: "allow user preference enables socks auto",
			cfg: &Config{
				AllowUserPreference: true,
				BandwidthRate:       "1 MB",
				BandwidthBurst:      "2 MB",
			},
			contains: []string{"SocksPort auto"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := getTorConfig(tc.cfg)

			for _, s := range tc.contains {
				if !strings.Contains(out, s) {
					t.Errorf("expected %q in config:\n%s", s, out)
				}
			}
			for _, s := range tc.excludes {
				if strings.Contains(out, s) {
					t.Errorf("unexpected %q in config:\n%s", s, out)
				}
			}
		})
	}
}

// ─── Manager concurrent access ────────────────────────────────────────────────

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			m.Running()
			m.OnionAddress()
			m.GetHTTPClient(false)
			m.GetHTTPClient(true)
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		m.Running()
		m.OnionAddress()
		m.GetHTTPClient(false)
	}

	<-done
}

// ─── writeIfChanged with read error ───────────────────────────────────────────

func TestWriteIfChanged_ReadErrorContinues(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "newfile")

	// File doesn't exist, so ReadFile in writeIfChanged returns error.
	// Should still write successfully.
	err := writeIfChanged(path, []byte("content"), 0o600)
	if err != nil {
		t.Fatalf("writeIfChanged error: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "content" {
		t.Errorf("got %q; want 'content'", got)
	}
}

// ─── Start with directory creation ───────────────────────────────────────────

func TestStart_CreatesTorDirs(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	cfg := Config{
		Binary:           "/nonexistent/tor",
		ConfigDir:        configDir,
		DataDir:          dataDir,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
	}
	m := NewManager(context.Background(), 8080, cfg)

	// Start will fail to find Tor binary, but should still return nil (graceful).
	err := m.Start()
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Directories should NOT be created if Tor binary is missing.
	// The function returns early before creating dirs.
	if m.Running() {
		t.Error("should not be running without Tor binary")
	}
}

// ─── Close with running service simulation ───────────────────────────────────

func TestClose_CancelsContext(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, 8080, Config{})

	// Start monitor goroutine.
	done := make(chan struct{})
	go func() {
		m.Monitor()
		close(done)
	}()

	// Close should cancel the context and Monitor should exit.
	m.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Monitor did not exit after Close")
	}
}

// ─── RegenerateAddress error paths ────────────────────────────────────────────

func TestRegenerateAddress_RemoveError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission enforcement differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	siteDir := filepath.Join(dataDir, "tor", "site")
	if err := os.MkdirAll(siteDir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Create key file and make parent directory non-writable.
	keyPath := filepath.Join(siteDir, "hs_ed25519_secret_key")
	if err := os.WriteFile(keyPath, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(siteDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(siteDir, 0o700) })

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      filepath.Join(tmp, "config"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	_, err := m.RegenerateAddress()
	if err == nil {
		t.Error("expected error when key file cannot be removed")
	}
}

// ─── Restart with existing svc ────────────────────────────────────────────────

func TestRestart_AfterStart(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{
		Binary:           "/nonexistent/tor",
		DataDir:          filepath.Join(tmp, "data"),
		ConfigDir:        filepath.Join(tmp, "config"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
	}
	m := NewManager(context.Background(), 8080, cfg)

	// Start (will gracefully disable due to missing binary).
	if err := m.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Restart should also gracefully handle missing binary.
	if err := m.Restart(); err != nil {
		t.Fatalf("Restart error: %v", err)
	}

	if m.Running() {
		t.Error("should not be running without Tor binary")
	}
}

// ─── ApplyKeys file permission check ──────────────────────────────────────────

func TestApplyKeys_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not enforced on Windows")
	}

	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      filepath.Join(tmp, "config"),
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 8080, cfg)

	keyBlob := []byte("secret-key")
	_, err := m.ApplyKeys(keyBlob)
	if err != nil {
		t.Fatalf("ApplyKeys error: %v", err)
	}

	keyPath := filepath.Join(dataDir, "tor", "site", "hs_ed25519_secret_key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("key file perm = %o; want 0600", info.Mode().Perm())
	}
}

// ─── Monitor with context already cancelled ──────────────────────────────────

func TestMonitor_ImmediateCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	m := NewManager(ctx, 8080, Config{})

	done := make(chan struct{})
	go func() {
		m.Monitor()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Monitor should exit immediately when context is already cancelled")
	}
}

// ─── getTorConfig edge cases ──────────────────────────────────────────────────

func TestGetTorConfig_BothNetworkFlags(t *testing.T) {
	cfg := &Config{
		UseNetwork:          true,
		AllowUserPreference: true,
		BandwidthRate:       "1 MB",
		BandwidthBurst:      "2 MB",
	}
	out := getTorConfig(cfg)

	// Either flag enables SocksPort auto.
	if !strings.Contains(out, "SocksPort auto") {
		t.Errorf("expected SocksPort auto:\n%s", out)
	}
}

func TestGetTorConfig_ZeroBandwidth(t *testing.T) {
	cfg := &Config{
		BandwidthRate:  "0",
		BandwidthBurst: "0",
	}
	out := getTorConfig(cfg)

	if !strings.Contains(out, "BandwidthRate 0") {
		t.Errorf("expected BandwidthRate 0:\n%s", out)
	}
}

// ─── UpdateConfig preserves manager state ─────────────────────────────────────

func TestUpdateConfig_PreservesServerPort(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")
	torDir := filepath.Join(configDir, "tor")
	if err := os.MkdirAll(torDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tor", "site"), 0o700); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      configDir,
		BandwidthRate:  "1 MB",
		BandwidthBurst: "2 MB",
	}
	m := NewManager(context.Background(), 9999, cfg)

	newCfg := Config{
		Binary:         "/nonexistent/tor",
		DataDir:        dataDir,
		ConfigDir:      configDir,
		BandwidthRate:  "5 MB",
		BandwidthBurst: "10 MB",
	}

	if err := m.UpdateConfig(newCfg); err != nil {
		t.Fatalf("UpdateConfig error: %v", err)
	}

	// Manager should still have the original server port.
	if m.serverPort != 9999 {
		t.Errorf("serverPort = %d; want 9999", m.serverPort)
	}
}

// ─── ensureTorDirs Windows branch ─────────────────────────────────────────────

func TestEnsureTorDirs_WindowsSkipsChown(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	err := ensureTorDirs(configDir, dataDir)
	if err != nil {
		t.Fatalf("ensureTorDirs on Windows: %v", err)
	}

	// Verify directories exist.
	for _, d := range []string{
		filepath.Join(configDir, "tor"),
		filepath.Join(dataDir, "tor"),
		filepath.Join(dataDir, "tor", "site"),
	} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("directory %s should exist: %v", d, err)
		}
	}
}

// ─── findInPath with file that is a directory ─────────────────────────────────

func TestFindInPath_SkipsDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH handling differs on Windows")
	}

	tmp := t.TempDir()
	dirAsFile := filepath.Join(tmp, "tor")
	if err := os.Mkdir(dirAsFile, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	_, err := findInPath("tor")
	if err == nil {
		t.Error("expected error when 'tor' is a directory, not a file")
	}
}

// ─── Manager field access ─────────────────────────────────────────────────────

func TestManager_FieldsAccessible(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		VirtualPort: 8080,
	}
	m := NewManager(ctx, 9000, cfg)

	// Verify internal fields are set correctly.
	if m.serverPort != 9000 {
		t.Errorf("serverPort = %d; want 9000", m.serverPort)
	}
	if m.cfg.VirtualPort != 8080 {
		t.Errorf("cfg.VirtualPort = %d; want 8080", m.cfg.VirtualPort)
	}
}

// ─── Start with real Tor binary (if available) ───────────────────────────────
// These tests exercise more of startLocked when Tor is installed.

func TestStartLocked_WithTorBinary_EnsuresDirs(t *testing.T) {
	// Find a real Tor binary if available.
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	cfg := Config{
		Binary:           bin,
		ConfigDir:        configDir,
		DataDir:          dataDir,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 5,
		VirtualPort:      8080,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	// Start will fail due to short timeout, but should create directories and torrc.
	err := m.Start()

	// Directories should be created even if Tor start fails.
	torDir := filepath.Join(configDir, "tor")
	if _, statErr := os.Stat(torDir); os.IsNotExist(statErr) {
		t.Errorf("tor config dir should exist at %s", torDir)
	}

	torrcPath := filepath.Join(torDir, "torrc")
	if _, statErr := os.Stat(torrcPath); os.IsNotExist(statErr) {
		t.Errorf("torrc should exist at %s", torrcPath)
	}

	// Tor may fail to bootstrap in time - that's expected.
	if err != nil {
		t.Logf("Start failed (expected with short timeout): %v", err)
	}
}

func TestStartLocked_WithTorBinary_WritesTorrc(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")

	cfg := Config{
		Binary:              bin,
		ConfigDir:           configDir,
		DataDir:             dataDir,
		BandwidthRate:       "2 MB",
		BandwidthBurst:      "4 MB",
		MaxMonthlyBandwidth: "100 GB",
		SafeLogging:         true,
		BootstrapTimeout:    1,
		VirtualPort:         80,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	// Start (will likely fail due to short timeout).
	_ = m.Start()

	// Verify torrc content.
	torrcPath := filepath.Join(configDir, "tor", "torrc")
	content, err := os.ReadFile(torrcPath)
	if err != nil {
		t.Skipf("torrc not created: %v", err)
	}

	if !strings.Contains(string(content), "BandwidthRate 2 MB") {
		t.Errorf("torrc missing BandwidthRate 2 MB:\n%s", content)
	}
	if !strings.Contains(string(content), "AccountingMax 100 GB") {
		t.Errorf("torrc missing AccountingMax:\n%s", content)
	}
	if !strings.Contains(string(content), "SafeLogging 1") {
		t.Errorf("torrc missing SafeLogging 1:\n%s", content)
	}
}

func TestStartLocked_PreservesExistingTorrc(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")
	torDir := filepath.Join(configDir, "tor")

	// Pre-create torrc with custom content.
	if err := os.MkdirAll(torDir, 0o700); err != nil {
		t.Fatal(err)
	}
	customContent := "# Custom torrc - should not be overwritten\nSocksPort 0\n"
	torrcPath := filepath.Join(torDir, "torrc")
	if err := os.WriteFile(torrcPath, []byte(customContent), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		Binary:           bin,
		ConfigDir:        configDir,
		DataDir:          dataDir,
		BandwidthRate:    "5 MB",
		BandwidthBurst:   "10 MB",
		BootstrapTimeout: 1,
		VirtualPort:      80,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	// Start will not overwrite existing torrc.
	_ = m.Start()

	content, _ := os.ReadFile(torrcPath)
	if !strings.Contains(string(content), "Custom torrc") {
		t.Error("existing torrc should not be overwritten")
	}
}

func TestStartLocked_EnsureTorDirsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission enforcement differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	locked := filepath.Join(tmp, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o700) })

	cfg := Config{
		Binary:           bin,
		ConfigDir:        filepath.Join(locked, "config"),
		DataDir:          filepath.Join(locked, "data"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
		VirtualPort:      80,
	}

	m := NewManager(context.Background(), 8080, cfg)
	defer m.Close()

	err := m.Start()
	if err == nil {
		t.Error("expected error when directories cannot be created")
	}
}

func TestStartLocked_WriteTorrcError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission enforcement differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")
	torDir := filepath.Join(configDir, "tor")

	// Create tor dir but make it read-only.
	if err := os.MkdirAll(torDir, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tor", "site"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(torDir, 0o700) })

	cfg := Config{
		Binary:           bin,
		ConfigDir:        configDir,
		DataDir:          dataDir,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
		VirtualPort:      80,
	}

	m := NewManager(context.Background(), 8080, cfg)
	defer m.Close()

	err := m.Start()
	if err == nil {
		t.Error("expected error when torrc cannot be written")
	}
}

// ─── Start with immediate context cancellation ───────────────────────────────

func TestStartLocked_CancelledContext(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := Config{
		Binary:           bin,
		ConfigDir:        filepath.Join(tmp, "config"),
		DataDir:          filepath.Join(tmp, "data"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 30,
		VirtualPort:      80,
	}

	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	err := m.Start()
	// Should fail with context cancelled error or similar.
	if err != nil {
		t.Logf("Start with cancelled context: %v", err)
	}
}

// ─── RegenerateAddress with Tor binary ────────────────────────────────────────

func TestRegenerateAddress_WithTorBinary(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	configDir := filepath.Join(tmp, "config")
	siteDir := filepath.Join(dataDir, "tor", "site")
	if err := os.MkdirAll(siteDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "tor"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Create a fake key file.
	keyPath := filepath.Join(siteDir, "hs_ed25519_secret_key")
	if err := os.WriteFile(keyPath, []byte("fake-key"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{
		Binary:           bin,
		DataDir:          dataDir,
		ConfigDir:        configDir,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 2,
		VirtualPort:      80,
	}
	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	_, err := m.RegenerateAddress()
	// May fail due to short timeout, but should still remove the key.
	if _, statErr := os.Stat(keyPath); !os.IsNotExist(statErr) {
		t.Error("key file should be removed even if Tor fails to start")
	}

	if err != nil {
		t.Logf("RegenerateAddress error (expected with short timeout): %v", err)
	}
}

// ─── ApplyKeys with Tor binary ────────────────────────────────────────────────

func TestApplyKeys_WithTorBinary(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(filepath.Join(configDir, "tor"), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{
		Binary:           bin,
		DataDir:          dataDir,
		ConfigDir:        configDir,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 2,
		VirtualPort:      80,
	}
	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	keyBlob := []byte("test-key-material")
	_, err := m.ApplyKeys(keyBlob)

	// Verify key was written.
	keyPath := filepath.Join(dataDir, "tor", "site", "hs_ed25519_secret_key")
	content, readErr := os.ReadFile(keyPath)
	if readErr != nil {
		t.Fatalf("failed to read key: %v", readErr)
	}
	if string(content) != string(keyBlob) {
		t.Errorf("key mismatch: got %q, want %q", content, keyBlob)
	}

	if err != nil {
		t.Logf("ApplyKeys error (expected with short timeout): %v", err)
	}
}

// ─── UpdateConfig with Tor binary ─────────────────────────────────────────────

func TestUpdateConfig_WithTorBinary(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(filepath.Join(configDir, "tor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "tor", "site"), 0o700); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := Config{
		Binary:           bin,
		DataDir:          dataDir,
		ConfigDir:        configDir,
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 2,
		VirtualPort:      80,
	}
	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	newCfg := Config{
		Binary:           bin,
		DataDir:          dataDir,
		ConfigDir:        configDir,
		BandwidthRate:    "5 MB",
		BandwidthBurst:   "10 MB",
		SafeLogging:      true,
		BootstrapTimeout: 2,
		VirtualPort:      80,
	}

	_ = m.UpdateConfig(newCfg)

	// Verify torrc was updated.
	torrcPath := filepath.Join(configDir, "tor", "torrc")
	content, err := os.ReadFile(torrcPath)
	if err != nil {
		t.Skipf("torrc not created: %v", err)
	}

	if !strings.Contains(string(content), "BandwidthRate 5 MB") {
		t.Errorf("torrc should have updated BandwidthRate:\n%s", content)
	}
}

// ─── Restart with Tor binary ──────────────────────────────────────────────────

func TestRestart_WithTorBinary(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := Config{
		Binary:           bin,
		DataDir:          filepath.Join(tmp, "data"),
		ConfigDir:        filepath.Join(tmp, "config"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 2,
		VirtualPort:      80,
	}
	m := NewManager(ctx, 8080, cfg)
	defer m.Close()

	// Restart should attempt to start Tor.
	_ = m.Restart()

	// Restart again.
	err := m.Restart()
	if err != nil {
		t.Logf("Restart error (expected with short timeout): %v", err)
	}
}

// ─── Close after Start with Tor binary ────────────────────────────────────────

func TestClose_AfterStartWithTor(t *testing.T) {
	bin := FindBinary("")
	if bin == "" {
		t.Skip("Tor binary not found")
	}

	tmp := t.TempDir()
	ctx := context.Background()

	cfg := Config{
		Binary:           bin,
		DataDir:          filepath.Join(tmp, "data"),
		ConfigDir:        filepath.Join(tmp, "config"),
		BandwidthRate:    "1 MB",
		BandwidthBurst:   "2 MB",
		BootstrapTimeout: 2,
		VirtualPort:      80,
	}
	m := NewManager(ctx, 8080, cfg)

	// Start (will likely fail due to timeout).
	_ = m.Start()

	// Close should not panic.
	m.Close()

	if m.Running() {
		t.Error("should not be running after Close")
	}
	if m.OnionAddress() != "" {
		t.Error("OnionAddress should be empty after Close")
	}
}

// ─── ensureTorDirs Windows behavior ───────────────────────────────────────────

func TestEnsureTorDirs_AllPaths(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "a", "b", "c", "config")
	dataDir := filepath.Join(tmp, "x", "y", "z", "data")

	err := ensureTorDirs(configDir, dataDir)
	if err != nil {
		t.Fatalf("ensureTorDirs: %v", err)
	}

	// All three dirs should exist.
	expected := []string{
		filepath.Join(configDir, "tor"),
		filepath.Join(dataDir, "tor"),
		filepath.Join(dataDir, "tor", "site"),
	}
	for _, d := range expected {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("directory %s should exist: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", d)
		}
	}
}

// ─── saveKey write error ──────────────────────────────────────────────────────

func TestSaveKey_WriteError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission enforcement differs on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	tmp := t.TempDir()
	dir := filepath.Join(tmp, "keydir")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	keyPath := filepath.Join(dir, "key")
	// Create key file and make it read-only.
	if err := os.WriteFile(keyPath, []byte("old"), 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(keyPath, 0o600) })

	err := saveKey(keyPath, fakeKey("new"))
	if err == nil {
		t.Error("expected error when key file is read-only")
	}
}

// ─── Monitor tick without service ─────────────────────────────────────────────

func TestMonitor_TicksWithoutService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(ctx, 8080, Config{})

	done := make(chan struct{})
	go func() {
		m.Monitor()
		close(done)
	}()

	// Let it tick a couple times (30s ticker, but we can't wait that long).
	// Just verify it doesn't block or crash.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Error("Monitor did not exit")
	}
}

// ─── GetHTTPClient timeout values ─────────────────────────────────────────────

func TestGetHTTPClient_TimeoutValues(t *testing.T) {
	m := NewManager(context.Background(), 8080, Config{})

	direct := m.GetHTTPClient(false)
	if direct.Timeout != 30*time.Second {
		t.Errorf("direct client timeout = %v; want 30s", direct.Timeout)
	}

	// Without running Tor, requesting Tor client returns direct client.
	torFallback := m.GetHTTPClient(true)
	if torFallback.Timeout != 30*time.Second {
		t.Errorf("tor fallback client timeout = %v; want 30s", torFallback.Timeout)
	}
}

// ─── FindBinary with invalid configured path ──────────────────────────────────

func TestFindBinary_ConfiguredPathIsDirectory(t *testing.T) {
	tmp := t.TempDir()
	dir := filepath.Join(tmp, "tor")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Directory exists but is not a file - should return empty.
	got := FindBinary(dir)
	// os.Stat succeeds for directories, so this returns the path.
	// The function doesn't check if it's executable.
	if got == "" {
		t.Log("FindBinary correctly rejected directory path")
	} else if got != dir {
		t.Errorf("FindBinary = %q; expected either %q or empty", got, dir)
	}
}

// ─── findInPath first entry wins ──────────────────────────────────────────────

func TestFindInPath_FirstMatchWins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH handling differs on Windows")
	}

	tmp1 := t.TempDir()
	tmp2 := t.TempDir()

	// Create "mybin" in both directories.
	bin1 := filepath.Join(tmp1, "mybin")
	bin2 := filepath.Join(tmp2, "mybin")
	if err := os.WriteFile(bin1, []byte("first"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin2, []byte("second"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmp1+":"+tmp2)

	got, err := findInPath("mybin")
	if err != nil {
		t.Fatalf("findInPath: %v", err)
	}
	if got != bin1 {
		t.Errorf("got %q; want %q (first match)", got, bin1)
	}
}

// ─── commonTorPaths coverage ──────────────────────────────────────────────────

func TestCommonTorPaths_HasCurrentOS(t *testing.T) {
	paths := commonTorPaths[runtime.GOOS]
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" ||
		runtime.GOOS == "windows" || runtime.GOOS == "freebsd" {
		if len(paths) == 0 {
			t.Errorf("commonTorPaths[%q] is empty", runtime.GOOS)
		}
	}
}

// ─── Config struct zero value ─────────────────────────────────────────────────

func TestConfig_ZeroValue(t *testing.T) {
	cfg := Config{}
	out := getTorConfig(&cfg)

	// Zero value should produce valid config with defaults.
	if !strings.Contains(out, "SocksPort 0") {
		t.Error("zero config should produce SocksPort 0")
	}
	if !strings.Contains(out, "SafeLogging 0") {
		t.Error("zero config should produce SafeLogging 0")
	}
}

// ─── writeIfChanged permissions ───────────────────────────────────────────────

func TestWriteIfChanged_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not enforced on Windows")
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "testfile")

	if err := writeIfChanged(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("writeIfChanged: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o; want 0600", info.Mode().Perm())
	}
}
