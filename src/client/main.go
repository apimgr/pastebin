// pastebin-cli — command-line client for the pastebin service.
//
// Usage:
//
//	pastebin-cli [--server URL] [--json] <command> [args]
//
// Commands:
//
//	create [file]             Create paste from stdin or file
//	get <id>                  Fetch raw paste content
//	delete <id> <token>       Delete paste using delete token
//	list [--limit N]          List recent public pastes
//	update                    Check for and apply CLI updates
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/apimgr/pastebin/src/client/tui"
	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/shell"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Version, CommitID, BuildEpoch, and OfficialSite are injected at build time via
// -ldflags. BuildDate is NOT an ldflag itself — it is derived from BuildEpoch in
// init(), matching the server binary's pattern (see src/main.go).
var (
	Version  = "dev"
	CommitID = "unknown"
	// BuildDate is derived from BuildEpoch in init(); stays "unknown" when BuildEpoch is unset.
	BuildDate = "unknown"
	// BuildEpoch is the Unix build timestamp (seconds, UTC) set via -ldflags; "0" when unset.
	BuildEpoch   = "0"
	OfficialSite = ""
)

// init derives BuildDate (RFC 3339 UTC) from the embedded BuildEpoch.
func init() {
	if n, err := strconv.ParseInt(BuildEpoch, 10, 64); err == nil && n > 0 {
		BuildDate = time.Unix(n, 0).UTC().Format("2006-01-02T15:04:05Z")
	}
}

// projectName is the hardcoded internal name used for User-Agent and config paths.
// Display uses filepath.Base(os.Args[0]) per PART 32.
const projectName = "pastebin"

// apiVersion is the {api_version} route segment (PART 14) this CLI targets.
// Centralized here rather than scattered per call site; matches the server's
// default (config.DefaultAPIVersion) since the CLI is compiled against the
// same project's API surface.
const apiVersion = "v1"

// Exit codes per PART 32.
const (
	exitSuccess    = 0
	exitGeneral    = 1
	exitConfig     = 2
	exitConnection = 3
	exitAuth       = 4
	exitNotFound   = 5
	exitUsage      = 64
)

// printConnectionError prints the PART 32 "Error Messages" connection-error
// format (multi-line: reason + two hints) and exits exitConnection. serverURL
// is the resolved --server value shown in the message; detail is the
// underlying error, appended so operators keep diagnostic detail the AI.md
// example omits but every other error path in this CLI includes.
func printConnectionError(serverURL string, detail error) {
	fmt.Fprintf(os.Stderr, "Error: cannot connect to server at %s\n", serverURL)
	fmt.Fprintln(os.Stderr, "  Check your network connection and server address.")
	fmt.Fprintln(os.Stderr, "  Use --server to specify a different server.")
	if detail != nil {
		fmt.Fprintf(os.Stderr, "  (%v)\n", detail)
	}
	os.Exit(exitConnection)
}

// printAuthError prints the PART 32 "Error Messages" auth-error format
// (multi-line: reason + two hints) and exits exitAuth.
func printAuthError() {
	fmt.Fprintln(os.Stderr, "Error: authentication failed")
	fmt.Fprintln(os.Stderr, "  Your API token is invalid or expired.")
	fmt.Fprintln(os.Stderr, "  Update auth.token in cli.yml or use --token flag.")
	os.Exit(exitAuth)
}

// printNotFoundError prints the PART 32 "Error Messages" not-found format
// (single line) and exits exitNotFound.
func printNotFoundError(resource string) {
	fmt.Fprintf(os.Stderr, "Error: resource not found: %s\n", resource)
	os.Exit(exitNotFound)
}

// ─── CLI config (cli.yml) ─────────────────────────────────────────────────────

// cliConfig mirrors the complete structure of cli.yml (PART 32).
type cliConfig struct {
	Server struct {
		Primary     string `yaml:"primary"`
		APIVersion  string `yaml:"api_version"`
		Timeout     string `yaml:"timeout"`
		Retry       int    `yaml:"retry"`
		RetryDelay  string `yaml:"retry_delay"`
	} `yaml:"server"`
	Update struct {
		Auto          bool   `yaml:"auto"`
		CheckInterval string `yaml:"check_interval"`
		Channel       string `yaml:"channel"`
	} `yaml:"update"`
	Display struct {
		Mode string `yaml:"mode"`
	} `yaml:"display"`
	Auth struct {
		Token     string `yaml:"token"`
		TokenFile string `yaml:"token_file"`
	} `yaml:"auth"`
	Output struct {
		Format  string `yaml:"format"`
		Color   string `yaml:"color"`
		Pager   string `yaml:"pager"`
		Quiet   bool   `yaml:"quiet"`
		Verbose bool   `yaml:"verbose"`
	} `yaml:"output"`
	TUI struct {
		Enabled bool   `yaml:"enabled"`
		Theme   string `yaml:"theme"`
		Mouse   bool   `yaml:"mouse"`
		Unicode bool   `yaml:"unicode"`
	} `yaml:"tui"`
	Logging struct {
		Level    string `yaml:"level"`
		File     string `yaml:"file"`
		MaxSize  int    `yaml:"max_size"`
		MaxFiles int    `yaml:"max_files"`
	} `yaml:"logging"`
	Cache struct {
		Enabled bool   `yaml:"enabled"`
		TTL     string `yaml:"ttl"`
		MaxSize int    `yaml:"max_size"`
	} `yaml:"cache"`
	Debug    bool `yaml:"debug"`
	Defaults struct {
		Lang   string `yaml:"lang"`
		Public bool   `yaml:"public"`
		Expire string `yaml:"expire"`
		Syntax string `yaml:"syntax"`
		Output string `yaml:"output"`
		Limit  int    `yaml:"limit"`
	} `yaml:"defaults"`
}

// activeConfigPath, when non-empty, overrides the default cli.yml path.
// It is set from the --config flag (profile name or explicit path) during
// startup, before cli.yml is loaded, so the chosen profile feeds every
// flag default. Empty means "use cliConfigPath()".
var activeConfigPath string

// cliConfigPath returns the platform-correct path to cli.yml.
// The CLI always uses user-scope directories regardless of privilege level;
// it never falls back to system directories like /etc/.
func cliConfigPath() string {
	if p := os.Getenv("CLI_CONFIG"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "apimgr", projectName, "cli.yml")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", "apimgr", projectName, "cli.yml")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "apimgr", projectName, "cli.yml")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "apimgr", projectName, "cli.yml")
	}
}

// resolvedConfigPath returns the active config path, falling back to the
// default cli.yml location when no --config profile/path was selected.
func resolvedConfigPath() string {
	if activeConfigPath != "" {
		return activeConfigPath
	}
	return cliConfigPath()
}

