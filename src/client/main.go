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
	"encoding/json"
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
	"text/tabwriter"
	"time"

	"github.com/apimgr/pastebin/src/paths"
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

// ─── CLI config (cli.yml) ─────────────────────────────────────────────────────

// cliConfig mirrors the structure of cli.yml.
type cliConfig struct {
	Server  string `yaml:"server"`
	Update  struct {
		Auto          bool   `yaml:"auto"`
		CheckInterval string `yaml:"check_interval"`
		Channel       string `yaml:"channel"`
	} `yaml:"update"`
	Display struct {
		Mode string `yaml:"mode"`
	} `yaml:"display"`
}

// cliConfigPath returns the platform-correct path to cli.yml.
func cliConfigPath() string {
	if p := os.Getenv("CLI_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "apimgr", projectName, "cli.yml")
	}
	return filepath.Join(paths.GetConfigDir(projectName), "cli.yml")
}

// loadCLIConfig reads cli.yml; returns zero-value config if absent.
func loadCLIConfig() (cliConfig, error) {
	var cfg cliConfig
	cfg.Update.Channel = "stable"
	cfg.Update.CheckInterval = "per_invocation"
	cfg.Display.Mode = "auto"

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
	return os.WriteFile(p, data, 0o644)
}

// saveIfEmptyOrInvalid updates dst with src when dst is empty or invalid, and
// returns both the resolved value and whether it should be persisted.
// Implements PART 32 Flag-to-Config Save Rules.
func saveIfEmptyOrInvalid(current, flagValue string, validate func(string) bool) (resolved string, persist bool) {
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

	// Config-only flags that still allow TUI launch.
	configFlags := map[string]bool{
		"--config": true, "--server": true, "--token": true, "--debug": true,
		"--color": true, "--json": true, "--lang": true,
	}

	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return "cli"
		}
		parts := strings.SplitN(arg, "=", 2)
		if !configFlags[parts[0]] {
			return "cli"
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
func checkCLIUpdate(serverURL string) error {
	if serverURL == "" || Version == "dev" {
		return nil
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(serverURL, "/")+"/api/autodiscover", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))

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
			"this CLI is too old; the server requires %s — run 'pastebin-cli --update yes' to upgrade",
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

func main() {
	log.SetFlags(0)
	log.SetPrefix(filepath.Base(os.Args[0]) + ": ")

	// Load cli.yml.
	fileCfg, err := loadCLIConfig()
	if err != nil {
		log.Printf("warning: could not load cli.yml: %v", err)
	}

	server := flag.String("server", envOrDefault("PASTEBIN_SERVER", fileCfg.Server), "server base URL")
	asJSON := flag.Bool("json", false, "machine-readable JSON output")
	colorFlag := flag.String("color", "auto", "color output: auto, yes, no")
	showVersion := flag.Bool("version", false, "print version and exit")
	showHelp := flag.Bool("help", false, "show help and exit")
	debugFlag := flag.Bool("debug", false, "enable debug output")
	doUpdate := flag.String("update", "", "check for CLI updates: 'check' or 'yes'")
	// PART 32: --lang sets the output/UI language; default auto-detects from the LANG env var.
	langFlag := flag.String("lang", "auto", "output language code (default: auto-detect from LANG)")

	// -h and -v are aliases for --help and --version.
	flag.BoolVar(showHelp, "h", false, "show help and exit")
	flag.BoolVar(showVersion, "v", false, "print version and exit")

	flag.Usage = printUsage
	flag.Parse()

	// Honour NO_COLOR env var (https://no-color.org/) and --color flag.
	// Spec PART 8 values: auto, yes, no. always/never are tolerated aliases.
	switch *colorFlag {
	case "no", "never":
		os.Setenv("NO_COLOR", "1")
	case "yes", "always":
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
				os.Exit(1)
			}
		case "completions":
			shellShell := ""
			if len(args) >= 3 {
				shellShell = args[2]
			}
			if err := shell.PrintClientCompletions(filepath.Base(os.Args[0]), shellShell); err != nil {
				fmt.Fprintf(os.Stderr, "%s: --shell completions: %v\n", filepath.Base(os.Args[0]), err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "%s: --shell: unknown subcommand %q\n", filepath.Base(os.Args[0]), shellArg)
			fmt.Fprintf(os.Stderr, "Run '%s --shell --help' for usage.\n", filepath.Base(os.Args[0]))
			os.Exit(1)
		}
		return
	}

	// Apply SaveIfEmptyOrInvalid: persist server to cli.yml when config was empty.
	// Use the current parsed value of --server as the flagValue.
	resolved, shouldPersist := saveIfEmptyOrInvalid(fileCfg.Server, *server, isValidURL)
	if shouldPersist && resolved != "" {
		fileCfg.Server = resolved
		if err := saveCLIConfig(fileCfg); err != nil {
			log.Printf("warning: could not save cli.yml: %v", err)
		}
	}
	if resolved != "" {
		*server = resolved
	}

	// Handle --update flag.
	locale := detectLocale(*langFlag)

	if *doUpdate != "" {
		c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON, lang: locale}
		c.cmdUpdate(*doUpdate)
		return
	}

	// Auto-detect display mode per PART 32.
	mode := detectMode(args)
	if mode == "tui" {
		runTUI(*server, fileCfg)
		return
	}

	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	if *server == "" {
		log.Fatal("no server URL set — use --server <url> or set $PASTEBIN_SERVER")
	}

	// Check for CLI updates (non-blocking; only blocks on min_version violation).
	if err := checkCLIUpdate(*server); err != nil {
		log.Fatal(err)
	}

	c := &client{server: strings.TrimRight(*server, "/"), asJSON: *asJSON, lang: locale}

	switch args[0] {
	case "create":
		c.cmdCreate(args[1:])
	case "get":
		c.cmdGet(args[1:])
	case "delete", "del", "rm":
		c.cmdDelete(args[1:])
	case "list", "ls":
		c.cmdList(args[1:])
	case "version":
		binaryName := filepath.Base(os.Args[0])
		fmt.Printf("%s %s (commit %s, built %s)\n", binaryName, Version, CommitID, BuildDate)
	default:
		log.Fatalf("unknown command %q (try: create, get, delete, list)", args[0])
	}
}

