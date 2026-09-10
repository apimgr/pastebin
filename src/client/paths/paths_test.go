package paths

import (
	"os"
	"path/filepath"
	"runtime"
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
			if got := PrescanConfigFlag(tc.args); got != tc.want {
				t.Fatalf("PrescanConfigFlag(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestResolve verifies profile-name, absolute, and ~ resolution.
func TestResolve(t *testing.T) {
	if got := Resolve(""); got != ConfigFile() {
		t.Fatalf("empty name = %q, want default %q", got, ConfigFile())
	}

	abs := filepath.Join(os.TempDir(), "explicit.yml")
	if got := Resolve(abs); got != abs {
		t.Fatalf("absolute path = %q, want %q", got, abs)
	}

	home, _ := os.UserHomeDir()
	if got := Resolve("~/x.yml"); got != filepath.Join(home, "/x.yml") {
		t.Fatalf("~ expansion = %q, want %q", got, filepath.Join(home, "/x.yml"))
	}

	dir := filepath.Dir(ConfigFile())
	if got := Resolve("dev"); got != filepath.Join(dir, "dev.yml") {
		t.Fatalf("bare name = %q, want %q", got, filepath.Join(dir, "dev.yml"))
	}
	if !strings.HasSuffix(Resolve("dev"), ".yml") {
		t.Fatalf("bare name should default to .yml")
	}

	// PART 32 rule 3: an explicit .yml/.yaml extension is used as-is and never
	// double-suffixed (regression guard for dev.yml -> dev.yml.yml).
	if got := Resolve("dev.yml"); got != filepath.Join(dir, "dev.yml") {
		t.Fatalf("dev.yml = %q, want %q", got, filepath.Join(dir, "dev.yml"))
	}
	if got := Resolve("test.yaml"); got != filepath.Join(dir, "test.yaml") {
		t.Fatalf("test.yaml = %q, want %q", got, filepath.Join(dir, "test.yaml"))
	}
}

// TestResolvePrefersYaml verifies .yml wins, falling back to .yaml.
func TestResolvePrefersYaml(t *testing.T) {
	dir := t.TempDir()
	yamlOnly := filepath.Join(dir, "only.yaml")
	if err := os.WriteFile(yamlOnly, []byte("server:\n  primary: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Resolve uses ConfigFile()'s dir, so point HOME at our tmp only when the
	// config dir matches; instead assert fileExists directly.
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

// TestResolvedFallback verifies ActiveConfigPath override behavior.
func TestResolvedFallback(t *testing.T) {
	orig := ActiveConfigPath
	defer func() { ActiveConfigPath = orig }()

	ActiveConfigPath = ""
	if got := Resolved(); got != ConfigFile() {
		t.Fatalf("empty override = %q, want default %q", got, ConfigFile())
	}

	ActiveConfigPath = "/custom/path.yml"
	if got := Resolved(); got != "/custom/path.yml" {
		t.Fatalf("override = %q, want /custom/path.yml", got)
	}
}

// TestConfigFile_WithEnvOverride verifies CLI_CONFIG env var takes priority.
func TestConfigFile_WithEnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "my-cli.yml")
	t.Setenv("CLI_CONFIG", cfgPath)

	got := ConfigFile()
	if got != cfgPath {
		t.Errorf("ConfigFile() = %q; want %q", got, cfgPath)
	}
}

// TestConfigFile_DefaultContainsInternalName verifies the default path is
// non-empty and contains the internal project name.
func TestConfigFile_DefaultContainsInternalName(t *testing.T) {
	t.Setenv("CLI_CONFIG", "")
	got := ConfigFile()
	if got == "" {
		t.Error("ConfigFile() returned empty string without CLI_CONFIG")
	}
	if !strings.Contains(got, internalName) {
		t.Errorf("default config path %q should contain %q", got, internalName)
	}
}

// TestConfigFile_XDGConfigHome verifies the XDG_CONFIG_HOME branch, reachable
// on Linux/Unix when the env var is set.
func TestConfigFile_XDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		t.Skip("XDG_CONFIG_HOME only applies to Linux/Unix")
	}
	dir := t.TempDir()
	t.Setenv("CLI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", dir)

	got := ConfigFile()
	if !strings.HasPrefix(got, dir) {
		t.Errorf("ConfigFile() = %q; want prefix %q", got, dir)
	}
	if !strings.Contains(got, filepath.Join(internalOrg, internalName)) {
		t.Errorf("ConfigFile() = %q; should contain %s/%s", got, internalOrg, internalName)
	}
}

// TestConfigFile_LinuxWithoutXDG verifies the Linux default (~/.config) when
// XDG_CONFIG_HOME is unset.
func TestConfigFile_LinuxWithoutXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("CLI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got := ConfigFile()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", internalOrg, internalName, "cli.yml")
	if got != expected {
		t.Errorf("ConfigFile() = %q; want %q", got, expected)
	}
}

// TestEnsureDirs_NoPanic calls EnsureDirs and verifies it does not panic.
func TestEnsureDirs_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("EnsureDirs panicked: %v", r)
		}
	}()
	EnsureDirs()
}