// prescanConfigFlag scans args for --config NAME / --config=NAME before
// flag.Parse runs, so the selected profile can be loaded first (PART 32).
func prescanConfigFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--config" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(a, "--config=") {
			return strings.TrimPrefix(a, "--config=")
		}
	}
	return ""
}

// resolveConfigPath maps a --config value to a concrete file path (PART 32):
//   - empty         → default cli.yml
//   - ~ or absolute → expanded, then extension-resolved
//   - relative name → {config_dir}/{name}, then extension-resolved
func resolveConfigPath(name string) string {
	if name == "" {
		return cliConfigPath()
	}
	if strings.HasPrefix(name, "~") {
		home, _ := os.UserHomeDir()
		name = filepath.Join(home, strings.TrimPrefix(name, "~"))
	}
	if filepath.IsAbs(name) {
		return resolveYamlExtension(name)
	}
	dir := filepath.Dir(cliConfigPath())
	return resolveYamlExtension(filepath.Join(dir, name))
}

// resolveYamlExtension applies PART 32 rules 3-5: an explicit .yml/.yaml (or any
// other) extension is kept as-is; an extensionless path prefers an existing
// .yml, then .yaml, defaulting to .yml for a new config.
func resolveYamlExtension(path string) string {
	switch filepath.Ext(path) {
	case ".yml", ".yaml":
		return path
	case "":
		if fileExists(path + ".yml") {
			return path + ".yml"
		}
		if fileExists(path + ".yaml") {
			return path + ".yaml"
		}
		return path + ".yml"
	default:
		return path
	}
}

// fileExists reports whether path exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// loadCLIConfig reads cli.yml; returns zero-value config if absent.
func loadCLIConfig() (cliConfig, error) {
	var cfg cliConfig
	cfg.Server.APIVersion = apiVersion
	cfg.Server.Timeout = "30s"
	cfg.Server.Retry = 3
	cfg.Server.RetryDelay = "1s"
	cfg.Update.Channel = "stable"
	cfg.Update.CheckInterval = "per_invocation"
	cfg.Display.Mode = "auto"
	cfg.Output.Format = "table"
	cfg.Output.Color = "auto"
	cfg.TUI.Enabled = true
	cfg.TUI.Unicode = true
	cfg.Cache.TTL = "5m"
	cfg.Cache.MaxSize = 100
	cfg.Defaults.Expire = "never"
	cfg.Defaults.Syntax = "text"
	cfg.Defaults.Limit = 20

	data, err := os.ReadFile(resolvedConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse cli.yml: %w", err)
	}
	return cfg, nil
}

// saveCLIConfig writes cfg to cli.yml, creating parent dirs as needed.
func saveCLIConfig(cfg cliConfig) error {
	p := resolvedConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	// cli.yml holds the API token as well as server connection config —
	// create it with user-only permissions (PART 32).
	return os.WriteFile(p, data, 0o600)
}

// saveIfUnset updates dst with src when dst is empty or invalid, and
// returns both the resolved value and whether it should be persisted.
// Implements PART 32 Flag-to-Config Save Rules.
func saveIfUnset(current, flagValue string, validate func(string) bool) (resolved string, persist bool) {
	if flagValue == "" {
		return current, false
	}
	if !validate(flagValue) {
		log.Printf("warning: invalid server URL %q, keeping current config", flagValue)
		return current, false
	}
	if current == "" {
		return flagValue, true
	}
	if !validate(current) {
		return flagValue, true
	}
	return flagValue, false
}

// isValidURL is the validate function for server URLs.
func isValidURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// readTokenFile reads an API token from path (PART 32 --token-file / auth.token_file),
// trimming surrounding whitespace so a trailing newline from the file doesn't
// end up in the Authorization header.
func readTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return token, nil
}

// ─── Display mode detection ───────────────────────────────────────────────────

// detectMode returns "tui", "cli", or "plain" based on environment and args.
// Implements PART 32 Automatic Mode Detection rules. displayMode is the
// cli.yml `display.mode` override ("auto" (default), "tui", or "gui"); it
// never comes from a CLI flag — PART 32 forbids --tui/--gui/--mode-ui flags.
func detectMode(args []string, displayMode string) string {
	// Exit-immediately flags — never TUI.
	for _, arg := range args {
		switch arg {
		case "-h", "--help", "-v", "--version":
			return "cli"
		}
	}

	// Config override: force TUI even when stdout isn't a terminal.
	// "gui" is not implemented (no native GUI toolkit shipped) — treated as
	// unsupported and falls back to auto-detection rather than failing silently.
	if displayMode == "tui" {
		return "tui"
	}

	// Not a terminal → plain output (piped, cron, scripts).
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "plain"
	}

	// Config-only flags that still allow TUI launch (value: flag consumes the next arg).
	configFlags := map[string]bool{
		"--config": true, "--server": true, "--token": true, "--token-file": true,
		"--debug": true, "--color": true, "--json": true, "--lang": true,
	}
	valueFlags := map[string]bool{
		"--config": true, "--server": true, "--token": true, "--token-file": true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return "cli"
		}
		parts := strings.SplitN(arg, "=", 2)
		if !configFlags[parts[0]] {
			return "cli"
		}
		// Space syntax: skip the flag's value (--flag value).
		if valueFlags[parts[0]] && !strings.Contains(arg, "=") && i+1 < len(args) {
			i++
		}
	}

	return "tui"
}

// ─── Auto-update via autodiscover ────────────────────────────────────────────

// autodiscoverResponse is the subset of /api/autodiscover we need.
type autodiscoverResponse struct {
	CLIVersions   map[string]cliVersionInfo `json:"cli_versions"`
	CLIMinVersion string                    `json:"cli_min_version"`
}

