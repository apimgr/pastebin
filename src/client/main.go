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
	"github.com/apimgr/pastebin/src/shell"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Version, CommitID, BuildDate, and OfficialSite are injected at build time via -ldflags.
var (
	Version      = "dev"
	CommitID     = "unknown"
	BuildDate    = "unknown"
	OfficialSite = ""
)

// projectName is the hardcoded internal name used for User-Agent and config paths.
// Display uses filepath.Base(os.Args[0]) per PART 32.
const projectName = "pastebin"

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

// ─── CLI config (cli.yml) ─────────────────────────────────────────────────────

// cliConfig mirrors the complete structure of cli.yml (PART 32).
type cliConfig struct {
	Server struct {
		Primary string `yaml:"primary"`
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

// loadCLIConfig reads cli.yml; returns zero-value config if absent.
func loadCLIConfig() (cliConfig, error) {
	var cfg cliConfig
	cfg.Update.Channel = "stable"
	cfg.Update.CheckInterval = "per_invocation"
	cfg.Display.Mode = "auto"
	cfg.Output.Format = "text"
	cfg.Output.Color = "auto"
	cfg.TUI.Enabled = true
	cfg.TUI.Unicode = true
	cfg.Cache.TTL = "5m"
	cfg.Cache.MaxSize = 100
	cfg.Defaults.Expire = "never"
	cfg.Defaults.Syntax = "text"
	cfg.Defaults.Limit = 20

	data, err := os.ReadFile(cliConfigPath())
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
	p := cliConfigPath()
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

// ─── Display mode detection ───────────────────────────────────────────────────

// detectMode returns "tui", "cli", or "plain" based on environment and args.
// Implements PART 32 Automatic Mode Detection rules.
func detectMode(args []string) string {
	// Exit-immediately flags — never TUI.
	for _, arg := range args {
		switch arg {
		case "-h", "--help", "-v", "--version":
			return "cli"
		}
	}

	// Not a terminal → plain output (piped, cron, scripts).
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "plain"
	}

	// Config-only flags that still allow TUI launch (value: flag consumes the next arg).
	configFlags := map[string]bool{
		"--config": true, "--server": true, "--token": true, "--debug": true,
		"--color": true, "--json": true, "--lang": true,
	}
	valueFlags := map[string]bool{
		"--config": true, "--server": true, "--token": true,
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
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix(filepath.Base(os.Args[0]) + ": ")

	ensureDirs()

	// Load cli.yml.
	fileCfg, err := loadCLIConfig()
	if err != nil {
		log.Printf("warning: could not load cli.yml: %v", err)
	}

	server := flag.String("server", envOrDefault("PASTEBIN_SERVER_PRIMARY", fileCfg.Server.Primary), "server base URL")
	asJSON := flag.Bool("json", false, "machine-readable JSON output")
	colorFlag := flag.String("color", "auto", "color output: auto, yes, no")
	showVersion := flag.Bool("version", false, "print version and exit")
	showHelp := flag.Bool("help", false, "show help and exit")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	doUpdate := flag.String("update", "", "check for CLI updates: 'check' or 'yes'")
	// PART 32: --lang sets the output/UI language; default auto-detects from the LANG env var.
	langFlag := flag.String("lang", "auto", "output language code (default: auto-detect from LANG)")
	// PART 32: operator/owner API token. Priority: --token flag → PASTEBIN_TOKEN env → cli.yml auth.token.
	tokenFlag := flag.String("token", "", "operator/owner API token (or set PASTEBIN_TOKEN)")

	// -h and -v are aliases for --help and --version.
	flag.BoolVar(showHelp, "h", false, "show help and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit")

	flag.Usage = printUsage
	flag.Parse()

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
		binaryName := filepath.Base(os.Args[0])
		fmt.Printf("%s %s (commit %s, built %s)\n", binaryName, Version, CommitID, BuildDate)
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

	// Resolve the API token (PART 32 priority): --token flag → PASTEBIN_TOKEN env → cli.yml auth.token.
	// The env var never persists; the --token flag saves to cli.yml only when the stored token is empty/invalid.
	token := *tokenFlag
	if token == "" {
		token = os.Getenv("PASTEBIN_TOKEN")
	}
	if token == "" {
		token = fileCfg.Auth.Token
	}
	if *tokenFlag != "" {
		if resolvedTok, persist := saveIfUnset(fileCfg.Auth.Token, *tokenFlag, func(s string) bool { return s != "" }); persist && resolvedTok != "" {
			fileCfg.Auth.Token = resolvedTok
			if err := saveCLIConfig(fileCfg); err != nil {
				log.Printf("warning: could not save cli.yml: %v", err)
			}
		}
	}

	// Handle --update flag.
	locale := detectLocale(*langFlag, fileCfg.Defaults.Lang)

	if *doUpdate != "" {
		c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON, lang: locale, token: token}
		c.cmdUpdate(*doUpdate)
		return
	}

	// Auto-detect display mode per PART 32.
	mode := detectMode(args)
	if mode == "tui" {
		runTUI(*server, locale, fileCfg)
		return
	}

	if len(args) == 0 {
		printUsage()
		os.Exit(exitUsage)
	}

	if *server == "" {
		fmt.Fprintf(os.Stderr, "%s: no server URL set — use --server <url> or set $PASTEBIN_SERVER_PRIMARY\n", filepath.Base(os.Args[0]))
		os.Exit(exitConnection)
	}

	// Check for CLI updates (non-blocking; only blocks on min_version violation).
	if err := checkCLIUpdate(*server, locale); err != nil {
		fmt.Fprintf(os.Stderr, "%s: update check: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
	}

	c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON, lang: locale, token: token}

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
		binaryName := filepath.Base(os.Args[0])
		fmt.Printf("%s %s (commit %s, built %s)\n", binaryName, Version, CommitID, BuildDate)
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
		CfgPath: cliConfigPath(),
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
	// lang is the resolved output/UI locale sent as the Accept-Language header (PART 30/32).
	lang string
	// token is the resolved operator/owner API token sent as the Authorization bearer header (PART 32).
	token string
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
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	lang := fs.String("lang", "text", "syntax language")
	expiry := fs.String("expiry", "never", "expiry: 1h 1d 1w 1m 3m 6m 1y 2y never or seconds")
	burn := fs.Int("burn", 0, "delete after N views (0 = disabled)")
	unlisted := fs.Bool("unlisted", false, "create as unlisted (not shown in recent)")
	title := fs.String("title", "", "paste title")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: create: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}

	var content []byte
	var err error

	if fs.NArg() > 0 {
		content, err = os.ReadFile(fs.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: read file: %v\n", filepath.Base(os.Args[0]), err)
			os.Exit(exitGeneral)
		}
		// Auto-detect language from extension if not set.
		if *lang == "text" {
			*lang = detectLang(fs.Arg(0))
		}
		if *title == "" {
			*title = fs.Arg(0)
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
		"title":      *title,
		"language":   *lang,
		"expires_in": *expiry,
		"burn_after": *burn,
		"visibility": vis,
	}

	// Binary files (images, archives, etc.) cannot travel as raw bytes inside a
	// JSON string — invalid UTF-8 gets replaced and corrupts the data. Detect
	// the MIME type, base64-encode, and tell the server via content_type.
	sample := content
	if len(sample) > 512 {
		sample = sample[:512]
	}
	if detected := http.DetectContentType(sample); !strings.HasPrefix(detected, "text/") {
		body["content"] = base64.StdEncoding.EncodeToString(content)
		body["content_type"] = detected
	}

	resp, err := c.postJSON("/api/v1/pastes", body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: create: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "%s: decode response: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		errMsg, _ := result["error"].(string)
		fmt.Fprintf(os.Stderr, "%s: create: authentication failed (%d): %s\n", filepath.Base(os.Args[0]), resp.StatusCode, errMsg)
		os.Exit(exitAuth)
	}
	if resp.StatusCode != http.StatusCreated {
		errMsg, _ := result["error"].(string)
		fmt.Fprintf(os.Stderr, "%s: create: server error %d: %s\n", filepath.Base(os.Args[0]), resp.StatusCode, errMsg)
		os.Exit(exitGeneral)
	}

	if c.asJSON {
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
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: get: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}
	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "%s: usage: get <id>\n", filepath.Base(os.Args[0]))
		os.Exit(exitUsage)
	}
	id := fs.Arg(0)

	resp, err := c.get("/raw/" + url.PathEscape(id))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: get: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "%s: paste %q not found or has expired\n", filepath.Base(os.Args[0]), id)
		os.Exit(exitNotFound)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Fprintf(os.Stderr, "%s: get: authentication required (%d)\n", filepath.Base(os.Args[0]), resp.StatusCode)
		os.Exit(exitAuth)
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
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
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
		c.url("/api/v1/pastes/"+url.PathEscape(id)+"?token="+url.QueryEscape(token)),
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

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: delete: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintf(os.Stderr, "%s: paste %q not found or invalid token\n", filepath.Base(os.Args[0]), id)
		os.Exit(exitNotFound)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Fprintf(os.Stderr, "%s: delete: authentication failed (%d)\n", filepath.Base(os.Args[0]), resp.StatusCode)
		os.Exit(exitAuth)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "%s: delete: server returned %d\n", filepath.Base(os.Args[0]), resp.StatusCode)
		os.Exit(exitGeneral)
	}

	if c.asJSON {
		io.Copy(os.Stdout, resp.Body)
		return
	}
	fmt.Printf("paste %s deleted\n", id)
}

