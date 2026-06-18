package task_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/task"
)

// ─── BlocklistUpdate ──────────────────────────────────────────────────────────

func TestBlocklistUpdate_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	fn := task.BlocklistUpdate(dir)
	if err := fn(); err != nil {
		t.Fatalf("BlocklistUpdate error: %v", err)
	}
	expected := filepath.Join(dir, "security", "blocklists")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected directory %s to exist: %v", expected, err)
	}
}

func TestBlocklistUpdate_CountsFiles(t *testing.T) {
	dir := t.TempDir()
	blDir := filepath.Join(dir, "security", "blocklists")
	if err := os.MkdirAll(blDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// Create 2 dummy files.
	for _, n := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(blDir, n), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fn := task.BlocklistUpdate(dir)
	if err := fn(); err != nil {
		t.Fatalf("BlocklistUpdate error: %v", err)
	}
}

// ─── CVEUpdate ────────────────────────────────────────────────────────────────

func TestCVEUpdate_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	fn := task.CVEUpdate(dir)
	if err := fn(); err != nil {
		t.Fatalf("CVEUpdate error: %v", err)
	}
	expected := filepath.Join(dir, "security", "cve")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected directory %s to exist: %v", expected, err)
	}
}

// ─── LogRotation ─────────────────────────────────────────────────────────────

func TestLogRotation_CompressesOldLogs(t *testing.T) {
	dir := t.TempDir()

	// Create a .log file with a very old mtime.
	logPath := filepath.Join(dir, "app.log")
	if err := os.WriteFile(logPath, []byte("log content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Set mtime to 60 days ago.
	old := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(logPath, old, old); err != nil {
		t.Fatal(err)
	}

	fn := task.LogRotation(dir, 30*24*time.Hour)
	if err := fn(); err != nil {
		t.Fatalf("LogRotation error: %v", err)
	}

	// Source .log should be gone, .log.gz should exist.
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("original .log file should have been compressed and removed")
	}
	if _, err := os.Stat(logPath + ".gz"); err != nil {
		t.Errorf("compressed .log.gz should exist: %v", err)
	}
}

func TestLogRotation_DeletesOldGzFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a .log.gz that is > 3×maxAge old.
	gzPath := filepath.Join(dir, "old.log.gz")
	if err := os.WriteFile(gzPath, []byte("gz"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-100 * 24 * time.Hour) // 100 days, 3×30 = 90
	if err := os.Chtimes(gzPath, old, old); err != nil {
		t.Fatal(err)
	}

	fn := task.LogRotation(dir, 30*24*time.Hour)
	if err := fn(); err != nil {
		t.Fatalf("LogRotation error: %v", err)
	}

	if _, err := os.Stat(gzPath); !os.IsNotExist(err) {
		t.Errorf("old .log.gz should have been deleted")
	}
}

func TestLogRotation_SkipsRecentLogs(t *testing.T) {
	dir := t.TempDir()

	// A recent .log file should be left alone.
	logPath := filepath.Join(dir, "recent.log")
	if err := os.WriteFile(logPath, []byte("recent\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fn := task.LogRotation(dir, 30*24*time.Hour)
	if err := fn(); err != nil {
		t.Fatalf("LogRotation error: %v", err)
	}

	if _, err := os.Stat(logPath); err != nil {
		t.Errorf("recent .log file should still exist: %v", err)
	}
}

func TestLogRotation_DefaultMaxAge(t *testing.T) {
	dir := t.TempDir()
	fn := task.LogRotation(dir, 0) // 0 = use default (30 days)
	if err := fn(); err != nil {
		t.Fatalf("LogRotation with default maxAge error: %v", err)
	}
}

func TestLogRotation_NonexistentDir(t *testing.T) {
	fn := task.LogRotation("/nonexistent/logs", 24*time.Hour)
	// WalkDir on nonexistent dir should return an error.
	err := fn()
	// LogRotation wraps the walk error; it should be non-nil.
	if err == nil {
		t.Error("expected error for nonexistent logs dir, got nil")
	}
}

// ─── SSLRenewal ───────────────────────────────────────────────────────────────

func TestSSLRenewal_NoDir(t *testing.T) {
	// When certRoot does not exist, SSLRenewal returns nil (graceful no-op).
	dir := t.TempDir()
	fn := task.SSLRenewal(dir, "example.com")
	if err := fn(); err != nil {
		t.Fatalf("SSLRenewal with no cert dir should return nil, got: %v", err)
	}
}

// ─── TorHealth ────────────────────────────────────────────────────────────────

func TestTorHealth_NilFunc(t *testing.T) {
	fn := task.TorHealth(nil)
	if err := fn(); err != nil {
		t.Fatalf("TorHealth(nil) should return nil, got: %v", err)
	}
}

func TestTorHealth_Running(t *testing.T) {
	fn := task.TorHealth(func() bool { return true })
	if err := fn(); err != nil {
		t.Fatalf("TorHealth running=true should return nil, got: %v", err)
	}
}

func TestTorHealth_NotRunning(t *testing.T) {
	fn := task.TorHealth(func() bool { return false })
	if err := fn(); err != nil {
		t.Fatalf("TorHealth running=false should return nil, got: %v", err)
	}
}

// ─── BackupDaily ──────────────────────────────────────────────────────────────

func TestBackupDaily_CreatesBackup(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	dataDir := filepath.Join(root, "data")
	bkpDir := filepath.Join(root, "backup")
	for _, d := range []string{cfgDir, filepath.Join(dataDir, "db"), bkpDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "server.yml"), []byte("mode: production\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := task.BackupConfig{
		ProjectName: "pastebin",
		ConfigDir:   cfgDir,
		DataDir:     dataDir,
		BackupDir:   bkpDir,
		AppVersion:  "v1.0.0",
		Retention:   task.BackupRetention{MaxBackups: 3},
	}
	fn := task.BackupDaily(cfg)
	if err := fn(); err != nil {
		t.Fatalf("BackupDaily error: %v", err)
	}

	entries, err := os.ReadDir(bkpDir)
	if err != nil {
		t.Fatal(err)
	}
	// Expect at least one dated backup file.
	if len(entries) == 0 {
		t.Error("BackupDaily: no backup files created")
	}
}

// ─── BackupHourly ─────────────────────────────────────────────────────────────

func TestBackupHourly_CreatesRollingBackup(t *testing.T) {
	root := t.TempDir()
	cfgDir := filepath.Join(root, "config")
	dataDir := filepath.Join(root, "data")
	bkpDir := filepath.Join(root, "backup")
	for _, d := range []string{cfgDir, filepath.Join(dataDir, "db"), bkpDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "server.yml"), []byte("mode: production\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := task.BackupConfig{
		ProjectName: "pastebin",
		ConfigDir:   cfgDir,
		DataDir:     dataDir,
		BackupDir:   bkpDir,
		AppVersion:  "v1.0.0",
	}
	fn := task.BackupHourly(cfg)
	if err := fn(); err != nil {
		t.Fatalf("BackupHourly error: %v", err)
	}

	// The hourly backup file should exist.
	hourlyPath := filepath.Join(bkpDir, "pastebin-hourly.tar.gz")
	if _, err := os.Stat(hourlyPath); err != nil {
		t.Errorf("expected hourly backup at %s: %v", hourlyPath, err)
	}
}