type cliVersionInfo struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// printVersionInfo prints the CLI's own version line and, when a server URL
// is configured and reachable, the AI.md PART 32 "--version Extended Output"
// block: the server URL, a version-compatibility check against
// GET /api/{api_version}/server/version, and the Go/OS build info. The
// extended block is best-effort — any lookup failure silently falls back to
// the base version line only, matching checkCLIUpdate's non-fatal contract.
func printVersionInfo(serverURL string) {
	binaryName := filepath.Base(os.Args[0])
	fmt.Printf("%s %s (commit %s, built %s)\n", binaryName, Version, CommitID, BuildDate)

	if serverURL == "" {
		return
	}
	serverURL = strings.TrimRight(serverURL, "/")

	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/"+apiVersion+"/server/version", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))

	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Version   string `json:"version"`
			GoVersion string `json:"go_version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || !payload.OK || payload.Data.Version == "" {
		return
	}

	compat := "compatible"
	cliMajor := strings.SplitN(Version, ".", 2)[0]
	serverMajor := strings.SplitN(payload.Data.Version, ".", 2)[0]
	if Version != "dev" && payload.Data.Version != "unknown" && cliMajor != serverMajor {
		compat = "incompatible"
	}

	fmt.Println()
	fmt.Printf("Server: %s\n", serverURL)
	fmt.Printf("Server Version: %s (%s)\n", payload.Data.Version, compat)
	fmt.Println()
	fmt.Println("Build Info:")
	fmt.Printf("  Go: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// resolveOutputFormat combines the legacy --json boolean alias with the
// PART 32 "Output Formats" --output flag (json, table, plain) into a single
// resolved value. An explicit --output wins over --json; an unrecognized
// --output value falls back to "plain" rather than erroring, matching this
// codebase's config.ParseBool-style default-on-invalid convention.
func resolveOutputFormat(asJSON bool, output string) string {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "json":
		return "json"
	case "table":
		return "table"
	case "plain":
		return "plain"
	}
	if asJSON {
		return "json"
	}
	return "plain"
}

// checkCLIUpdate queries /api/autodiscover and enforces cli_min_version.
// It logs a notice when a newer version is available but does not auto-update
// (cli.yml update.auto defaults to false for the CLI per PART 32).
// Returns an error only when Version < cli_min_version (must refuse further requests).
func checkCLIUpdate(serverURL, lang string) error {
	if serverURL == "" || Version == "dev" {
		return nil
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(serverURL, "/")+"/api/autodiscover", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))
	if lang != "" {
		req.Header.Set("Accept-Language", lang)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var disc autodiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		return nil
	}

	// Enforce minimum version requirement.
	if disc.CLIMinVersion != "" && versionLessThan(Version, disc.CLIMinVersion) {
		return fmt.Errorf(
			"this CLI is too old; the server requires %s — run 'pastebin-cli update yes' to upgrade",
			disc.CLIMinVersion,
		)
	}

	// Notify when a newer version is available.
	osArch := runtime.GOOS + "-" + runtime.GOARCH
	if info, ok := disc.CLIVersions[osArch]; ok {
		if versionLessThan(Version, info.Version) {
			fmt.Fprintf(os.Stderr, "notice: pastebin-cli %s is available (you have %s); run 'pastebin-cli --update yes' to upgrade\n",
				info.Version, Version)
		}
	}

	return nil
}

// versionLessThan returns true when semver a < b.
// Compares MAJOR.MINOR.PATCH numerically so "0.9.0" < "0.10.0" is correct.
// Returns false for non-numeric or special versions (dev, unknown).
func versionLessThan(a, b string) bool {
	if a == "dev" || b == "dev" || a == "unknown" || b == "unknown" {
		return false
	}
	aParts := strings.SplitN(a, ".", 3)
	bParts := strings.SplitN(b, ".", 3)
	for len(aParts) < 3 {
		aParts = append(aParts, "0")
	}
	for len(bParts) < 3 {
		bParts = append(bParts, "0")
	}
	for i := range 3 {
		av, ae := strconv.Atoi(aParts[i])
		bv, be := strconv.Atoi(bParts[i])
		if ae != nil || be != nil {
			// Non-numeric component — fall back to string compare for this segment.
			c := strings.Compare(aParts[i], bParts[i])
			if c != 0 {
				return c < 0
			}
			continue
		}
		if av != bv {
			return av < bv
		}
	}
	return false
}

// ─── Entry point ─────────────────────────────────────────────────────────────

// ensureDirs creates the standard user-scope directories for the CLI client
// (config, data, cache, log). Called at startup before any config is loaded.
func ensureDirs() {
	home, _ := os.UserHomeDir()
	dirs := []string{
		filepath.Join(home, ".config", "apimgr", projectName),
		filepath.Join(home, ".local", "share", "apimgr", projectName),
		filepath.Join(home, ".cache", "apimgr", projectName),
		filepath.Join(home, ".local", "log", "apimgr", projectName),
	}
	for _, d := range dirs {
		os.MkdirAll(d, 0o700)
		// PART 32 CLI Startup Sequence step 2: re-assert 0700 on every startup —
		// MkdirAll leaves an already-existing directory's permissions untouched.
		if runtime.GOOS != "windows" {
			os.Chmod(d, 0o700)
		}
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix(filepath.Base(os.Args[0]) + ": ")

	ensureDirs()

	// PART 32: --config selects a named profile (or explicit path) before
	// cli.yml is loaded, so the chosen profile feeds every flag default.
	if name := prescanConfigFlag(os.Args[1:]); name != "" {
		activeConfigPath = resolveConfigPath(name)
	}

	// Load cli.yml.
	fileCfg, err := loadCLIConfig()
	if err != nil {
		log.Printf("warning: could not load cli.yml: %v", err)
	}

	server := flag.String("server", envOrDefault("PASTEBIN_SERVER_PRIMARY", fileCfg.Server.Primary), "server base URL")
	// PART 32 config precedence: CLI flag > env var > cli.yml > compiled default.
	jsonDefault := envOrDefault("PASTEBIN_OUTPUT_FORMAT", fileCfg.Output.Format) == "json"
	asJSON := flag.Bool("json", jsonDefault, "machine-readable JSON output (alias for --output json)")
	// PART 32 "Output Formats": json, table, plain. --json remains as a
	// back-compat boolean alias, folded into the resolved format below.
	outputFlag := flag.String("output", envOrDefault("PASTEBIN_OUTPUT_FORMAT", firstNonEmpty(fileCfg.Output.Format, "plain")), "output format: json, table, plain")
	colorFlag := flag.String("color", envOrDefault("PASTEBIN_OUTPUT_COLOR", firstNonEmpty(fileCfg.Output.Color, "auto")), "color output: auto, yes, no")
	showVersion := flag.Bool("version", false, "print version and exit")
	showHelp := flag.Bool("help", false, "show help and exit")
	debugDefault, _ := config.ParseBool(envOrDefault("PASTEBIN_DEBUG", ""), fileCfg.Debug)
	debugFlag := flag.Bool("debug", debugDefault, "enable debug output")
	doUpdate := flag.String("update", "", "check for CLI updates: 'check' or 'yes'")
	// PART 32: --lang sets the output/UI language; default auto-detects from the LANG env var.
	langFlag := flag.String("lang", "auto", "output language code (default: auto-detect from LANG)")
	// PART 32: operator/owner API token. Priority: --token flag → PASTEBIN_TOKEN env → cli.yml auth.token.
	tokenFlag := flag.String("token", "", "operator/owner API token (or set PASTEBIN_TOKEN)")
	// PART 32: alternative to --token — read the token from a file instead of the command line.
	tokenFileFlag := flag.String("token-file", "", "read API token from file")
	// PART 32: --config selects a named profile or explicit path to cli.yml.
	// Pre-scanned above so it applies before config load; declared here so
	// flag.Parse accepts it and --help lists it.
	flag.String("config", "", "config profile name or path (default: cli.yml)")

	// -h and -v are aliases for --help and --version.
	flag.BoolVar(showHelp, "h", false, "show help and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit")

	flag.Usage = printUsage
	// ContinueOnError instead of the default ExitOnError: the stdlib flag
	// package hardcodes os.Exit(2) on a parse error, which collides with
	// this CLI's own exitConfig=2 (PART 32/8 exit codes reserve 2 for
	// configuration errors, not bad arguments). A malformed flag is a
	// usage error and must exit exitUsage=64 instead.
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		os.Exit(exitUsage)
	}

	// Honour NO_COLOR env var (https://no-color.org/) and --color flag.
	// Spec canonical values: auto, yes, no (AI.md PART 8).
	// always/never are accepted as backward-compatible aliases.
	switch *colorFlag {
	case "never", "no":
		os.Setenv("NO_COLOR", "1")
	case "always", "yes":
		os.Unsetenv("NO_COLOR")
	}

	if *showHelp {
		printUsage()
		return
	}

	if *showVersion {
		printVersionInfo(*server)
		return
	}

	if *debugFlag {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	args := flag.Args()

	// --shell completions / --shell init — handle before server check.
	if len(args) >= 1 && args[0] == "--shell" {
		shellArg := ""
		if len(args) >= 2 {
			shellArg = args[1]
		}
		switch shellArg {
		case "--help", "help", "":
			shell.PrintHelp(filepath.Base(os.Args[0]))
		case "init":
			shellShell := ""
			if len(args) >= 3 {
				shellShell = args[2]
			}
			if err := shell.PrintInit(filepath.Base(os.Args[0]), shellShell); err != nil {
				fmt.Fprintf(os.Stderr, "%s: --shell init: %v\n", filepath.Base(os.Args[0]), err)
				os.Exit(exitUsage)
			}
		case "completions":
			shellShell := ""
			if len(args) >= 3 {
				shellShell = args[2]
			}
			if err := shell.PrintClientCompletions(filepath.Base(os.Args[0]), shellShell); err != nil {
				fmt.Fprintf(os.Stderr, "%s: --shell completions: %v\n", filepath.Base(os.Args[0]), err)
				os.Exit(exitUsage)
			}
		default:
			fmt.Fprintf(os.Stderr, "%s: --shell: unknown subcommand %q\n", filepath.Base(os.Args[0]), shellArg)
			fmt.Fprintf(os.Stderr, "Run '%s --shell --help' for usage.\n", filepath.Base(os.Args[0]))
			os.Exit(exitUsage)
		}
		return
	}

	// Apply saveIfUnset: persist server to cli.yml when config was empty or invalid.
	// Use the current parsed value of --server as the flagValue.
	resolved, shouldPersist := saveIfUnset(fileCfg.Server.Primary, *server, isValidURL)
	if shouldPersist && resolved != "" {
		fileCfg.Server.Primary = resolved
		if err := saveCLIConfig(fileCfg); err != nil {
			log.Printf("warning: could not save cli.yml: %v", err)
		}
	}
	if resolved != "" {
		*server = resolved
	}

	*server = defaultServerURL(*server, OfficialSite)

	// Resolve the API token (PART 32 priority): --token flag → --token-file flag →
	// PASTEBIN_TOKEN env → cli.yml auth.token → cli.yml auth.token_file.
	// The env var never persists; the --token/--token-file flags save to cli.yml
	// only when the stored value is empty/invalid.
	token := *tokenFlag
	if token == "" && *tokenFileFlag != "" {
		t, err := readTokenFile(*tokenFileFlag)
		if err != nil {
			log.Printf("warning: could not read --token-file %s: %v", *tokenFileFlag, err)
		}
		token = t
	}
	if token == "" {
		token = os.Getenv("PASTEBIN_TOKEN")
	}
	if token == "" {
		token = fileCfg.Auth.Token
	}
	if token == "" && fileCfg.Auth.TokenFile != "" {
		t, err := readTokenFile(fileCfg.Auth.TokenFile)
		if err != nil {
			log.Printf("warning: could not read auth.token_file %s: %v", fileCfg.Auth.TokenFile, err)
		}
		token = t
	}
	if *tokenFlag != "" {
		if resolvedTok, persist := saveIfUnset(fileCfg.Auth.Token, *tokenFlag, func(s string) bool { return s != "" }); persist && resolvedTok != "" {
			fileCfg.Auth.Token = resolvedTok
			if err := saveCLIConfig(fileCfg); err != nil {
				log.Printf("warning: could not save cli.yml: %v", err)
			}
		}
	}
	if *tokenFileFlag != "" {
		if resolvedPath, persist := saveIfUnset(fileCfg.Auth.TokenFile, *tokenFileFlag, func(s string) bool { return s != "" }); persist && resolvedPath != "" {
			fileCfg.Auth.TokenFile = resolvedPath
			if err := saveCLIConfig(fileCfg); err != nil {
				log.Printf("warning: could not save cli.yml: %v", err)
			}
		}
	}

	// Handle --update flag.
	locale := detectLocale(*langFlag, fileCfg.Defaults.Lang)

	// PART 32 env var mapping ({PROJECT_NAME}_SERVER_TIMEOUT, _RETRY,
	// _RETRY_DELAY, _API_VERSION) overrides cli.yml before defaults are
	// resolved, matching the documented CLI flag > env var > cli.yml >
	// compiled default precedence.
	fileCfg.Server.Timeout = envOrDefault("PASTEBIN_SERVER_TIMEOUT", fileCfg.Server.Timeout)
	fileCfg.Server.RetryDelay = envOrDefault("PASTEBIN_SERVER_RETRY_DELAY", fileCfg.Server.RetryDelay)
	fileCfg.Server.APIVersion = envOrDefault("PASTEBIN_SERVER_API_VERSION", fileCfg.Server.APIVersion)
	if v := os.Getenv("PASTEBIN_SERVER_RETRY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			fileCfg.Server.Retry = n
		}
	}

	// Resolve server.timeout/retry/retry_delay/api_version from cli.yml
	// (PART 32) once, shared by every client instance constructed below.
	timeout, retry, retryDelay := resolveServerTiming(fileCfg)
	resolvedAPIVersion := fileCfg.Server.APIVersion
	if resolvedAPIVersion == "" {
		resolvedAPIVersion = apiVersion
	}

	// Resolve the PART 32 "Output Formats" value once. --json is a
	// back-compat boolean alias for --output json.
	outputFormat := resolveOutputFormat(*asJSON, *outputFlag)

	if *doUpdate != "" {
		c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON, outputFormat: outputFormat, lang: locale, token: token, apiVersion: resolvedAPIVersion, timeout: timeout, retry: retry, retryDelay: retryDelay}
		c.cmdUpdate(*doUpdate)
		return
	}

	// Auto-detect display mode per PART 32, honoring the cli.yml
	// display.mode override ("auto", "tui", or "gui" — never a CLI flag).
	if fileCfg.Display.Mode == "gui" {
		fmt.Fprintf(os.Stderr, "%s: display.mode is set to \"gui\" in cli.yml, but no native GUI is available on this build\n", filepath.Base(os.Args[0]))
		os.Exit(exitGeneral)
	}
	mode := detectMode(args, fileCfg.Display.Mode)
	// cli.yml tui.enabled: false forces CLI-only mode even when the
	// terminal would otherwise auto-detect TUI (PART 32, AI.md line 44633).
	if mode == "tui" && !fileCfg.TUI.Enabled {
		mode = "cli"
	}
	if mode == "tui" {
		runTUI(*server, locale, fileCfg)
		return
	}

	if len(args) == 0 {
		printUsage()
		os.Exit(exitUsage)
	}

	if *server == "" {
		printNoServerError()
		os.Exit(exitConnection)
	}

	// Check for CLI updates (non-blocking; only blocks on min_version violation).
	if err := checkCLIUpdate(*server, locale); err != nil {
		fmt.Fprintf(os.Stderr, "%s: update check: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
	}

	c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON, outputFormat: outputFormat, lang: locale, token: token, apiVersion: resolvedAPIVersion, timeout: timeout, retry: retry, retryDelay: retryDelay}

	switch args[0] {
	case "create":
		c.cmdCreate(args[1:])
	case "get":
		c.cmdGet(args[1:])
	case "delete", "del", "rm":
		c.cmdDelete(args[1:])
	case "list", "ls":
		c.cmdList(args[1:])
	case "update":
		// Positional form: pastebin-cli update [check|yes]
		// Equivalent to --update flag; defaults to "check" when no sub-arg given.
		action := "check"
		if len(args) >= 2 {
			action = args[1]
		}
		c.cmdUpdate(action)
	case "version":
		printVersionInfo(*server)
	default:
		fmt.Fprintf(os.Stderr, "%s: unknown command %q (try: create, get, delete, list, update)\n", filepath.Base(os.Args[0]), args[0])
		os.Exit(exitUsage)
	}
}

// saveCLIConfigURL updates the server URL field in cli.yml.
func saveCLIConfigURL(serverURL string) error {
	cfg, _ := loadCLIConfig()
	cfg.Server.Primary = serverURL
	return saveCLIConfig(cfg)
}

// runTUI launches the interactive bubbletea TUI mode.
// When no server is configured, the TUI setup wizard collects it.
func runTUI(server, lang string, cfg cliConfig) {
	// Auto-update check in TUI mode (non-fatal for version notices).
	if server != "" {
		if err := checkCLIUpdate(server, lang); err != nil {
			fmt.Fprintf(os.Stderr, "%s: update check: %v\n", filepath.Base(os.Args[0]), err)
			os.Exit(exitConnection)
		}
	}

	tuiCfg := tui.ClientConfig{
		Server:  server,
		Lang:    lang,
		SaveURL: saveCLIConfigURL,
		CfgPath: resolvedConfigPath(),
		Theme:   cfg.TUI.Theme,
	}
	if err := tui.Run(tuiCfg); err != nil {
		fmt.Fprintf(os.Stderr, "%s: tui: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}
}

// ─── Client ───────────────────────────────────────────────────────────────────

type client struct {
	server string
	asJSON bool
	// outputFormat is the resolved --output value (json, table, or plain;
	// PART 32 "Output Formats"). --json is a back-compat alias for
	// --output json and is folded into this field, not read separately
	// past flag parsing.
	outputFormat string
	// lang is the resolved output/UI locale sent as the Accept-Language header (PART 30/32).
	lang string
	// token is the resolved operator/owner API token sent as the Authorization bearer header (PART 32).
	token string
	// apiVersion is the resolved cli.yml server.api_version (PART 32), falling
	// back to the compiled apiVersion const when unset or invalid.
	apiVersion string
	// timeout is the resolved cli.yml server.timeout (PART 32), applied to every request.
	timeout time.Duration
	// retry is the resolved cli.yml server.retry (PART 32): retryable-error attempts after the first.
	retry int
	// retryDelay is the resolved cli.yml server.retry_delay (PART 32): fixed delay between retries.
	retryDelay time.Duration
}

// defaultClientTimeout/defaultClientRetry/defaultClientRetryDelay mirror the
// cli.yml server.{timeout,retry,retry_delay} compiled defaults (PART 32) and
// are used whenever cli.yml is missing or a value fails to parse.
const (
	defaultClientTimeout    = 30 * time.Second
	defaultClientRetry      = 3
	defaultClientRetryDelay = 1 * time.Second
)

// resolveServerTiming parses cli.yml's server.timeout/retry/retry_delay,
// falling back to the compiled defaults on empty or invalid values (PART 32:
// invalid config values warn and substitute the default, never crash).
func resolveServerTiming(cfg cliConfig) (timeout time.Duration, retry int, retryDelay time.Duration) {
	timeout = defaultClientTimeout
	if cfg.Server.Timeout != "" {
		if d, err := time.ParseDuration(cfg.Server.Timeout); err == nil && d > 0 {
			timeout = d
		} else {
			log.Printf("warning: invalid server.timeout %q in cli.yml, using default %s", cfg.Server.Timeout, defaultClientTimeout)
		}
	}
	retry = defaultClientRetry
	if cfg.Server.Retry > 0 {
		retry = cfg.Server.Retry
	}
	retryDelay = defaultClientRetryDelay
	if cfg.Server.RetryDelay != "" {
		if d, err := time.ParseDuration(cfg.Server.RetryDelay); err == nil && d >= 0 {
			retryDelay = d
		} else {
			log.Printf("warning: invalid server.retry_delay %q in cli.yml, using default %s", cfg.Server.RetryDelay, defaultClientRetryDelay)
		}
	}
	return timeout, retry, retryDelay
}

// doWithRetry executes req via httpClient, retrying up to c.retry additional
// times (fixed delay c.retryDelay between attempts) on network errors or a
// 503 response — never on 4xx (PART 9/11 retry policy: retryable errors only).
func (c *client) doWithRetry(httpClient *http.Client, req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retry; attempt++ {
		if attempt > 0 {
			time.Sleep(c.retryDelay)
			// Requests with a body (POST) must have it rewound before each
			// retry — http.NewRequest populates GetBody for bytes.Reader
			// bodies, but Do() only consumes it on redirects, not on a
			// second manual call with the same *http.Request.
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				req.Body = body
			}
		}
		resp, err := httpClient.Do(req)
		if err == nil && resp.StatusCode != http.StatusServiceUnavailable {
			return resp, nil
		}
		if err == nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
		} else {
			lastErr = err
		}
	}
	return nil, lastErr
}

// setAuth adds the Authorization bearer header when an API token is configured (PART 32).
func (c *client) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// detectLocale resolves the output locale from the --lang flag, falling back to
// cli.yml's defaults.lang, then to the LANG / LC_ALL environment variables, then to
// "en" (PART 30 priority order). A value of "auto" or "" triggers the next priority.
// The server silently falls back to English for unsupported codes.
func detectLocale(flagVal, configVal string) string {
	v := strings.TrimSpace(flagVal)
	if v != "" && v != "auto" {
		return v
	}
	if cv := strings.TrimSpace(configVal); cv != "" && cv != "auto" {
		return cv
	}
	for _, env := range []string{"LC_ALL", "LANG", "LANGUAGE"} {
		if val := os.Getenv(env); val != "" {
			// Strip encoding/territory suffixes: "en_US.UTF-8" -> "en".
			code := val
			if i := strings.IndexAny(code, "._@"); i >= 0 {
				code = code[:i]
			}
			if i := strings.IndexByte(code, '_'); i >= 0 {
				code = code[:i]
			}
			if code != "" && code != "C" && code != "POSIX" {
				return code
			}
		}
	}
	return "en"
}

func (c *client) cmdCreate(args []string) {
	// ContinueOnError so a bad flag falls through to the exitUsage=64 handling
	// below instead of ExitOnError's hardcoded os.Exit(2).
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	lang := fs.String("lang", "text", "syntax language")
	expiry := fs.String("expiry", "never", "expiry: 1h 1d 1w 1m 3m 6m 1y 2y never or seconds")
	burn := fs.Int("burn", 0, "delete after N views (0 = disabled)")
	unlisted := fs.Bool("unlisted", false, "create as unlisted (not shown in recent)")
	title := fs.String("title", "", "paste title")
	asLink := fs.Bool("link", false, "create as a link — content/arg must be an absolute http:// or https:// URL; server issues a 302 redirect instead of rendering")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: create: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}

	var content []byte
	var err error
	effLang := *lang
	effTitle := *title

	if fs.NArg() > 0 {
		if *asLink {
			// Link mode: the positional arg is the target URL itself, not a
			// file path — never read it off disk.
			content = []byte(fs.Arg(0))
		} else {
			// Smart argument detection (AI.md PART 32 "Smart Argument
			// Detection"): stat the arg first. A directory gets a dedicated
			// error, an existing file is read from disk, and anything else
			// is treated as literal paste text rather than a raw I/O error.
			arg := fs.Arg(0)
			info, statErr := os.Stat(arg)
			switch {
			case statErr == nil && info.IsDir():
				fmt.Fprintf(os.Stderr, "%s: create: %q is a directory, not a file\n", filepath.Base(os.Args[0]), arg)
				os.Exit(exitUsage)
			case statErr == nil:
				content, err = os.ReadFile(arg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "%s: read file: %v\n", filepath.Base(os.Args[0]), err)
					os.Exit(exitGeneral)
				}
				if effLang == "text" {
					effLang = detectLang(arg)
				}
				if effTitle == "" {
					effTitle = arg
				}
			default:
				content = []byte(strings.Join(fs.Args(), " "))
			}
		}
	} else {
		content, err = io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: read stdin: %v\n", filepath.Base(os.Args[0]), err)
			os.Exit(exitGeneral)
		}
	}

	vis := 0
	if *unlisted {
		vis = 1
	}

	body := map[string]interface{}{
		"content":    string(content),
		"title":      effTitle,
		"language":   effLang,
		"expires_in": *expiry,
		"burn_after": *burn,
		"visibility": vis,
	}

	if *asLink {
		// Links carry no language/syntax mode and are never base64-encoded —
		// content is always the plain target URL string. There is no is_link
		// field to set: the server auto-detects link mode because content is
		// exactly one http(s):// URL and nothing else.
		delete(body, "language")
	} else {
		// Binary files (images, archives, etc.) cannot travel as raw bytes inside
		// a JSON string — invalid UTF-8 gets replaced and corrupts the data.
		// Detect the MIME type, base64-encode, and tell the server via
		// content_type.
		sample := content
		if len(sample) > 512 {
			sample = sample[:512]
		}
		if detected := http.DetectContentType(sample); !strings.HasPrefix(detected, "text/") {
			body["content"] = base64.StdEncoding.EncodeToString(content)
			body["content_type"] = detected
		}
	}

	resp, err := c.postJSON("/api/"+c.apiVer()+"/pastes", body)
	if err != nil {
		printConnectionError(c.server, err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "%s: decode response: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		printAuthError()
	}
	if resp.StatusCode != http.StatusCreated {
		errMsg, _ := result["error"].(string)
		fmt.Fprintf(os.Stderr, "%s: create: server error %d: %s\n", filepath.Base(os.Args[0]), resp.StatusCode, errMsg)
		os.Exit(exitGeneral)
	}

	if c.outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	link, _ := result["link"].(string)
	token, _ := result["delete_token"].(string)
	fmt.Printf("URL:          %s\n", link)
	if token != "" {
		fmt.Printf("Delete token: %s\n", token)
		fmt.Println("(save the delete token — it will not be shown again)")
	}
}

func (c *client) cmdGet(args []string) {
	// ContinueOnError so a bad flag falls through to the exitUsage=64 handling
	// below instead of ExitOnError's hardcoded os.Exit(2).
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: get: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "%s: usage: get <id>\n", filepath.Base(os.Args[0]))
		os.Exit(exitUsage)
	}
	id := fs.Arg(0)

	// PART 32 "Output Formats": --output json returns the full paste
	// metadata via the JSON API; plain/table (table has no meaning for a
	// single resource, so it degrades to plain) return raw content only,
	// as documented in the "--output plain" example.
	path := "/raw/" + url.PathEscape(id)
	if c.outputFormat == "json" {
		path = "/api/" + c.apiVer() + "/pastes/" + url.PathEscape(id)
	}

	resp, err := c.get(path)
	if err != nil {
		printConnectionError(c.server, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		printNotFoundError(id)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		printAuthError()
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "%s: get: server returned %d\n", filepath.Base(os.Args[0]), resp.StatusCode)
		os.Exit(exitGeneral)
	}

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		fmt.Fprintf(os.Stderr, "%s: get: read response: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}
}

func (c *client) cmdDelete(args []string) {
	// ContinueOnError so a bad flag falls through to the exitUsage=64 handling
	// below instead of ExitOnError's hardcoded os.Exit(2).
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: delete: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}
	if fs.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "%s: usage: delete <id> <token>\n", filepath.Base(os.Args[0]))
		os.Exit(exitUsage)
	}
	id, token := fs.Arg(0), fs.Arg(1)

	req, err := http.NewRequest(
		http.MethodDelete,
		c.url("/api/"+c.apiVer()+"/pastes/"+url.PathEscape(id)+"?token="+url.QueryEscape(token)),
		nil,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: delete: build request: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))
	if c.lang != "" {
		req.Header.Set("Accept-Language", c.lang)
	}
	c.setAuth(req)

	httpClient := &http.Client{Timeout: c.clientTimeout()}
	resp, err := c.doWithRetry(httpClient, req)
	if err != nil {
		printConnectionError(c.server, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		printNotFoundError(id)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		printAuthError()
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "%s: delete: server returned %d\n", filepath.Base(os.Args[0]), resp.StatusCode)
		os.Exit(exitGeneral)
	}

	if c.outputFormat == "json" {
		io.Copy(os.Stdout, resp.Body)
		return
	}
	fmt.Printf("paste %s deleted\n", id)
}

func (c *client) cmdList(args []string) {
	// ContinueOnError so a bad flag falls through to the exitUsage=64 handling
	// below instead of ExitOnError's hardcoded os.Exit(2).
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "number of pastes to list (max 100)")
	page := fs.Int("page", 1, "page number")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: list: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}

	resp, err := c.get(fmt.Sprintf("/api/%s/pastes?page=%d&limit=%d", c.apiVer(), *page, *limit))
	if err != nil {
		printConnectionError(c.server, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		printAuthError()
	}

	var result struct {
		Pastes []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Language  string    `json:"language"`
			Views     int       `json:"views"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"data"`
		Pagination struct {
			Total int `json:"total"`
			Pages int `json:"pages"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "%s: list: decode: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}

	if c.outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	if len(result.Pastes) == 0 {
		fmt.Println("no pastes found")
		return
	}

	if c.outputFormat == "table" {
		headers := []string{"ID", "Title", "Language", "Created"}
		rows := make([][]string, 0, len(result.Pastes))
		for _, p := range result.Pastes {
			title := p.Title
			if len(title) > 45 {
				title = title[:42] + "..."
			}
			rows = append(rows, []string{p.ID, title, p.Language, p.CreatedAt.Format("2006-01-02")})
		}
		renderBorderedTable(os.Stdout, headers, rows)
		fmt.Printf("\n(%d total, page %d of %d)\n",
			result.Pagination.Total, *page, result.Pagination.Pages)
		return
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTITLE\tLANG\tVIEWS\tCREATED")
	for _, p := range result.Pastes {
		title := p.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n",
			p.ID, title, p.Language, p.Views,
			p.CreatedAt.Format("2006-01-02 15:04"),
		)
	}
	tw.Flush()
	fmt.Printf("\n(%d total, page %d of %d)\n",
		result.Pagination.Total, *page, result.Pagination.Pages)
}