func (c *client) cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	limit := fs.Int("limit", 20, "number of pastes to list (max 100)")
	page := fs.Int("page", 1, "page number")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "%s: list: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitUsage)
	}

	resp, err := c.get(fmt.Sprintf("/api/v1/pastes?page=%d&limit=%d", *page, *limit))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: list: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		fmt.Fprintf(os.Stderr, "%s: list: authentication required (%d)\n", filepath.Base(os.Args[0]), resp.StatusCode)
		os.Exit(exitAuth)
	}

	var result struct {
		Pastes []struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			Language  string    `json:"language"`
			Views     int       `json:"views"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"pastes"`
		Pagination struct {
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "%s: list: decode: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}

	if c.asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return
	}

	if len(result.Pastes) == 0 {
		fmt.Println("no pastes found")
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
		result.Pagination.Total, *page, result.Pagination.TotalPages)
}

// defaultServerURL falls back to the embedded OfficialSite (site.txt at build time)
// when --server, $PASTEBIN_SERVER_PRIMARY, and cli.yml are all unset. Never persisted to cli.yml.
func defaultServerURL(resolved, official string) string {
	if resolved == "" {
		return official
	}
	return resolved
}

// cmdUpdate handles 'pastebin-cli --update check|yes'.
func (c *client) cmdUpdate(action string) {
	if c.server == "" {
		fmt.Fprintf(os.Stderr, "%s: no server URL set — use --server <url> or set $PASTEBIN_SERVER_PRIMARY\n", filepath.Base(os.Args[0]))
		os.Exit(exitConnection)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, c.url("/api/autodiscover"), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: update: build request: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitGeneral)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))
	if c.lang != "" {
		req.Header.Set("Accept-Language", c.lang)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: update: autodiscover: %v\n", filepath.Base(os.Args[0]), err)
		os.Exit(exitConnection)
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

func (c *client) get(path string) (*http.Response, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
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
	return httpClient.Do(req)
}

func (c *client) postJSON(path string, body interface{}) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
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
	return httpClient.Do(req)
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

func printUsage() {
	binaryName := filepath.Base(os.Args[0])
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

LIST FLAGS
    --limit <n>          Number of pastes per page (default: 20)
    --page <n>           Page number (default: 1)

GLOBAL FLAGS
    --server <url>       Server base URL (required; or set $PASTEBIN_SERVER_PRIMARY)
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

`, binaryName, Version, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName)
}
