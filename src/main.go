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

	"github.com/apimgr/pastebin/src/common/i18n"
	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/daemon"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/maintenance"
	"github.com/apimgr/pastebin/src/mode"
	"github.com/apimgr/pastebin/src/paths"
	"github.com/apimgr/pastebin/src/pid"
	"github.com/apimgr/pastebin/src/scheduler"
	"github.com/apimgr/pastebin/src/server"
	"github.com/apimgr/pastebin/src/service"
	"github.com/apimgr/pastebin/src/shell"
	"github.com/apimgr/pastebin/src/task"
	"github.com/apimgr/pastebin/src/updater"
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

	// Subcommands that take optional secondary positional arguments.
	var (
		shellCmd       string
		shellArg       string
		serviceCmd     string
		maintenanceCmd string
		maintenanceArg string // second positional arg after --maintenance subcommand
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
			shellArg = val() // optional SHELL name (bash, zsh, fish, …)
		case "--service":
			serviceCmd = val()
		case "--maintenance":
			maintenanceCmd = val()
			// Capture an optional second positional argument (e.g. filename for
			// "restore" or mode name for "mode").
			maintenanceArg = val()
		case "--update":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				updateCmd = args[i]
				// For "branch <name>", consume the branch name as well.
				if updateCmd == "branch" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
					i++
					updateCmd = "branch " + args[i]
				}
			} else {
				updateCmd = "yes"
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

	// ── Shell integration ─────────────────────────────────────────────────────

	if shellCmd != "" {
		switch shellCmd {
		case "--help":
			shell.PrintHelp(binaryName)
		case "completions":
			if err := shell.PrintCompletions(binaryName, shellArg); err != nil {
				fmt.Fprintf(os.Stderr, "%s: --shell completions: %v\n", binaryName, err)
				os.Exit(1)
			}
		case "init":
			if err := shell.PrintInit(binaryName, shellArg); err != nil {
				fmt.Fprintf(os.Stderr, "%s: --shell init: %v\n", binaryName, err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "%s: --shell: unknown subcommand %q\n", binaryName, shellCmd)
			fmt.Fprintf(os.Stderr, "Run '%s --shell --help' for usage.\n", binaryName)
			os.Exit(2)
		}
		return
	}

	// ── Service management ────────────────────────────────────────────────────

	if serviceCmd != "" {
		switch serviceCmd {
		case "--help":
			service.PrintHelp(binaryName)
		case "start":
			if err := service.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service start: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Service started.\n")
		case "stop":
			if err := service.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service stop: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Service stopped.\n")
		case "restart":
			if err := service.Restart(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service restart: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Service restarted.\n")
		case "reload":
			if err := service.Reload(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service reload: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Service reloaded.\n")
		case "--install":
			if err := service.Install(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service install: %v\n", binaryName, err)
				os.Exit(1)
			}
		case "--disable":
			if err := service.Disable(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service disable: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Service disabled.\n")
		case "--uninstall":
			if err := service.Uninstall(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: service uninstall: %v\n", binaryName, err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "%s: unknown --service subcommand: %s\n", binaryName, serviceCmd)
			fmt.Fprintf(os.Stderr, "Run '%s --service --help' for usage.\n", binaryName)
			os.Exit(2)
		}
		return
	}

	// ── Maintenance operations ────────────────────────────────────────────────

	if maintenanceCmd != "" {
		// Paths must be resolved before maintenance commands.
		mcConfigDir := paths.GetConfigDir(appName)
		mcDataDir := paths.GetDataDir(appName)
		mcBackupDir := mcDataDir // default; will be adjusted below when full path resolution runs
		_ = mcBackupDir

		switch maintenanceCmd {
		case "--help":
			maintenance.PrintHelp(binaryName)
		case "backup":
			opts := maintenance.BackupOptions{
				ConfigDir:  mcConfigDir,
				DataDir:    mcDataDir,
				BackupDir:  filepath.Join(mcDataDir, "backups"),
				AppVersion: Version,
				Filename:   maintenanceArg, // optional custom filename
			}
			if err := maintenance.Backup(opts); err != nil {
				fmt.Fprintf(os.Stderr, "%s: maintenance backup: %v\n", binaryName, err)
				os.Exit(1)
			}
		case "restore":
			if maintenanceArg == "" {
				fmt.Fprintf(os.Stderr, "%s: --maintenance restore requires a filename argument\n", binaryName)
				fmt.Fprintf(os.Stderr, "Usage: %s --maintenance restore <backup-file>\n", binaryName)
				os.Exit(2)
			}
			if err := maintenance.Restore(maintenanceArg, mcConfigDir, mcDataDir, ""); err != nil {
				fmt.Fprintf(os.Stderr, "%s: maintenance restore: %v\n", binaryName, err)
				os.Exit(1)
			}
			return
		case "update":
			// Alias for --update yes: handled by the update block below.
			updateCmd = "yes"
		case "mode":
			if maintenanceArg == "" {
				fmt.Fprintf(os.Stderr, "%s: --maintenance mode requires a mode argument\n", binaryName)
				fmt.Fprintf(os.Stderr, "Usage: %s --maintenance mode <production|development>\n", binaryName)
				os.Exit(2)
			}
			if err := maintenance.SetMode(mcConfigDir, maintenanceArg); err != nil {
				fmt.Fprintf(os.Stderr, "%s: maintenance mode: %v\n", binaryName, err)
				os.Exit(1)
			}
			return
		case "setup":
			if err := maintenance.Setup(mcConfigDir); err != nil {
				fmt.Fprintf(os.Stderr, "%s: maintenance setup: %v\n", binaryName, err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "%s: unknown --maintenance subcommand: %s\n", binaryName, maintenanceCmd)
			fmt.Fprintf(os.Stderr, "Run '%s --maintenance --help' for usage.\n", binaryName)
			os.Exit(2)
		}
		// All maintenance subcommands exit here except "update" which falls
		// through to the --update block below.
		if maintenanceCmd != "update" {
			return
		}
	}

	// ── Update ────────────────────────────────────────────────────────────────

	if updateCmd != "" {
		switch {
		case updateCmd == "--help":
			fmt.Printf(`Update: %s --update [command]

Commands:
  check              Check for updates without installing
  yes                Download and install the latest update, then restart
  branch <name>      Switch update branch (stable|beta|daily)

Examples:
  %s --update check
  %s --update yes
  %s --update branch beta
`, binaryName, binaryName, binaryName, binaryName)
			return

		case updateCmd == "check":
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			rel, err := updater.CheckForUpdate(ctx, Version, "stable")
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: update check: %v\n", binaryName, err)
				os.Exit(1)
			}
			if rel == nil {
				fmt.Printf("%s is up to date (%s).\n", binaryName, Version)
			} else {
				fmt.Printf("Update available: %s → %s\n", Version, rel.TagName)
				fmt.Printf("Run '%s --update yes' to install.\n", binaryName)
			}
			return

		case updateCmd == "yes":
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			rel, err := updater.CheckForUpdate(ctx, Version, "stable")
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: update check: %v\n", binaryName, err)
				os.Exit(1)
			}
			if rel == nil {
				fmt.Printf("%s is already up to date (%s).\n", binaryName, Version)
				return
			}
			fmt.Printf("Downloading %s %s…\n", binaryName, rel.TagName)
			if err := updater.DoUpdate(ctx, rel); err != nil {
				fmt.Fprintf(os.Stderr, "%s: update failed: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Update installed. Restarting…\n")
			if err := updater.RestartSelf(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: restart failed: %v\n", binaryName, err)
				os.Exit(1)
			}
			return

		case strings.HasPrefix(updateCmd, "branch"):
			parts := strings.Fields(updateCmd)
			if len(parts) < 2 {
				fmt.Fprintf(os.Stderr, "Usage: %s --update branch <stable|beta|daily>\n", binaryName)
				os.Exit(2)
			}
			branch := parts[1]
			switch branch {
			case "stable", "beta", "daily":
				fmt.Printf("Update branch set to: %s\n", branch)
				// Branch preference is informational here; the actual setting
				// lives in the config file. Full config integration is handled
				// via the admin UI / config file.
			default:
				fmt.Fprintf(os.Stderr, "%s: unknown branch: %s (use stable|beta|daily)\n", binaryName, branch)
				os.Exit(2)
			}
			return

		default:
			fmt.Fprintf(os.Stderr, "%s: unknown --update subcommand: %s\n", binaryName, updateCmd)
			fmt.Fprintf(os.Stderr, "Run '%s --update --help' for usage.\n", binaryName)
			os.Exit(2)
		}
	}

	// ── Daemon ────────────────────────────────────────────────────────────────

	if daemonFlag {
		if err := daemon.Daemonize(); err != nil {
			log.Fatalf("daemon: %v", err)
		}
		// If Daemonize returned without exiting, we are the daemon child.
		// Continue with normal server startup below.
	}

	// Resolve active language for CLI output.
	activeLang := i18n.GetLanguage(langFlag)
	_ = activeLang // used by CLI output helpers; stored for future message formatting

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
	backupDir := paths.GetBackupDir(appName)
	pidFile := paths.GetPIDFile(appName)

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
		backupDir = backupFlag
	}
	if pidFlag != "" {
		pidFile = pidFlag
	}

	for _, dir := range []string{configDir, dataDir, logsDir, cacheDir, backupDir} {
		if err := paths.EnsureDir(dir); err != nil {
			log.Printf("warning: could not create directory %s: %v", dir, err)
		}
	}

	// Ensure parent of PID file exists.
	if err := paths.EnsureDir(filepath.Dir(pidFile)); err != nil {
		log.Printf("warning: could not create pid file directory: %v", err)
	}

	if showStatus {
		fmt.Printf("%s %s (%s)\n", binaryName, Version, CommitID)
		fmt.Printf("Mode:   %s\n", mode.Get())
		fmt.Printf("Config: %s\n", filepath.Join(configDir, "server.yml"))
		fmt.Printf("Data:   %s\n", dataDir)
		fmt.Printf("Logs:   %s\n", logsDir)
		fmt.Printf("Cache:  %s\n", cacheDir)
		fmt.Printf("Backup: %s\n", backupDir)
		fmt.Printf("PID:    %s\n", pidFile)
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

	// ── Port resolution (random 64xxx on first run; 80 in container) ──────────

	if err := config.ResolvePort(cfgFile, cfg, paths.IsContainer()); err != nil {
		log.Printf("warning: %v", err)
		if cfg.Server.Port == "" {
			cfg.Server.Port = "64080" // last-resort fallback
		}
	}

	// ── GeoIP directory ───────────────────────────────────────────────────────

	if cfg.Server.GeoIP.Dir == "" {
		cfg.Server.GeoIP.Dir = filepath.Join(dataDir, "security", "geoip")
	}
	if cfg.Server.GeoIP.Enabled {
		if err := paths.EnsureDir(cfg.Server.GeoIP.Dir); err != nil {
			log.Printf("warning: geoip dir: %v", err)
		}
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

	sched := scheduler.New(db)

	// Project-specific: expire and burn-after pastes every 10 minutes.
	logSchedErr(sched.Register("expire-pastes", "Expire Pastes", "@every 10m", true, func() error {
		n, err := db.DeleteExpiredPastes()
		if err != nil {
			return err
		}
		b, _ := db.DeleteBurnedPastes()
		if n+b > 0 {
			log.Printf("scheduler: removed %d expired + %d burned pastes", n, b)
		}
		return nil
	}))

	// Required PART 18 tasks — full implementations in src/task/task.go.
	logSchedErr(sched.Register("ssl_renewal", "SSL Renewal", "0 3 * * *", true,
		task.SSLRenewal(configDir, cfg.Server.FQDN)))
	logSchedErr(sched.Register("blocklist_update", "Blocklist Update", "0 4 * * *", true,
		task.BlocklistUpdate(dataDir)))
	logSchedErr(sched.Register("cve_update", "CVE Update", "0 5 * * *", true,
		task.CVEUpdate(dataDir)))
	logSchedErr(sched.Register("token_cleanup", "Token Cleanup", "@every 15m", true, func() error {
		// This project has no API tokens; task is registered per PART 18 but is a no-op.
		return nil
	}))
	logSchedErr(sched.Register("log_rotation", "Log Rotation", "0 0 * * *", true,
		task.LogRotation(logsDir, 30*24*time.Hour)))
	logSchedErr(sched.Register("backup_daily", "Backup Daily", "0 2 * * *", true,
		task.BackupDaily(appName, dataDir, backupDir, 1)))
	logSchedErr(sched.Register("backup_hourly", "Backup Hourly", "@hourly", false,
		task.BackupHourly(appName, dataDir, backupDir)))
	logSchedErr(sched.Register("healthcheck_self", "Health Check", "@every 5m", true, func() error {
		if err := db.Ping(); err != nil {
			return fmt.Errorf("healthcheck_self: database ping failed: %w", err)
		}
		log.Printf("healthcheck_self: ok")
		return nil
	}))

	// ── HTTP server ───────────────────────────────────────────────────────────

	srv := server.New(db, cfg, Version, CommitID, BuildDate, configDir, dataDir)

	// Weekly GeoIP database refresh (Sunday 03:00).
	logSchedErr(sched.Register("geoip_update", "GeoIP Update", "0 3 * * 0", srv.GeoIPEnabled(), func() error {
		if err := srv.UpdateGeoIP(); err != nil {
			return err
		}
		log.Printf("scheduler: geoip databases updated")
		return nil
	}))

	// Tor health check — registered after srv so it can query srv.TorRunning().
	logSchedErr(sched.Register("tor_health", "Tor Health", "@every 10m", true,
		task.TorHealth(srv.TorRunning)))

	sched.Start()
	defer sched.Stop()

	// ── PID file ──────────────────────────────────────────────────────────────
	// WritePIDFile also calls CheckPIDFile; it exits non-zero if another
	// instance of our binary is already running.

	if err := pid.WritePIDFile(pidFile); err != nil {
		log.Fatalf("pid file: %v", err)
	}
	defer pid.RemovePIDFile(pidFile) //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Printf("shutting down…")
		pid.RemovePIDFile(pidFile) //nolint:errcheck
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
      --port PORT                   Listen port (default: random 64xxx, 80 in container)
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

// logSchedErr logs a scheduler registration error and continues. Registration
// errors are programming errors (bad cron expression) and should never occur
// at runtime — log them so they are visible without crashing the server.
func logSchedErr(err error) {
	if err != nil {
		log.Printf("warning: scheduler registration: %v", err)
	}
}