// renderBorderedTable prints a PART 32 "Output Formats" style box-drawing
// table (┌─┬─┐ borders) to w. Column widths are sized to the longest cell
// (header or data) per column, mirroring the AI.md --output table example.
func renderBorderedTable(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len([]rune(h))
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := len([]rune(cell)); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	border := func(left, mid, right string) {
		fmt.Fprint(w, left)
		for i, wd := range widths {
			fmt.Fprint(w, strings.Repeat("─", wd+2))
			if i < len(widths)-1 {
				fmt.Fprint(w, mid)
			}
		}
		fmt.Fprintln(w, right)
	}

	printRow := func(cells []string) {
		fmt.Fprint(w, "│")
		for i, wd := range widths {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			pad := wd - len([]rune(cell))
			if pad < 0 {
				pad = 0
			}
			fmt.Fprintf(w, " %s%s │", cell, strings.Repeat(" ", pad))
		}
		fmt.Fprintln(w)
	}

	border("┌", "┬", "┐")
	printRow(headers)
	border("├", "┼", "┤")
	for _, row := range rows {
		printRow(row)
	}
	border("└", "┴", "┘")
}

// defaultServerURL falls back to the embedded OfficialSite (site.txt at build time)
// when --server, $PASTEBIN_SERVER_PRIMARY, and cli.yml are all unset. Never persisted to cli.yml.
func defaultServerURL(resolved, official string) string {
	if resolved == "" {
		return official
	}
	return resolved
}

