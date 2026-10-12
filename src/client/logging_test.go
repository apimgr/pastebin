package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseByteSize(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  int64
	}{
		{"10MB", 10 * 1024 * 1024},
		{"100 MiB", 100 * 1024 * 1024},
		{"4KB", 4096},
		{"512B", 512},
	} {
		got, err := parseByteSize(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("parseByteSize(%q) = %d, %v; want %d", tc.input, got, err, tc.want)
		}
	}
	for _, input := range []string{"", "-1MB", "bogus", "99999999999999999999GB"} {
		if _, err := parseByteSize(input); err == nil {
			t.Errorf("parseByteSize(%q) unexpectedly succeeded", input)
		}
	}
}

func TestRotateLogKeepsConfiguredFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cli.log")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".1", []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	rotateLog(path, 4, 2)
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("new rotated log missing: %v", err)
	}
	if _, err := os.Stat(path + ".2"); err != nil {
		t.Fatalf("older rotated log missing: %v", err)
	}
}

func TestInitializeLoggingUsesUserOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not available on Windows")
	}
	root := t.TempDir()
	path := filepath.Join(root, "logs", "cli.log")
	var cfg cliConfig
	cfg.Logging.File = path
	cfg.Logging.Level = "warn"
	cfg.Logging.MaxSize = "1MB"
	cfg.Logging.MaxFiles = 2
	oldWriter, oldFlags, oldPrefix := log.Writer(), log.Flags(), log.Prefix()
	defer func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	}()

	initializeLogging(cfg)

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat log directory: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("log directory permissions = %04o, want 0700", got)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("log file permissions = %04o, want 0600", got)
	}
}