// runTUI launches the interactive TUI mode.
// When no server is configured it acts as a setup wizard.
// NOTE: full bubbletea TUI implementation is tracked in AUDIT.AI.md.
func runTUI(server string, cfg cliConfig) {
	binaryName := filepath.Base(os.Args[0])
	if server == "" {
		fmt.Printf("%s: interactive setup\n", binaryName)
		fmt.Printf("No server configured. Set one with:\n")
		fmt.Printf("  %s --server https://your-server.example.com list\n", binaryName)
		fmt.Printf("  or export PASTEBIN_SERVER=https://your-server.example.com\n")
		fmt.Printf("  or edit %s\n", cliConfigPath())
		os.Exit(0)
	}

	// Auto-update check in TUI mode.
	if err := checkCLIUpdate(server); err != nil {
		log.Fatal(err)
	}

	// TUI not yet implemented — fall back to help.
	fmt.Fprintf(os.Stderr, "%s: TUI mode detected but not yet implemented.\n", binaryName)
	fmt.Fprintf(os.Stderr, "Use command-line mode: %s <command> [args]\n", binaryName)
	printUsage()
	os.Exit(1)
}

// ─── Client ───────────────────────────────────────────────────────────────────

type client struct {
	server string
	asJSON bool
	// lang is the resolved output/UI locale sent as the Accept-Language header (PART 30/32).
	lang string
}

// detectLocale resolves the output locale from the --lang flag, falling back to the
// LANG / LC_ALL environment variables, then to "en". A value of "auto" or "" triggers
// environment detection. The server silently falls back to English for unsupported codes.
func detectLocale(flagVal string) string {
	v := strings.TrimSpace(flagVal)
	if v != "" && v != "auto" {
		return v
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
		log.Fatal(err)
	}

	var content []byte
	var err error

	if fs.NArg() > 0 {
		content, err = os.ReadFile(fs.Arg(0))
		if err != nil {
			log.Fatalf("read file: %v", err)
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
			log.Fatalf("read stdin: %v", err)
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

	resp, err := c.postJSON("/api/v1/pastes", body)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("decode response: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		errMsg, _ := result["error"].(string)
		log.Fatalf("server error %d: %s", resp.StatusCode, errMsg)
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
		log.Fatal(err)
	}
	if fs.NArg() < 1 {
		log.Fatal("usage: get <id>")
	}
	id := fs.Arg(0)

	resp, err := c.get("/raw/" + id)
	if err != nil {
		log.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Fatalf("paste %q not found or has expired", id)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("server returned %d", resp.StatusCode)
	}

	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		log.Fatalf("read response: %v", err)
	}
}

func (c *client) cmdDelete(args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		log.Fatal(err)
	}
	if fs.NArg() < 2 {
		log.Fatal("usage: delete <id> <token>")
	}
	id, token := fs.Arg(0), fs.Arg(1)

	req, err := http.NewRequest(
		http.MethodDelete,
		c.url("/api/v1/pastes/"+id+"?token="+url.QueryEscape(token)),
		nil,
	)
	if err != nil {
		log.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("delete: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Fatalf("paste %q not found or invalid token", id)
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("server returned %d", resp.StatusCode)
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
		log.Fatal(err)
	}

	resp, err := c.get(fmt.Sprintf("/api/v1/pastes?page=%d&limit=%d", *page, *limit))
	if err != nil {
		log.Fatalf("list: %v", err)
	}
	defer resp.Body.Close()

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
		log.Fatalf("decode: %v", err)
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

// cmdUpdate handles 'pastebin-cli --update check|yes'.
func (c *client) cmdUpdate(action string) {
	if c.server == "" {
		log.Fatal("no server URL set — use --server <url> or set $PASTEBIN_SERVER")
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, c.url("/api/autodiscover"), nil)
	if err != nil {
		log.Fatalf("update: build request: %v", err)
	}
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", projectName, Version))

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Fatalf("update: autodiscover: %v", err)
	}
	defer resp.Body.Close()

	var disc autodiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&disc); err != nil {
		log.Fatalf("update: decode: %v", err)
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
		fmt.Printf("run 'pastebin-cli --update yes' to install\n")
		return
	}

	fmt.Printf("downloading pastebin-cli %s for %s…\n", info.Version, osArch)
	log.Fatal("auto-install not yet implemented; download manually from server")
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
    --server <url>       Server base URL (required; or set $PASTEBIN_SERVER)
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
    PASTEBIN_SERVER=https://paste.example.com %s create --lang text < file.txt
    %s --server https://paste.example.com create --lang go myfile.go
    %s --server https://paste.example.com get abc12345
    %s --server https://paste.example.com delete abc12345 <delete-token>
    %s --server https://paste.example.com list --limit 10

`, binaryName, Version, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName)
}