// printNoServerError prints the exact multi-line "no server configured"
// message documented in AI.md PART 32 "Server Address Resolution" (line
// 45115-45126), for projects without a compiled {official_site} default.
func printNoServerError() {
	bin := filepath.Base(os.Args[0])
	fmt.Fprintf(os.Stderr, "Error: no server configured\n\n")
	fmt.Fprintf(os.Stderr, "To configure a server, run:\n")
	fmt.Fprintf(os.Stderr, "  %s --server https://your-server.example.com list\n\n", bin)
	fmt.Fprintf(os.Stderr, "This will save the server address for future commands.\n")
	fmt.Fprintf(os.Stderr, "Or edit ~/.config/apimgr/%s/cli.yml directly.\n", projectName)
}

// cmdUpdate handles 'pastebin-cli --update check|yes'.
func (c *client) cmdUpdate(action string) {
	if c.server == "" {
		printNoServerError()
		os.Exit(exitConnection)
	}

	httpClient := &http.Client{Timeout: c.clientTimeout()}
	req, err := http.NewRequest(http.MethodGet, c.url("/api/autodiscover"), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: update: build request: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))
	if c.lang != "" {
		req.Header.Set("Accept-Language", c.lang)
	}

	resp, err := c.doWithRetry(httpClient, req)
	if err != nil {
		printConnectionError(c.server, err)
	}
	defer resp.Body.Close()

	var disc autodiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		fmt.Fprintf(os.Stderr, "%s: update: decode: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}

	osArch := runtime.GOOS + "-" + runtime.GOARCH
	info, ok := disc.CLIVersions[osArch]
	if !ok {
		fmt.Printf("no CLI binary available for %s\n", osArch)
		return
	}

	if !versionLessThan(Version, info.Version) {
		fmt.Printf("pastebin-cli is up to date (%s)\n", Version)
		return
	}

	fmt.Printf("update available: %s → %s\n", Version, info.Version)
	if action != "yes" {
		fmt.Printf("run 'pastebin-cli update yes' to install\n")
		return
	}

	if err := c.downloadAndApplyUpdate(
		fmt.Sprintf("%s/cli/binaries/pastebin-cli-%s-%s", c.server, runtime.GOOS, runtime.GOARCH),
		info.SHA256,
	); err != nil {
		fmt.Fprintf(os.Stderr, "%s: update failed: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}
}

// downloadAndApplyUpdate downloads the CLI binary, verifies SHA-256, and replaces the current binary.
func (c *client) downloadAndApplyUpdate(downloadURL, expectedSHA string) error {
	// Determine current binary path.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	// Download to ${TMPDIR:-/tmp}/apimgr/pastebin-XXXXXX/cli.update.tmp, verify
	// there, then atomically replace the installed binary (PART 32 step 3–5).
	tmpDir, tmpFile, err := updateTempDir()
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	httpClient := &http.Client{Timeout: 5 * time.Minute}
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		f.Close()
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	// Write and hash simultaneously so we only stream the body once.
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, h), resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}

	// Verify SHA-256.
	got := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(got, expectedSHA) {
		return fmt.Errorf("SHA-256 mismatch: got %s, want %s", got, expectedSHA)
	}

	// Atomically replace the current binary. os.Rename is atomic only within a
	// filesystem; the temp dir is usually a separate tmpfs, so on a cross-device
	// error stage the verified binary beside the target and rename from there.
	if err := os.Rename(tmpFile, exe); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("replace binary: %w", err)
		}
		if err := replaceCrossDevice(tmpFile, exe); err != nil {
			return err
		}
	}

	// Re-exec or inform the user on Windows.
	return reExec(exe)
}

