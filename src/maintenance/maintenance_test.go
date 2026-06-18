package maintenance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/pastebin/src/maintenance"
)

// makeTestDirs creates temp config and data directories with dummy files.
func makeTestDirs(t *testing.T) (configDir, dataDir, backupDir string) {
	t.Helper()
	root := t.TempDir()
	configDir = filepath.Join(root, "config")
	dataDir = filepath.Join(root, "data")
	backupDir = filepath.Join(root, "backup")
	for _, d := range []string{
		configDir,
		filepath.Join(dataDir, "db"),
		backupDir,
	} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// Write a dummy server.yml.
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"),
		[]byte("mode: production\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return configDir, dataDir, backupDir
}

// ─── Backup + VerifyBackup ────────────────────────────────────────────────────

func TestBackupAndVerify_NoEncryption(t *testing.T) {
	cfgDir, dataDir, bkpDir := makeTestDirs(t)

	opts := maintenance.BackupOptions{
		ConfigDir:  cfgDir,
		DataDir:    dataDir,
		BackupDir:  bkpDir,
		AppVersion: "v1.0.0",
	}
	if err := maintenance.Backup(opts); err != nil {
		t.Fatalf("Backup error: %v", err)
	}

	// Find the created backup file.
	entries, err := os.ReadDir(bkpDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no backup file created: %v", err)
	}
	bkpPath := filepath.Join(bkpDir, entries[0].Name())

	if err := maintenance.VerifyBackup(bkpPath, ""); err != nil {
		t.Fatalf("VerifyBackup error: %v", err)
	}
}

func TestBackupAndVerify_WithEncryption(t *testing.T) {
	cfgDir, dataDir, bkpDir := makeTestDirs(t)

	opts := maintenance.BackupOptions{
		ConfigDir:  cfgDir,
		DataDir:    dataDir,
		BackupDir:  bkpDir,
		AppVersion: "v1.0.0",
		Password:   "s3cur3p@ssw0rd",
	}
	if err := maintenance.Backup(opts); err != nil {
		t.Fatalf("Backup error: %v", err)
	}

	entries, err := os.ReadDir(bkpDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("no backup file created: %v", err)
	}
	bkpPath := filepath.Join(bkpDir, entries[0].Name())

	// Verify with correct password.
	if err := maintenance.VerifyBackup(bkpPath, "s3cur3p@ssw0rd"); err != nil {
		t.Fatalf("VerifyBackup (correct password) error: %v", err)
	}
}

func TestBackupAndVerify_WrongPassword(t *testing.T) {
	cfgDir, dataDir, bkpDir := makeTestDirs(t)

	opts := maintenance.BackupOptions{
		ConfigDir:  cfgDir,
		DataDir:    dataDir,
		BackupDir:  bkpDir,
		AppVersion: "v1.0.0",
		Password:   "correct",
	}
	if err := maintenance.Backup(opts); err != nil {
		t.Fatalf("Backup error: %v", err)
	}

	entries, _ := os.ReadDir(bkpDir)
	bkpPath := filepath.Join(bkpDir, entries[0].Name())

	// Verify with wrong password should fail.
	if err := maintenance.VerifyBackup(bkpPath, "wrong"); err == nil {
		t.Error("expected error with wrong password, got nil")
	}
}

func TestBackupAndVerify_CustomFilename(t *testing.T) {
	cfgDir, dataDir, bkpDir := makeTestDirs(t)

	opts := maintenance.BackupOptions{
		ConfigDir:  cfgDir,
		DataDir:    dataDir,
		BackupDir:  bkpDir,
		AppVersion: "v1.0.0",
		Filename:   "custom_backup.tar.gz",
	}
	if err := maintenance.Backup(opts); err != nil {
		t.Fatalf("Backup error: %v", err)
	}

	bkpPath := filepath.Join(bkpDir, "custom_backup.tar.gz")
	if _, err := os.Stat(bkpPath); err != nil {
		t.Fatalf("expected custom backup at %s: %v", bkpPath, err)
	}

	if err := maintenance.VerifyBackup(bkpPath, ""); err != nil {
		t.Fatalf("VerifyBackup error: %v", err)
	}
}

func TestVerifyBackup_Nonexistent(t *testing.T) {
	if err := maintenance.VerifyBackup("/nonexistent/backup.tar.gz", ""); err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestVerifyBackup_EncryptedRequiresPassword(t *testing.T) {
	cfgDir, dataDir, bkpDir := makeTestDirs(t)

	opts := maintenance.BackupOptions{
		ConfigDir: cfgDir, DataDir: dataDir, BackupDir: bkpDir,
		AppVersion: "v1.0.0", Password: "secret",
	}
	if err := maintenance.Backup(opts); err != nil {
		t.Fatalf("Backup error: %v", err)
	}

	entries, _ := os.ReadDir(bkpDir)
	bkpPath := filepath.Join(bkpDir, entries[0].Name())

	// Verify without password on an .enc file should fail.
	if err := maintenance.VerifyBackup(bkpPath, ""); err == nil {
		t.Error("expected error verifying encrypted backup without password, got nil")
	}
}

// ─── Restore ─────────────────────────────────────────────────────────────────

func TestRestoreRoundTrip(t *testing.T) {
	cfgDir, dataDir, bkpDir := makeTestDirs(t)

	opts := maintenance.BackupOptions{
		ConfigDir: cfgDir, DataDir: dataDir, BackupDir: bkpDir,
		AppVersion: "v1.0.0",
	}
	if err := maintenance.Backup(opts); err != nil {
		t.Fatalf("Backup error: %v", err)
	}
	entries, _ := os.ReadDir(bkpDir)
	bkpPath := filepath.Join(bkpDir, entries[0].Name())

	// Restore to new directories.
	restCfg := t.TempDir()
	restData := t.TempDir()
	if err := maintenance.Restore(bkpPath, restCfg, restData, ""); err != nil {
		t.Fatalf("Restore error: %v", err)
	}

	// server.yml should have been restored.
	if _, err := os.Stat(filepath.Join(restCfg, "server.yml")); err != nil {
		t.Errorf("server.yml not restored: %v", err)
	}
}

// ─── SetMode ─────────────────────────────────────────────────────────────────

func TestSetMode(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(cfgPath, []byte("mode: production\nport: 8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := maintenance.SetMode(dir, "development"); err != nil {
		t.Fatalf("SetMode error: %v", err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mode: development") {
		t.Errorf("expected mode to be development, got: %s", string(data))
	}
}

func TestSetMode_NonexistentConfig(t *testing.T) {
	if err := maintenance.SetMode("/nonexistent/dir", "development"); err == nil {
		t.Error("expected error for nonexistent config dir, got nil")
	}
}

// ─── Setup ───────────────────────────────────────────────────────────────────

func TestSetup(t *testing.T) {
	dir := t.TempDir()
	if err := maintenance.Setup(dir); err != nil {
		t.Fatalf("Setup error: %v", err)
	}
}

// ─── PrintHelp ───────────────────────────────────────────────────────────────

func TestPrintHelp(t *testing.T) {
	// Should not panic.
	maintenance.PrintHelp("pastebin")
}
