package maintenance

// PART 21 restore hardening: extractEntry must refuse to write through a
// pre-existing symlink, whether the symlink is the destination file itself
// or one of its parent directory components. A crafted backup that could
// plant either one would otherwise escape the destination base directory,
// because the containment check in extractEntry is purely lexical.

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func buildSingleEntryArchive(t *testing.T, name, body string) (*tar.Reader, *tar.Header) {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	payload := []byte(body)
	if err := tw.WriteHeader(&tar.Header{
		Name: name, Mode: 0o600, Size: int64(len(payload)), Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	return tr, hdr
}

func TestExtractEntryRefusesSymlinkedDestination(t *testing.T) {
	cfgDir := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("ORIGINAL"), 0o600); err != nil {
		t.Fatal(err)
	}

	themeDir := filepath.Join(cfgDir, "theme")
	if err := os.MkdirAll(themeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(themeDir, "evil.css")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	tr, hdr := buildSingleEntryArchive(t, "theme/evil.css", "PWNED")
	if err := extractEntry(tr, hdr, cfgDir, t.TempDir()); err == nil {
		t.Error("extractEntry accepted a symlinked destination")
	}

	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "PWNED") {
		t.Errorf("wrote outside configDir: %s contains %q", victim, got)
	}
}

func TestExtractEntryRefusesSymlinkedParentDir(t *testing.T) {
	cfgDir := t.TempDir()
	outside := t.TempDir()
	planted := filepath.Join(outside, "planted")
	if err := os.MkdirAll(planted, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(planted, filepath.Join(cfgDir, "theme")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	tr, hdr := buildSingleEntryArchive(t, "theme/x.css", "PWNED")
	if err := extractEntry(tr, hdr, cfgDir, t.TempDir()); err == nil {
		t.Error("extractEntry accepted a symlinked parent directory")
	}

	if _, err := os.Stat(filepath.Join(planted, "x.css")); err == nil {
		t.Errorf("wrote through parent symlink into %s", planted)
	}
}

func TestExtractEntryAllowsCleanExtraction(t *testing.T) {
	cfgDir := t.TempDir()

	tr, hdr := buildSingleEntryArchive(t, "theme/ok.css", "body{}")
	if err := extractEntry(tr, hdr, cfgDir, t.TempDir()); err != nil {
		t.Fatalf("clean extraction rejected: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(cfgDir, "theme", "ok.css"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "body{}" {
		t.Errorf("content mismatch: got %q", got)
	}
	info, err := os.Stat(filepath.Join(cfgDir, "theme", "ok.css"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("restored mode = %o, want 600", info.Mode().Perm())
	}
}