// updateTempDir creates the PART 32 CLI update staging directory
// (${TMPDIR:-/tmp}/apimgr/pastebin-XXXXXX/) and returns both the directory and
// the cli.update.tmp file path inside it. The caller removes the directory.
func updateTempDir() (dir, file string, err error) {
	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", "", fmt.Errorf("create temp base %s: %w", base, err)
	}
	dir, err = os.MkdirTemp(base, projectName+"-*")
	if err != nil {
		return "", "", fmt.Errorf("create temp dir: %w", err)
	}
	return dir, filepath.Join(dir, "cli.update.tmp"), nil
}

// replaceCrossDevice copies src to a staging file beside dst (same filesystem)
// and atomically renames it over dst. Used when src and dst live on different
// filesystems and a direct rename returns EXDEV.
func replaceCrossDevice(src, dst string) error {
	staging := dst + ".new"
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open staged binary: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(staging, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(staging)
		return fmt.Errorf("copy staged binary: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(staging)
		return fmt.Errorf("close staging file: %w", err)
	}
	if err := os.Rename(staging, dst); err != nil {
		os.Remove(staging)
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

func (c *client) url(path string) string {
	return c.server + path
}

// apiVer returns c.apiVersion, falling back to the compiled apiVersion const
// when the client was constructed without an explicit cli.yml server.api_version.
func (c *client) apiVer() string {
	if c.apiVersion != "" {
		return c.apiVersion
	}
	return apiVersion
}

// clientTimeout returns c.timeout, falling back to the compiled default when
// the client was constructed without an explicit cli.yml server.timeout.
func (c *client) clientTimeout() time.Duration {
	if c.timeout > 0 {
		return c.timeout
	}
	return defaultClientTimeout
}

func (c *client) get(path string) (*http.Response, error) {
	httpClient := &http.Client{Timeout: c.clientTimeout()}
	req, err := http.NewRequest(http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))
	req.Header.Set("Accept", "application/json")
	if c.lang != "" {
		req.Header.Set("Accept-Language", c.lang)
	}
	c.setAuth(req)
	return c.doWithRetry(httpClient, req)
}

func (c *client) postJSON(path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: c.clientTimeout()}
	req, err := http.NewRequest(http.MethodPost, c.url(path), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))
	if c.lang != "" {
		req.Header.Set("Accept-Language", c.lang)
	}
	c.setAuth(req)
	return c.doWithRetry(httpClient, req)
}

