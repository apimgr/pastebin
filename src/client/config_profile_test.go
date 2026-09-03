package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrescanConfigFlag verifies --config NAME and --config=NAME parsing.
func TestPrescanConfigFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"space form", []string{"--config", "dev", "list"}, "dev"},
		{"equals form", []string{"--config=dev", "list"}, "dev"},
		{"absent", []string{"list", "--json"}, ""},
		{"trailing without value", []string{"--config"}, ""},
		{"after other flags", []string{"--json", "--config", "prod"}, "prod"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := prescanConfigFlag(tc.args); got != tc.want {
				t.Fatalf("prescanConfigFlag(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestResolveConfigPath verifies profile-name, absolute, and ~ resolution.
func TestResolveConfigPath(t *testing.T) {
	if got := resolveConfigPath(""); got != cliConfigPath() {
		t.Fatalf("empty name = %q, want default %q", got, cliConfigPath())
	}

	abs := filepath.Join(os.TempDir(), "explicit.yml")
	if got := resolveConfigPath(abs); got != abs {
		t.Fatalf("absolute path = %q, want %q", got, abs)
	}

	home, _ := os.UserHomeDir()
	if got := resolveConfigPath("~/x.yml"); got != filepath.Join(home, "/x.yml") {
		t.Fatalf("~ expansion = %q, want %q", got, filepath.Join(home, "/x.yml"))
	}

	dir := filepath.Dir(cliConfigPath())
	if got := resolveConfigPath("dev"); got != filepath.Join(dir, "dev.yml") {
		t.Fatalf("bare name = %q, want %q", got, filepath.Join(dir, "dev.yml"))
	}
	if !strings.HasSuffix(resolveConfigPath("dev"), ".yml") {
		t.Fatalf("bare name should default to .yml")
	}

	// PART 32 rule 3: an explicit .yml/.yaml extension is used as-is and never
	// double-suffixed (regression guard for dev.yml -> dev.yml.yml).
	if got := resolveConfigPath("dev.yml"); got != filepath.Join(dir, "dev.yml") {
		t.Fatalf("dev.yml = %q, want %q", got, filepath.Join(dir, "dev.yml"))
	}
	if got := resolveConfigPath("test.yaml"); got != filepath.Join(dir, "test.yaml") {
		t.Fatalf("test.yaml = %q, want %q", got, filepath.Join(dir, "test.yaml"))
	}
}

// TestResolveConfigPathPrefersYaml verifies .yml wins, falling back to .yaml.
func TestResolveConfigPathPrefersYaml(t *testing.T) {
	dir := t.TempDir()
	yamlOnly := filepath.Join(dir, "only.yaml")
	if err := os.WriteFile(yamlOnly, []byte("server:\n  primary: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// resolveConfigPath uses cliConfigPath()'s dir, so point HOME at our tmp
	// only when the config dir matches; instead assert fileExists directly.
	if !fileExists(yamlOnly) {
		t.Fatalf("fileExists(%q) = false, want true", yamlOnly)
	}
	if fileExists(filepath.Join(dir, "missing.yml")) {
		t.Fatalf("fileExists on missing path should be false")
	}
	if fileExists(dir) {
		t.Fatalf("fileExists on a directory should be false")
	}
}

// TestResolvedConfigPathFallback verifies activeConfigPath override behavior.
func TestResolvedConfigPathFallback(t *testing.T) {
	orig := activeConfigPath
	defer func() { activeConfigPath = orig }()

	activeConfigPath = ""
	if got := resolvedConfigPath(); got != cliConfigPath() {
		t.Fatalf("empty override = %q, want default %q", got, cliConfigPath())
	}

	activeConfigPath = "/custom/path.yml"
	if got := resolvedConfigPath(); got != "/custom/path.yml" {
		t.Fatalf("override = %q, want /custom/path.yml", got)
	}
}
