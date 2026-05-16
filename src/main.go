package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/mode"
	"github.com/apimgr/pastebin/src/paths"
	"github.com/apimgr/pastebin/src/scheduler"
	"github.com/apimgr/pastebin/src/server"
)

// Version, CommitID, BuildDate, and OfficialSite are injected at build time via -ldflags.
var (
	Version      = "dev"
	CommitID     = "unknown"
	BuildDate    = "unknown"
	OfficialSite = ""
)

const appName = "pastebin"

func main() {
	binaryName := filepath.Base(os.Args[0])

	// Pre-process args: normalise -flag to --flag and expand -h/-v aliases.
	args := normalizeArgs(os.Args[1:])

	// Simple manual flag parser so we control order and aliases.
	var (
		portFlag     string
		addressFlag  string
		modeFlag     string
		configFlag   string
		dataFlag     string
		logFlag      string
		cacheFlag    string
		backupFlag   string
		pidFlag      string
		baseurlFlag  string
		colorFlag    string
		langFlag     string
		showVersion  bool
		showStatus   bool
		showHelp     bool
		daemonFlag   bool
		debugFlag    bool
		cleanExpired bool
	)

	// Stub commands — recognised but not yet fully implemented.
	var (
		shellCmd       string
		serviceCmd     string
		maintenanceCmd string
		updateCmd      string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		val := func() string {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				return args[i]
			}
			return ""
		}

		switch arg {
		case "--help":
			showHelp = true
		case "--version":
			showVersion = true
		case "--status":
			showStatus = true
		case "--daemon":
			daemonFlag = true
		case "--debug":
			debugFlag = true
		case "--clean-expired":
			cleanExpired = true
		case "--port":
			portFlag = val()
		case "--address":
			addressFlag = val()
		case "--mode":
			modeFlag = val()
		case "--config":
			configFlag = val()
		case "--data":
			dataFlag = val()
		case "--log":
			logFlag = val()
		case "--cache":
			cacheFlag = val()
		case "--backup":
			backupFlag = val()
		case "--pid":
			pidFlag = val()
		case "--baseurl":
			baseurlFlag = val()
		case "--color":
			colorFlag = val()
		case "--lang":
			langFlag = val()
		case "--shell":
			shellCmd = val()
		case "--service":
			serviceCmd = val()
		case "--maintenance":
			maintenanceCmd = val()
		case "--update":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				updateCmd = args[i]
			} else {
				updateCmd = "check"
			}
		default:
			if strings.HasPrefix(arg, "--") {
				fmt.Fprintf(os.Stderr, "%s: unknown flag: %s\n", binaryName, arg)
				fmt.Fprintf(os.Stderr, "Run '%s --help' for usage.\n", binaryName)
				os.Exit(2)
			}
		}
	}

	// Apply --color flag before any output.
	applyColor(colorFlag)

	if showHelp {
		printHelp(binaryName)
		return
	}

	if showVersion {
		fmt.Printf("%s %s (%s)\n", binaryName, Version, CommitID)
		if BuildDate != "unknown" {
			fmt.Printf("Built: %s\n", BuildDate)
		}
		if OfficialSite != "" {
			fmt.Printf("Site:  %s\n", OfficialSite)
		}
		return
	}

	// ── Stub commands ────────────────────────────────────────────────────────

	if shellCmd != "" {
		fmt.Fprintf(os.Stderr, "%s: --shell is not yet implemented\n", binaryName)
		os.Exit(1)
	}

	if serviceCmd != "" {
		fmt.Fprintf(os.Stderr, "%s: --service is not yet implemented\n", binaryName)
		os.Exit(1)
	}

	if maintenanceCmd != "" {
		fmt.Fprintf(os.Stderr, "%s: --maintenance is not yet implemented\n", binaryName)
		os.Exit(1)
	}

	if updateCmd != "" {
		fmt.Fprintf(os.Stderr, "%s: --update is not yet implemented\n", binaryName)
		os.Exit(1)
	}

	_ = daemonFlag // daemonize not yet implemented
	_ = langFlag   // language selection not yet implemented

	// ── Application mode ─────────────────────────────────────────────────────

	if err := mode.Initialize(modeFlag); err != nil {
		log.Printf("warning: %v", err)
	}

	if debugFlag {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("debug mode enabled")
	}

	// ── Directory resolution ─────────────────────────────────────────────────

	configDir := paths.GetConfigDir(appName)
	dataDir := paths.GetDataDir(appName)
	logsDir := paths.GetLogsDir(appName)
	cacheDir := paths.GetCacheDir(appName)

	if dataFlag != "" {
		dataDir = dataFlag
	}
	if logFlag != "" {
		logsDir = logFlag
	}
	if cacheFlag != "" {
		cacheDir = cacheFlag
	}
	if backupFlag != "" {
		// backup dir override: store for future use
		_ = backupFlag
	}
	if pidFlag != "" {
		// pid file override: store for future use
		_ = pidFlag
	}

	for _, dir := range []string{configDir, dataDir, logsDir, cacheDir} {
		if err := paths.EnsureDir(dir); err != nil {
			log.Printf("warning: could not create directory %s: %v", dir, err)
		}
	}

	if showStatus {
		fmt.Printf("%s %s (%s)\n", binaryName, Version, CommitID)
		fmt.Printf("Mode:   %s\n", mode.Get())
		fmt.Printf("Config: %s\n", filepath.Join(configDir, "server.yml"))
		fmt.Printf("Data:   %s\n", dataDir)
		fmt.Printf("Logs:   %s\n", logsDir)
		fmt.Printf("Cache:  %s\n", cacheDir)
		return
	}

	// ── Load config ───────────────────────────────────────────────────────────

	cfgFile := filepath.Join(configDir, "server.yml")
	if configFlag != "" {
		cfgFile = configFlag
	}
	cfg, err := config.Load(cfgFile)
	if err != nil {
		log.Printf("warning: config load: %v", err)
	}

	// CLI flag overrides on config.
	if portFlag != "" {
		cfg.Server.Port = portFlag
	}
	if addressFlag != "" {
		cfg.Server.Address = addressFlag
	}
	if baseurlFlag != "" {
		cfg.Server.BaseURL = baseurlFlag
	}
	if modeFlag != "" {
		cfg.Server.Mode = modeFlag
	}

	// ── Database ──────────────────────────────────────────────────────────────

	if cfg.Database.Path == "" {
		cfg.Database.Path = filepath.Join(dataDir, "db", "server.db")
	}
	if err := paths.EnsureDir(filepath.Dir(cfg.Database.Path)); err != nil {
		log.Printf("warning: db dir: %v", err)
	}

	db, err := database.NewDatabase(cfg.Database.Type, cfg.Database.Path)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	log.Printf("%s %s — database: %s (%s)", appName, Version, db.Type(), cfg.Database.Path)

	// ── One-shot: clean expired pastes ────────────────────────────────────────

	if cleanExpired {
		n, err := db.DeleteExpiredPastes()
		if err != nil {
			log.Fatalf("clean expired: %v", err)
		}
		b, _ := db.DeleteBurnedPastes()
		log.Printf("deleted %d expired + %d burned pastes", n, b)
		return
	}

	// ── Background scheduler ──────────────────────────────────────────────────

	sched := scheduler.New()
	sched.AddTask("expire-pastes", 10*time.Minute, func() error {
		n, err := db.DeleteExpiredPastes()
		if err != nil {
			return err
		}
		b, _ := db.DeleteBurnedPastes()
		if n+b > 0 {
			log.Printf("scheduler: removed %d expired + %d burned pastes", n, b)
		}
		return nil
	})
	sched.Start()
	defer sched.Stop()

	// ── HTTP server ───────────────────────────────────────────────────────────

	srv := server.New(db, cfg, Version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("shutting down…")
		cancel()
	}()

	addr := cfg.Server.Address + ":" + cfg.Server.Port
	log.Printf("listening on %s", addr)

	if err := srv.Run(ctx, addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// normalizeArgs converts single-dash long flags (-flag) to double-dash (--flag)
// and expands -h → --help, -v → --version.
func normalizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "-h":
			out = append(out, "--help")
		case "-v":
			out = append(out, "--version")
		default:
			// Convert -flag to --flag (single dash long flags)
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && len(a) > 2 {
				out = append(out, "-"+a)
			} else {
				out = append(out, a)
			}
		}
	}
	return out
}