// ─── Language detection ───────────────────────────────────────────────────────

func detectLang(filename string) string {
	ext := strings.ToLower(filename)
	if i := strings.LastIndex(ext, "."); i != -1 {
		ext = ext[i+1:]
	}
	m := map[string]string{
		"go": "go", "py": "python", "js": "javascript", "ts": "typescript",
		"rs": "rust", "java": "java", "c": "c", "cpp": "cpp", "cc": "cpp",
		"cs": "csharp", "php": "php", "rb": "ruby", "sh": "bash",
		"bash": "bash", "zsh": "bash", "ps1": "powershell",
		"html": "html", "htm": "html", "css": "css", "json": "json",
		"yaml": "yaml", "yml": "yaml", "toml": "toml", "xml": "xml",
		"sql": "sql", "md": "markdown", "txt": "text",
	}
	if lang, ok := m[ext]; ok {
		return lang
	}
	return "text"
}

// ─── Misc ─────────────────────────────────────────────────────────────────────

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// firstNonEmpty returns s if non-empty, otherwise def.
func firstNonEmpty(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

func printUsage() {
	binaryName := filepath.Base(os.Args[0])
	// AI.md PART 32 "Server Address Resolution" (line 45128-45136): when the
	// binary was built with a compiled {official_site}, --help shows it as
	// the default; otherwise the flag stays marked required.
	serverFlagHelp := "Server base URL (required; or set $PASTEBIN_SERVER_PRIMARY)"
	if OfficialSite != "" {
		serverFlagHelp = fmt.Sprintf("Server base URL (default: %s)", OfficialSite)
	}
	fmt.Fprintf(os.Stderr, `%s %s — command-line client for the pastebin service

USAGE
    %s [--server URL] [--json] <command> [flags] [args]

COMMANDS
    create [file]        Create paste from stdin or file; prints URL and delete token
    get <id>             Fetch and print raw paste content
    delete <id> <token>  Delete paste using its delete token
    list [--limit N]     List recent public pastes
    update [check|yes]   Check for or apply CLI updates (default: check)

    When no command is given in an interactive terminal, the TUI launches automatically.

CREATE FLAGS
    --lang <lang>        Syntax language (default: text)
    --expiry <duration>  1h 1d 1w 1m 3m 6m 1y 2y never, or seconds (default: never)
    --burn <n>           Delete after N views; 0 = disabled (default: 0)
    --unlisted           Create as unlisted (not shown in recent pastes)
    --title <title>      Paste title (optional)
    --link                Create as a link — arg/stdin must be an http:// or
                          https:// URL; visiting the paste 302-redirects there

LIST FLAGS
    --limit <n>          Number of pastes per page (default: 20)
    --page <n>           Page number (default: 1)

GLOBAL FLAGS
    --server <url>       %s
    --token <token>      Operator/owner API token (or set $PASTEBIN_TOKEN)
    --token-file <file>  Read the API token from file
    --json               Output machine-readable JSON
    --color <when>       Color output: auto, yes, no (default: auto; honors NO_COLOR)
    --lang <code>        Output language (default: auto-detect from LANG)
    --debug              Enable debug output
    --update check|yes   Check for or apply CLI updates
    --version            Print version
    --shell completions [SHELL]  Print shell completions
    --shell init [SHELL]         Print shell init command (eval-able)
    --shell --help               Show shell integration help

EXAMPLES
    PASTEBIN_SERVER_PRIMARY=https://paste.example.com %s create --lang text < file.txt
    %s --server https://paste.example.com create --lang go myfile.go
    %s --server https://paste.example.com get abc12345
    %s --server https://paste.example.com delete abc12345 <delete-token>
    %s --server https://paste.example.com list --limit 10

`, binaryName, Version, binaryName, serverFlagHelp, binaryName, binaryName, binaryName, binaryName, binaryName)
}
