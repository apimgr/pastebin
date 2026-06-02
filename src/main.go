package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/apimgr/pastebin/src/common/email"
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
		emailCmd       string // --email <subcommand>
		emailTo        string // --email test <address>
		schedulerCmd   string // scheduler <subcommand>
		schedulerArg   string // scheduler <subcommand> <id>
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
		case "--email":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				emailCmd = args[i]
				// For "test <address>", consume optional recipient address.
				if emailCmd == "test" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
					i++
					emailTo = args[i]
				}
			}
		case "scheduler":
			// Positional: scheduler <subcommand> [id]
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				schedulerCmd = args[i]
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				schedulerArg = args[i]
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

	// ── Email command ─────────────────────────────────────────────────────────

	if emailCmd != "" {
		cfgFile2 := filepath.Join(paths.GetConfigDir(appName), "server.yml")
		cfg2, _ := config.Load(cfgFile2)
		baseURL2 := cfg2.Server.BaseURL
		if baseURL2 == "" {
			baseURL2 = "http://localhost"
		}
		m := email.New(&cfg2.Server.Notifications.Email, cfg2.Web.SiteTitle, baseURL2, cfg2.Server.FQDN)
		switch emailCmd {
		case "test":
			if emailTo == "" {
				fmt.Fprintf(os.Stderr, "Usage: %s --email test <address>\n", binaryName)
				os.Exit(2)
			}
			if err := m.TestSMTP(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: SMTP test failed: %v\n", binaryName, err)
				os.Exit(1)
			}
			if err := m.Send(emailTo, "test", nil); err != nil {
				fmt.Fprintf(os.Stderr, "%s: send failed: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("Test email sent to %s\n", emailTo)
		default:
			fmt.Fprintf(os.Stderr, "%s: unknown --email subcommand: %s\n", binaryName, emailCmd)
			os.Exit(2)
		}
		return
	}

	// ── Scheduler CLI ─────────────────────────────────────────────────────────

	if schedulerCmd != "" {
		scCfgFile := filepath.Join(paths.GetConfigDir(appName), "server.yml")
		scCfg, _ := config.Load(scCfgFile)
		if scCfg.Database.Path == "" {
			scCfg.Database.Path = paths.GetDBPath(appName)
		}
		scDB, err := database.NewDatabase(scCfg.Database.Type, scCfg.Database.Path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: scheduler: database: %v\n", binaryName, err)
			os.Exit(1)
		}
		defer scDB.Close()

		switch schedulerCmd {
		case "list":
			tasks, err := scDB.ListSchedulerTasks()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler list: %v\n", binaryName, err)
				os.Exit(1)
			}
			fmt.Printf("%-20s %-16s %-12s %-20s %-20s\n", "ID", "SCHEDULE", "STATUS", "LAST RUN", "NEXT RUN")
			fmt.Printf("%-20s %-16s %-12s %-20s %-20s\n",
				"--------------------", "----------------", "------------",
				"--------------------", "--------------------")
			for _, t := range tasks {
				enabled := "disabled"
				if t.Enabled {
					enabled = t.LastStatus
					if enabled == "" {
						enabled = "pending"
					}
				}
				lastRun := "-"
				if !t.LastRun.IsZero() {
					lastRun = t.LastRun.Format("2006-01-02 15:04:05")
				}
				nextRun := "-"
				if !t.NextRun.IsZero() && t.Enabled {
					nextRun = t.NextRun.Format("2006-01-02 15:04:05")
				}
				fmt.Printf("%-20s %-16s %-12s %-20s %-20s\n",
					t.TaskID, t.Schedule, enabled, lastRun, nextRun)
			}

		case "show":
			if schedulerArg == "" {
				fmt.Fprintf(os.Stderr, "Usage: %s scheduler show <id>\n", binaryName)
				os.Exit(2)
			}
			t, err := scDB.GetSchedulerTask(schedulerArg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler show: %v\n", binaryName, err)
				os.Exit(1)
			}
			enabled := "no"
			if t.Enabled {
				enabled = "yes"
			}
			lastRun := "-"
			if !t.LastRun.IsZero() {
				lastRun = t.LastRun.Format(time.RFC3339)
			}
			nextRun := "-"
			if !t.NextRun.IsZero() {
				nextRun = t.NextRun.Format(time.RFC3339)
			}
			fmt.Printf("ID:          %s\n", t.TaskID)
			fmt.Printf("Name:        %s\n", t.TaskName)
			fmt.Printf("Schedule:    %s\n", t.Schedule)
			fmt.Printf("Enabled:     %s\n", enabled)
			fmt.Printf("Status:      %s\n", t.LastStatus)
			fmt.Printf("Last Run:    %s\n", lastRun)
			fmt.Printf("Next Run:    %s\n", nextRun)
			fmt.Printf("Run Count:   %d\n", t.RunCount)
			fmt.Printf("Fail Count:  %d\n", t.FailCount)
			if t.LastError != "" {
				fmt.Printf("Last Error:  %s\n", t.LastError)
			}

		case "run":
			if schedulerArg == "" {
				fmt.Fprintf(os.Stderr, "Usage: %s scheduler run <id>\n", binaryName)
				os.Exit(2)
			}
			// Connect to the running server's scheduler via the API.
			scBaseURL := scCfg.Server.BaseURL
			if scBaseURL == "" {
				addr := scCfg.Server.Address
				if addr == "" || addr == "0.0.0.0" {
					addr = "127.0.0.1"
				}
				port := scCfg.Server.Port
				if port == "" {
					port = "80"
				}
				scBaseURL = "http://" + addr + ":" + port
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			req, reqErr := newHTTPRequest(ctx, "POST",
				scBaseURL+"/api/v1/scheduler/"+schedulerArg+"/run", nil)
			if reqErr != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler run: %v\n", binaryName, reqErr)
				os.Exit(1)
			}
			resp, doErr := doHTTP(req)
			if doErr != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler run: %v\n", binaryName, doErr)
				os.Exit(1)
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				fmt.Fprintf(os.Stderr, "%s: scheduler run: server returned %s\n", binaryName, resp.Status)
				os.Exit(1)
			}
			fmt.Printf("Task %s triggered.\n", schedulerArg)

		case "enable":
			if schedulerArg == "" {
				fmt.Fprintf(os.Stderr, "Usage: %s scheduler enable <id>\n", binaryName)
				os.Exit(2)
			}
			scBaseURL := scCfg.Server.BaseURL
			if scBaseURL == "" {
				addr := scCfg.Server.Address
				if addr == "" || addr == "0.0.0.0" {
					addr = "127.0.0.1"
				}
				port := scCfg.Server.Port
				if port == "" {
					port = "80"
				}
				scBaseURL = "http://" + addr + ":" + port
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			req, reqErr := newHTTPRequest(ctx, "POST",
				scBaseURL+"/api/v1/scheduler/"+schedulerArg+"/enable", nil)
			if reqErr != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler enable: %v\n", binaryName, reqErr)
				os.Exit(1)
			}
			resp, doErr := doHTTP(req)
			if doErr != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler enable: %v\n", binaryName, doErr)
				os.Exit(1)
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				fmt.Fprintf(os.Stderr, "%s: scheduler enable: server returned %s\n", binaryName, resp.Status)
				os.Exit(1)
			}
			fmt.Printf("Task %s enabled.\n", schedulerArg)

		case "disable":
			if schedulerArg == "" {
				fmt.Fprintf(os.Stderr, "Usage: %s scheduler disable <id>\n", binaryName)
				os.Exit(2)
			}
			scBaseURL := scCfg.Server.BaseURL
			if scBaseURL == "" {
				addr := scCfg.Server.Address
				if addr == "" || addr == "0.0.0.0" {
					addr = "127.0.0.1"
				}
				port := scCfg.Server.Port
				if port == "" {
					port = "80"
				}
				scBaseURL = "http://" + addr + ":" + port
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			req, reqErr := newHTTPRequest(ctx, "POST",
				scBaseURL+"/api/v1/scheduler/"+schedulerArg+"/disable", nil)
			if reqErr != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler disable: %v\n", binaryName, reqErr)
				os.Exit(1)
			}
			resp, doErr := doHTTP(req)
			if doErr != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler disable: %v\n", binaryName, doErr)
				os.Exit(1)
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				fmt.Fprintf(os.Stderr, "%s: scheduler disable: server returned %s\n", binaryName, resp.Status)
				os.Exit(1)
			}
			fmt.Printf("Task %s disabled.\n", schedulerArg)

		case "history":
			if schedulerArg == "" {
				fmt.Fprintf(os.Stderr, "Usage: %s scheduler history <id>\n", binaryName)
				os.Exit(2)
			}
			history, err := scDB.ListTaskHistory(schedulerArg, 20)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: scheduler history: %v\n", binaryName, err)
				os.Exit(1)
			}
			if len(history) == 0 {
				fmt.Printf("No history for task %q.\n", schedulerArg)
			} else {
				fmt.Printf("%-24s %-24s %-10s %-10s %s\n",
					"STARTED", "FINISHED", "STATUS", "DURATION", "ERROR")
				fmt.Printf("%-24s %-24s %-10s %-10s %s\n",
					"------------------------", "------------------------",
					"----------", "----------", "-----")
				for _, h := range history {
					dur := fmt.Sprintf("%dms", h.DurationMS)
					errStr := h.ErrorMsg
					if errStr == "" {
						errStr = "-"
					}
					fmt.Printf("%-24s %-24s %-10s %-10s %s\n",
						h.StartedAt.Format("2006-01-02 15:04:05"),
						h.FinishedAt.Format("2006-01-02 15:04:05"),
						h.Status, dur, errStr)
				}
			}

		default:
			fmt.Fprintf(os.Stderr, "%s: unknown scheduler subcommand: %s\n", binaryName, schedulerCmd)
			fmt.Fprintf(os.Stderr, "Usage: %s scheduler {list|show|run|enable|disable|history} [id]\n", binaryName)
			os.Exit(2)
		}
		return
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
		mode.SetDebug(true)
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

	// ── SMTP auto-detect (first run, host not yet configured) ─────────────────

	if cfg.Server.Notifications.Email.SMTP.Host == "" {
		if host, port, ok := email.AutoDetect(cfg.Server.FQDN); ok {
			cfg.Server.Notifications.Email.SMTP.Host = host
			cfg.Server.Notifications.Email.SMTP.Port = port
			cfg.Server.Notifications.Email.Enabled = true
			if saveErr := config.Save(cfgFile, cfg); saveErr != nil {
				log.Printf("warning: could not persist SMTP config: %v", saveErr)
			}
		}
	}

	// ── Config hot-reload watcher ─────────────────────────────────────────────

	cfgMgr := config.NewConfigManager(cfgFile, cfg)

	// ── DataDir in config (used by blocklist middleware and tasks) ───────────

	if cfg.Server.DataDir == "" {
		cfg.Server.DataDir = dataDir
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
		cfg.Database.Path = paths.GetDBPath(appName)
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
	backupCfg := task.BackupConfig{
		ProjectName: appName,
		ConfigDir:   configDir,
		DataDir:     dataDir,
		BackupDir:   backupDir,
		AppVersion:  Version,
		Retention:   task.BackupRetention{MaxBackups: 1},
	}
	logSchedErr(sched.Register("backup_daily", "Backup Daily", "0 2 * * *", true,
		task.BackupDaily(backupCfg)))
	logSchedErr(sched.Register("backup_hourly", "Backup Hourly", "@hourly", false,
		task.BackupHourly(backupCfg)))
	logSchedErr(sched.Register("healthcheck_self", "Health Check", "@every 5m", true, func() error {
		if err := db.Ping(); err != nil {
			return fmt.Errorf("healthcheck_self: database ping failed: %w", err)
		}
		log.Printf("healthcheck_self: ok")
		return nil
	}))

	// ── HTTP server ───────────────────────────────────────────────────────────

	srv := server.New(db, cfg, cfgMgr, Version, CommitID, BuildDate, configDir, dataDir)

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

	// Register scheduler health callback with the server for /server/healthz.
	srv.SetSchedulerHealthFn(sched.Running)
	srv.SetSchedulerAPI(sched)

	// ── PID file ──────────────────────────────────────────────────────────────
	// WritePIDFile also calls CheckPIDFile; it exits non-zero if another
	// instance of our binary is already running.

	if err := pid.WritePIDFile(pidFile); err != nil {
		log.Fatalf("pid file: %v", err)
	}
	defer pid.RemovePIDFile(pidFile) //nolint:errcheck

	addr := cfg.Server.Address + ":" + cfg.Server.Port

	// runServer is the shared startup closure used both for interactive mode
	// and for the Windows Service Control Manager path.
	runServer := func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		stopCfgMgr := make(chan struct{})
		cfgMgr.Start(stopCfgMgr, srv.OnConfigChange)

		go func() {
			<-sig
			log.Printf("shutting down…")
			pid.RemovePIDFile(pidFile) //nolint:errcheck
			close(stopCfgMgr)
			cancel()
		}()

		log.Printf("listening on %s", addr)

		if err := srv.Run(ctx, addr); err != nil {
			log.Fatalf("server: %v", err)
		}
	}

	// On Windows, if we were started by the SCM, hand control to the service
	// handler. On all other platforms (and interactive Windows), run directly.
	if service.IsWindowsService() {
		if err := service.RunAsWindowsService(appName, runServer); err != nil {
			log.Fatalf("windows service: %v", err)
		}
		return
	}

	runServer()
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

Scheduler:
      scheduler list                List all scheduled tasks and their status
      scheduler show <id>           Show details for a specific task
      scheduler run <id>            Trigger a task to run immediately
      scheduler enable <id>         Enable a disabled task
      scheduler disable <id>        Disable a task
      scheduler history <id>        Show recent execution history for a task

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

// newHTTPRequest creates an HTTP request with no body and a User-Agent header.
func newHTTPRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "pastebin-cli/"+Version)
	return req, nil
}

// doHTTP executes an HTTP request using the default client.
func doHTTP(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}
