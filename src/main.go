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
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

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
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the testable entry point. rawArgs is os.Args[1:]; stdout and stderr are
// the output writers (os.Stdout / os.Stderr in production, captured buffers in
// tests). It returns a POSIX exit code: 0 = success, 1 = runtime error, 2 = usage error.
func run(rawArgs []string, stdout, stderr io.Writer) int {
	binaryName := filepath.Base(os.Args[0])

	// Pre-process args: normalise -flag to --flag and expand -h/-v aliases.
	args := normalizeArgs(rawArgs)

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
		shellCmd        string
		shellArg        string
		serviceCmd      string
		maintenanceCmd  string
		maintenanceArg  string // second positional arg after --maintenance subcommand
		maintenancePass string // --password flag for --maintenance backup/restore
		updateCmd       string
		emailCmd        string // --email <subcommand>
		emailTo         string // --email test <address>
		schedulerCmd    string // scheduler <subcommand>
		schedulerArg    string // scheduler <subcommand> <id>
		tokenCmd        string // token <subcommand>
		tokenArg        string // token <subcommand> <prefix>
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
		case "--help", "-h", "-help":
			showHelp = true
		case "--version", "-v", "-version":
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
		case "--password":
			maintenancePass = val()
		case "--maintenance":
			maintenanceCmd = val()
			// Capture an optional second positional argument (e.g. filename for
			// "restore" or mode name for "mode").
			maintenanceArg = val()
		case "--update":
			// Consume the next arg unconditionally as the update subcommand so
			// that "--update --help" routes to update-specific help instead of
			// triggering the global --help handler.
			if i+1 < len(args) {
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
		case "token":
			// Positional: token <subcommand> [prefix]
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				tokenCmd = args[i]
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				i++
				tokenArg = args[i]
			}
		default:
			if strings.HasPrefix(arg, "--") || (strings.HasPrefix(arg, "-") && len(arg) > 2) {
				fmt.Fprintf(stderr, "%s: unknown flag: %s\n", binaryName, arg)
				fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", binaryName)
				return 2
			}
		}
	}

	// Apply --color flag before any output.
	applyColor(colorFlag)

	if showHelp {
		printHelp(stdout, binaryName)
		return 0
	}

	if showVersion {
		fmt.Fprintf(stdout, "%s %s (%s)\n", binaryName, Version, CommitID)
		fmt.Fprintf(stdout, "Built: %s\n", BuildDate)
		fmt.Fprintf(stdout, "Go: %s\n", runtime.Version())
		fmt.Fprintf(stdout, "OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return 0
	}

	// ── Shell integration ─────────────────────────────────────────────────────

	if shellCmd != "" {
		switch shellCmd {
		case "--help":
			shell.PrintHelp(binaryName)
		case "completions":
			if err := shell.PrintCompletions(binaryName, shellArg); err != nil {
				fmt.Fprintf(stderr, "%s: --shell completions: %v\n", binaryName, err)
				return 1
			}
		case "init":
			if err := shell.PrintInit(binaryName, shellArg); err != nil {
				fmt.Fprintf(stderr, "%s: --shell init: %v\n", binaryName, err)
				return 1
			}
		default:
			fmt.Fprintf(stderr, "%s: --shell: unknown subcommand %q\n", binaryName, shellCmd)
			fmt.Fprintf(stderr, "Run '%s --shell --help' for usage.\n", binaryName)
			return 2
		}
		return 0
	}

	// ── Service management ────────────────────────────────────────────────────

	if serviceCmd != "" {
		switch serviceCmd {
		case "--help":
			service.PrintHelp(binaryName)
		case "start":
			if err := service.Start(); err != nil {
				fmt.Fprintf(stderr, "%s: service start: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Service started.\n")
		case "stop":
			if err := service.Stop(); err != nil {
				fmt.Fprintf(stderr, "%s: service stop: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Service stopped.\n")
		case "restart":
			if err := service.Restart(); err != nil {
				fmt.Fprintf(stderr, "%s: service restart: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Service restarted.\n")
		case "reload":
			if err := service.Reload(); err != nil {
				fmt.Fprintf(stderr, "%s: service reload: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Service reloaded.\n")
		case "--install":
			if err := service.Install(); err != nil {
				fmt.Fprintf(stderr, "%s: service install: %v\n", binaryName, err)
				return 1
			}
		case "--disable":
			if err := service.Disable(); err != nil {
				fmt.Fprintf(stderr, "%s: service disable: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Service disabled.\n")
		case "--uninstall":
			if err := service.Uninstall(); err != nil {
				fmt.Fprintf(stderr, "%s: service uninstall: %v\n", binaryName, err)
				return 1
			}
		default:
			fmt.Fprintf(stderr, "%s: unknown --service subcommand: %s\n", binaryName, serviceCmd)
			fmt.Fprintf(stderr, "Run '%s --service --help' for usage.\n", binaryName)
			return 2
		}
		return 0
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
				Password:   maintenancePass,
				Filename:   maintenanceArg, // optional custom filename
			}
			if err := maintenance.Backup(opts); err != nil {
				fmt.Fprintf(stderr, "%s: maintenance backup: %v\n", binaryName, err)
				return 1
			}
		case "restore":
			if maintenanceArg == "" {
				fmt.Fprintf(stderr, "%s: --maintenance restore requires a filename argument\n", binaryName)
				fmt.Fprintf(stderr, "Usage: %s --maintenance restore <backup-file>\n", binaryName)
				return 2
			}
			restorePass := maintenancePass
			// Prompt for password when restoring an encrypted backup and none was provided.
			if strings.HasSuffix(maintenanceArg, ".enc") && restorePass == "" {
				fmt.Fprint(stderr, "Backup password: ")
				pw, pwErr := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(stderr)
				if pwErr != nil {
					fmt.Fprintf(stderr, "%s: reading password: %v\n", binaryName, pwErr)
					return 1
				}
				restorePass = string(pw)
			}
			if err := maintenance.Restore(maintenanceArg, mcConfigDir, mcDataDir, restorePass); err != nil {
				fmt.Fprintf(stderr, "%s: maintenance restore: %v\n", binaryName, err)
				return 1
			}
			return 0
		case "update":
			// Alias for --update yes: handled by the update block below.
			updateCmd = "yes"
		case "mode":
			if maintenanceArg == "" {
				fmt.Fprintf(stderr, "%s: --maintenance mode requires a mode argument\n", binaryName)
				fmt.Fprintf(stderr, "Usage: %s --maintenance mode <production|development>\n", binaryName)
				return 2
			}
			if err := maintenance.SetMode(mcConfigDir, maintenanceArg); err != nil {
				fmt.Fprintf(stderr, "%s: maintenance mode: %v\n", binaryName, err)
				return 1
			}
			return 0
		case "setup":
			if err := maintenance.Setup(mcConfigDir); err != nil {
				fmt.Fprintf(stderr, "%s: maintenance setup: %v\n", binaryName, err)
				return 1
			}
			return 0
		default:
			fmt.Fprintf(stderr, "%s: unknown --maintenance subcommand: %s\n", binaryName, maintenanceCmd)
			fmt.Fprintf(stderr, "Run '%s --maintenance --help' for usage.\n", binaryName)
			return 2
		}
		// All maintenance subcommands return here except "update" which falls
		// through to the --update block below.
		if maintenanceCmd != "update" {
			return 0
		}
	}

	// ── Update ────────────────────────────────────────────────────────────────

	if updateCmd != "" {
		// Load config to resolve configured update branch.
		updateCfgFile := filepath.Join(paths.GetConfigDir(appName), "server.yml")
		updateCfg, _ := config.Load(updateCfgFile)
		configuredBranch := updateCfg.Server.UpdateBranch
		if configuredBranch == "" {
			configuredBranch = "stable"
		}

		switch {
		case updateCmd == "--help":
			fmt.Fprintf(stdout, `Update: %s --update [command]

Commands:
  check              Check for updates without installing
  yes                Download and install the latest update, then restart
  branch <name>      Switch update branch (stable|beta|daily)

Examples:
  %s --update check
  %s --update yes
  %s --update branch beta
`, binaryName, binaryName, binaryName, binaryName)
			return 0

		case updateCmd == "check":
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			rel, err := updater.CheckForUpdate(ctx, Version, configuredBranch)
			if err != nil {
				fmt.Fprintf(stderr, "%s: update check: %v\n", binaryName, err)
				return 1
			}
			if rel == nil {
				fmt.Fprintf(stdout, "%s is up to date (%s) on branch %s.\n", binaryName, Version, configuredBranch)
			} else {
				fmt.Fprintf(stdout, "Update available: %s → %s\n", Version, rel.TagName)
				fmt.Fprintf(stdout, "Run '%s --update yes' to install.\n", binaryName)
			}
			return 0

		case updateCmd == "yes":
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			rel, err := updater.CheckForUpdate(ctx, Version, configuredBranch)
			if err != nil {
				fmt.Fprintf(stderr, "%s: update check: %v\n", binaryName, err)
				return 1
			}
			if rel == nil {
				fmt.Fprintf(stdout, "%s is already up to date (%s) on branch %s.\n", binaryName, Version, configuredBranch)
				return 0
			}
			fmt.Fprintf(stdout, "Downloading %s %s…\n", binaryName, rel.TagName)
			if err := updater.DoUpdate(ctx, rel); err != nil {
				fmt.Fprintf(stderr, "%s: update failed: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Update installed. Restarting…\n")
			if err := updater.RestartSelf(); err != nil {
				fmt.Fprintf(stderr, "%s: restart failed: %v\n", binaryName, err)
				return 1
			}
			return 0

		case strings.HasPrefix(updateCmd, "branch"):
			parts := strings.Fields(updateCmd)
			if len(parts) < 2 {
				fmt.Fprintf(stderr, "Usage: %s --update branch <stable|beta|daily>\n", binaryName)
				return 2
			}
			branch := parts[1]
			switch branch {
			case "stable", "beta", "daily":
				// Persist the branch choice to the config file.
				if writeErr := maintenance.SetYAMLField(updateCfgFile, "update_branch", branch); writeErr != nil {
					fmt.Fprintf(stderr, "%s: could not persist update branch: %v\n", binaryName, writeErr)
					return 1
				}
				fmt.Fprintf(stdout, "Update branch set to: %s\n", branch)
			default:
				fmt.Fprintf(stderr, "%s: unknown branch: %s (use stable|beta|daily)\n", binaryName, branch)
				return 2
			}
			return 0

		default:
			fmt.Fprintf(stderr, "%s: unknown --update subcommand: %s\n", binaryName, updateCmd)
			fmt.Fprintf(stderr, "Run '%s --update --help' for usage.\n", binaryName)
			return 2
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
				fmt.Fprintf(stderr, "Usage: %s --email test <address>\n", binaryName)
				return 2
			}
			if err := m.TestSMTP(); err != nil {
				fmt.Fprintf(stderr, "%s: SMTP test failed: %v\n", binaryName, err)
				return 1
			}
			if err := m.Send(emailTo, "test", nil); err != nil {
				fmt.Fprintf(stderr, "%s: send failed: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Test email sent to %s\n", emailTo)
		default:
			fmt.Fprintf(stderr, "%s: unknown --email subcommand: %s\n", binaryName, emailCmd)
			return 2
		}
		return 0
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
			fmt.Fprintf(stderr, "%s: scheduler: database: %v\n", binaryName, err)
			return 1
		}
		defer scDB.Close()

		switch schedulerCmd {
		case "list":
			tasks, err := scDB.ListSchedulerTasks()
			if err != nil {
				fmt.Fprintf(stderr, "%s: scheduler list: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "%-20s %-16s %-12s %-20s %-20s\n", "ID", "SCHEDULE", "STATUS", "LAST RUN", "NEXT RUN")
			fmt.Fprintf(stdout, "%-20s %-16s %-12s %-20s %-20s\n",
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
				fmt.Fprintf(stdout, "%-20s %-16s %-12s %-20s %-20s\n",
					t.TaskID, t.Schedule, enabled, lastRun, nextRun)
			}

		case "show":
			if schedulerArg == "" {
				fmt.Fprintf(stderr, "Usage: %s scheduler show <id>\n", binaryName)
				return 2
			}
			t, err := scDB.GetSchedulerTask(schedulerArg)
			if err != nil {
				fmt.Fprintf(stderr, "%s: scheduler show: %v\n", binaryName, err)
				return 1
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
			fmt.Fprintf(stdout, "ID:          %s\n", t.TaskID)
			fmt.Fprintf(stdout, "Name:        %s\n", t.TaskName)
			fmt.Fprintf(stdout, "Schedule:    %s\n", t.Schedule)
			fmt.Fprintf(stdout, "Enabled:     %s\n", enabled)
			fmt.Fprintf(stdout, "Status:      %s\n", t.LastStatus)
			fmt.Fprintf(stdout, "Last Run:    %s\n", lastRun)
			fmt.Fprintf(stdout, "Next Run:    %s\n", nextRun)
			fmt.Fprintf(stdout, "Run Count:   %d\n", t.RunCount)
			fmt.Fprintf(stdout, "Fail Count:  %d\n", t.FailCount)
			if t.LastError != "" {
				fmt.Fprintf(stdout, "Last Error:  %s\n", t.LastError)
			}

		case "run":
			if schedulerArg == "" {
				fmt.Fprintf(stderr, "Usage: %s scheduler run <id>\n", binaryName)
				return 2
			}
			if scCfg.Server.Token == "" {
				fmt.Fprintf(stderr, "%s: scheduler run: server.token not set in config; cannot authenticate\n", binaryName)
				return 1
			}
			// Triggering a task requires the server to be running — send an
			// authenticated POST to the scheduler API (PART 18).
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
				fmt.Fprintf(stderr, "%s: scheduler run: %v\n", binaryName, reqErr)
				return 1
			}
			req.Header.Set("Authorization", "Bearer "+scCfg.Server.Token)
			resp, doErr := doHTTP(req)
			if doErr != nil {
				fmt.Fprintf(stderr, "%s: scheduler run: %v\n", binaryName, doErr)
				return 1
			}
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				fmt.Fprintf(stderr, "%s: scheduler run: server returned %s\n", binaryName, resp.Status)
				return 1
			}
			fmt.Fprintf(stdout, "Task %s triggered.\n", schedulerArg)

		case "enable":
			if schedulerArg == "" {
				fmt.Fprintf(stderr, "Usage: %s scheduler enable <id>\n", binaryName)
				return 2
			}
			// enable/disable write to the DB directly — no running server needed.
			if err := scDB.SetTaskEnabled(schedulerArg, true); err != nil {
				fmt.Fprintf(stderr, "%s: scheduler enable: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Task %s enabled.\n", schedulerArg)

		case "disable":
			if schedulerArg == "" {
				fmt.Fprintf(stderr, "Usage: %s scheduler disable <id>\n", binaryName)
				return 2
			}
			// enable/disable write to the DB directly — no running server needed.
			if err := scDB.SetTaskEnabled(schedulerArg, false); err != nil {
				fmt.Fprintf(stderr, "%s: scheduler disable: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Task %s disabled.\n", schedulerArg)

		case "history":
			if schedulerArg == "" {
				fmt.Fprintf(stderr, "Usage: %s scheduler history <id>\n", binaryName)
				return 2
			}
			history, err := scDB.ListTaskHistory(schedulerArg, 20)
			if err != nil {
				fmt.Fprintf(stderr, "%s: scheduler history: %v\n", binaryName, err)
				return 1
			}
			if len(history) == 0 {
				fmt.Fprintf(stdout, "No history for task %q.\n", schedulerArg)
			} else {
				fmt.Fprintf(stdout, "%-24s %-24s %-10s %-10s %s\n",
					"STARTED", "FINISHED", "STATUS", "DURATION", "ERROR")
				fmt.Fprintf(stdout, "%-24s %-24s %-10s %-10s %s\n",
					"------------------------", "------------------------",
					"----------", "----------", "-----")
				for _, h := range history {
					dur := fmt.Sprintf("%dms", h.DurationMS)
					errStr := h.ErrorMsg
					if errStr == "" {
						errStr = "-"
					}
					fmt.Fprintf(stdout, "%-24s %-24s %-10s %-10s %s\n",
						h.StartedAt.Format("2006-01-02 15:04:05"),
						h.FinishedAt.Format("2006-01-02 15:04:05"),
						h.Status, dur, errStr)
				}
			}

		default:
			fmt.Fprintf(stderr, "%s: unknown scheduler subcommand: %s\n", binaryName, schedulerCmd)
			fmt.Fprintf(stderr, "Usage: %s scheduler {list|show|run|enable|disable|history} [id]\n", binaryName)
			return 2
		}
		return 0
	}

	// ── Token management CLI ──────────────────────────────────────────────────

	if tokenCmd != "" {
		tkCfgFile := filepath.Join(paths.GetConfigDir(appName), "server.yml")
		tkCfg, _ := config.Load(tkCfgFile)
		if tkCfg.Database.Path == "" {
			tkCfg.Database.Path = paths.GetDBPath(appName)
		}
		tkDB, err := database.NewDatabase(tkCfg.Database.Type, tkCfg.Database.Path)
		if err != nil {
			fmt.Fprintf(stderr, "%s: token: database: %v\n", binaryName, err)
			return 1
		}
		defer tkDB.Close()

		switch tokenCmd {
		case "list":
			tokens, err := tkDB.ListAPITokens()
			if err != nil {
				fmt.Fprintf(stderr, "%s: token list: %v\n", binaryName, err)
				return 1
			}
			if len(tokens) == 0 {
				fmt.Fprintln(stdout, "No active tokens.")
			} else {
				fmt.Fprintf(stdout, "%-14s %-8s %-14s %-24s %-24s\n",
					"PREFIX", "TYPE", "RESOURCE", "CREATED", "EXPIRES")
				fmt.Fprintf(stdout, "%-14s %-8s %-14s %-24s %-24s\n",
					"--------------", "--------", "--------------",
					"------------------------", "------------------------")
				for _, t := range tokens {
					expires := "never"
					if t.ExpiresAt != nil {
						expires = t.ExpiresAt.Format("2006-01-02 15:04:05")
					}
					fmt.Fprintf(stdout, "%-14s %-8s %-14s %-24s %-24s\n",
						t.TokenPrefix, t.ResourceType, t.ResourceID,
						t.CreatedAt.Format("2006-01-02 15:04:05"), expires)
				}
			}

		case "revoke":
			if tokenArg == "" {
				fmt.Fprintf(stderr, "Usage: %s token revoke <prefix>\n", binaryName)
				return 2
			}
			if err := tkDB.RevokeAPIToken(tokenArg, "revoked via CLI"); err != nil {
				fmt.Fprintf(stderr, "%s: token revoke: %v\n", binaryName, err)
				return 1
			}
			fmt.Fprintf(stdout, "Token %s revoked.\n", tokenArg)

		default:
			fmt.Fprintf(stderr, "%s: unknown token subcommand: %s\n", binaryName, tokenCmd)
			fmt.Fprintf(stderr, "Usage: %s token {list|revoke} [prefix]\n", binaryName)
			return 2
		}
		return 0
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

	// Priority order for mode: (1) --mode CLI flag, (2) MODE env var, (3) default "production" (PART 6).
	// Priority order for debug: (1) --debug CLI flag, (2) DEBUG env var, (3) default false (PART 6).
	// Apply env vars first (lowest priority), then CLI flags override.
	mode.FromEnv()
	if modeFlag != "" {
		if err := mode.Set(modeFlag); err != nil {
			log.Printf("warning: %v", err)
		}
	}

	if debugFlag {
		mode.SetDebugEnabled(true)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("debug mode enabled")
	} else if mode.IsDebugEnabled() {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("debug mode enabled via DEBUG env var")
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
		// Spec (PART 4): cache and backup are subdirectories of dataDir.
		// Cascade the override so --data moves all data-relative dirs together,
		// unless the operator has explicitly overridden each one separately.
		if cacheFlag == "" {
			cacheDir = filepath.Join(dataDir, "cache")
		}
		if backupFlag == "" {
			backupDir = filepath.Join(dataDir, "backup")
		}
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

	// --status probes the live server via HTTP and exits 0=healthy, 1=unhealthy (PART 8).
	// Must run after paths are resolved but loads config independently.
	if showStatus {
		cfgPath := filepath.Join(configDir, "server.yml")
		if configFlag != "" {
			cfgPath = configFlag
		}
		statusCfg, _ := config.Load(cfgPath)
		port := statusCfg.Server.Port
		if port == "" {
			port = "80"
		}
		address := statusCfg.Server.Address
		if address == "" || address == "0.0.0.0" {
			address = "127.0.0.1"
		}
		probeURL := "http://" + address + ":" + port + "/api/v1/server/healthz"
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(probeURL)
		if err != nil || resp.StatusCode >= 500 {
			fmt.Fprintf(stderr, "%s: unhealthy — %v\n", binaryName, err)
			return 1
		}
		resp.Body.Close()
		fmt.Fprintf(stdout, "%s: healthy (port %s)\n", binaryName, port)
		return 0
	}

	// ── Load config ───────────────────────────────────────────────────────────

	cfgFile := filepath.Join(configDir, "server.yml")
	if configFlag != "" {
		cfgFile = configFlag
	}

	// Auto-migrate server.yaml → server.yml if the old filename exists and the new one does not.
	if configFlag == "" {
		yamlPath := filepath.Join(configDir, "server.yaml")
		if _, statErr := os.Stat(yamlPath); statErr == nil {
			if _, statErr2 := os.Stat(cfgFile); os.IsNotExist(statErr2) {
				if renameErr := os.Rename(yamlPath, cfgFile); renameErr == nil {
					log.Printf("config: migrated server.yaml → server.yml")
				} else {
					log.Printf("config: could not migrate server.yaml → server.yml: %v", renameErr)
				}
			}
		}
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
		// Derive from the (possibly --data-overridden) dataDir, not the global
		// paths.GetDBPath() which ignores the --data flag.
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
		return 0
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
		n, err := db.DeleteExpiredAPITokens()
		if err != nil {
			return err
		}
		if n > 0 {
			log.Printf("scheduler: removed %d expired/revoked API tokens", n)
		}
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

	// ── Runtime detection (PART 7/8) ──────────────────────────────────────────
	// Detect hostname and CPU count at startup — never hardcode these values.

	hostname, hostErr := os.Hostname()
	if hostErr != nil {
		hostname = "unknown"
		log.Printf("warning: could not detect hostname: %v", hostErr)
	}
	numCPU := runtime.NumCPU()
	log.Printf("host: %s | cpus: %d | mode: %s", hostname, numCPU, mode.Get())

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

	// Register privilege drop: after binding the listen port as root, switch to the
	// unprivileged service account, chowning runtime paths first (PART 23 step 8g).
	srv.SetPrivilegeDrop(cfg.Server.User, cfg.Server.Group, []string{
		configDir, dataDir, logsDir, cacheDir, backupDir, filepath.Dir(pidFile),
	})

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
		return 0
	}

	runServer()
	return 0
}

// normalizeArgs expands the two spec-allowed short flags: -h → --help, -v → --version.
// All other arguments are passed through unchanged. Single-dash multi-character flags
// are NOT converted — spec mandates short flags are ONLY -h and -v (PART 8).
func normalizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "-h":
			out = append(out, "--help")
		case "-v":
			out = append(out, "--version")
		default:
			out = append(out, a)
		}
	}
	return out
}

// applyColor applies the --color flag, updating NO_COLOR as needed.
// Spec canonical values: auto, always, never (PART 8 / binary-rules.md).
// yes/no are accepted as backward-compatible aliases.
func applyColor(v string) {
	switch v {
	case "never", "no":
		os.Setenv("NO_COLOR", "1")
	case "always", "yes":
		os.Unsetenv("NO_COLOR")
	}
	// "auto" or empty: leave NO_COLOR as-is
}

// printHelp writes usage information to w.
func printHelp(w io.Writer, name string) {
	fmt.Fprintf(w, `%s %s - a fast, public pastebin service

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
      --color {auto|always|never}   Color output (default: auto)
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
	req.Header.Set("User-Agent", "pastebin/"+Version)
	return req, nil
}

// doHTTP executes an HTTP request using the default client.
func doHTTP(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}