// applyColor applies the --color flag, updating NO_COLOR as needed.
func applyColor(v string) {
	switch v {
	case "never":
		os.Setenv("NO_COLOR", "1")
	case "always":
		os.Unsetenv("NO_COLOR")
	}
	// "auto" or empty: leave NO_COLOR as-is
}

func printHelp(name string) {
	fmt.Printf(`%s %s - a fast, public pastebin service

Usage:
  %s [flags]

Information:
  -h, --help                        Show help (--help for any command shows its help)
  -v, --version                     Show version
      --status                      Show server status and health

Shell Integration:
      --shell completions [SHELL]   Print shell completions
      --shell init [SHELL]          Print shell init command
      --shell --help                Show shell help

Server Configuration:
      --mode {production|development}  Application mode (default: production)
      --config DIR                  Config directory
      --data DIR                    Data directory
      --cache DIR                   Cache directory
      --log DIR                     Log directory
      --backup DIR                  Backup directory
      --pid FILE                    PID file path
      --address ADDR                Listen address (default: 0.0.0.0)
      --port PORT                   Listen port (default: 3010)
      --baseurl PATH                URL path prefix (default: /)
      --daemon                      Run as daemon (detach from terminal)
      --debug                       Enable debug mode
      --color {always|never|auto}   Color output (default: auto)
      --lang CODE                   Language for output (default: auto)

Service Management:
      --service CMD                 Service management (--service --help for details)
      --maintenance CMD             Maintenance operations (--maintenance --help for details)
      --update [CMD]                Check/perform updates (--update --help for details)

Maintenance:
      --clean-expired               Delete expired/burned pastes and exit

Run '%s <command> --help' for detailed help on any command.
`, name, Version, name, name)
}
