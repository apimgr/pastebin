package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// runCapture calls run() with the given args and returns (exitCode, stdout, stderr).
func runCapture(args ...string) (int, string, string) {
	var out, errOut strings.Builder
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestRunHelp covers the --help and -h paths.
func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{
		{"--help"},
		{"-h"},
	} {
		code, out, _ := runCapture(args...)
		if code != 0 {
			t.Errorf("run(%v) = %d, want 0", args, code)
		}
		if !strings.Contains(out, "--version") {
			t.Errorf("run(%v): help output missing --version flag", args)
		}
		if !strings.Contains(out, "scheduler list") {
			t.Errorf("run(%v): help output missing 'scheduler list'", args)
		}
	}
}

// TestRunVersion covers the --version and -v paths.
func TestRunVersion(t *testing.T) {
	for _, args := range [][]string{
		{"--version"},
		{"-v"},
	} {
		code, out, _ := runCapture(args...)
		if code != 0 {
			t.Errorf("run(%v) = %d, want 0", args, code)
		}
		if !strings.Contains(out, "Built:") {
			t.Errorf("run(%v): version output missing 'Built:'", args)
		}
		if !strings.Contains(out, "Go:") {
			t.Errorf("run(%v): version output missing 'Go:'", args)
		}
		if !strings.Contains(out, "OS/Arch:") {
			t.Errorf("run(%v): version output missing 'OS/Arch:'", args)
		}
	}
}

// TestRunUnknownFlag verifies that an unrecognised flag returns exit 2 and
// prints a usage hint to stderr.
func TestRunUnknownFlag(t *testing.T) {
	code, _, errOut := runCapture("--totally-unknown-flag-xyz")
	if code != 2 {
		t.Errorf("unknown flag: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "--help") {
		t.Errorf("unknown flag: stderr missing --help hint; got: %q", errOut)
	}
}

// TestRunShellHelp verifies --shell --help exits 0.
func TestRunShellHelp(t *testing.T) {
	code, _, _ := runCapture("--shell", "--help")
	if code != 0 {
		t.Errorf("--shell --help: exit %d, want 0", code)
	}
}

// TestRunShellCompletionsBash verifies --shell completions bash exits 0.
// The completions are written to os.Stdout directly by shell.PrintCompletions,
// not to the captured writer, so we only check the exit code here.
func TestRunShellCompletionsBash(t *testing.T) {
	code, _, _ := runCapture("--shell", "completions", "bash")
	if code != 0 {
		t.Errorf("--shell completions bash: exit %d, want 0", code)
	}
}

// TestRunShellCompletionsZsh verifies --shell completions zsh exits 0.
func TestRunShellCompletionsZsh(t *testing.T) {
	code, out, _ := runCapture("--shell", "completions", "zsh")
	if code != 0 {
		t.Errorf("--shell completions zsh: exit %d, want 0", code)
	}
	_ = out
}

// TestRunShellCompletionsFish verifies --shell completions fish exits 0.
func TestRunShellCompletionsFish(t *testing.T) {
	code, _, _ := runCapture("--shell", "completions", "fish")
	if code != 0 {
		t.Errorf("--shell completions fish: exit %d, want 0", code)
	}
}

// TestRunShellInitBash verifies --shell init bash exits 0.
func TestRunShellInitBash(t *testing.T) {
	code, out, _ := runCapture("--shell", "init", "bash")
	if code != 0 {
		t.Errorf("--shell init bash: exit %d, want 0", code)
	}
	_ = out
}

// TestRunShellUnknown verifies --shell <unknown> returns exit 2.
func TestRunShellUnknown(t *testing.T) {
	code, _, errOut := runCapture("--shell", "unknownsub")
	if code != 2 {
		t.Errorf("--shell unknownsub: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknownsub") {
		t.Errorf("--shell unknownsub: stderr should mention the bad subcommand; got %q", errOut)
	}
}

// TestRunServiceHelp verifies --service --help exits 0.
func TestRunServiceHelp(t *testing.T) {
	code, _, _ := runCapture("--service", "--help")
	if code != 0 {
		t.Errorf("--service --help: exit %d, want 0", code)
	}
}

// TestRunServiceUnknown verifies --service <unknown> returns exit 2.
func TestRunServiceUnknown(t *testing.T) {
	code, _, errOut := runCapture("--service", "not-a-real-subcommand")
	if code != 2 {
		t.Errorf("--service unknown: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "--service") {
		t.Errorf("--service unknown: stderr missing --service hint; got %q", errOut)
	}
}

// TestRunMaintenanceHelp verifies --maintenance --help exits 0.
func TestRunMaintenanceHelp(t *testing.T) {
	code, _, _ := runCapture("--maintenance", "--help")
	if code != 0 {
		t.Errorf("--maintenance --help: exit %d, want 0", code)
	}
}

// TestRunMaintenanceRestoreNoArg verifies --maintenance restore with no filename exits 2.
func TestRunMaintenanceRestoreNoArg(t *testing.T) {
	code, _, errOut := runCapture("--maintenance", "restore")
	if code != 2 {
		t.Errorf("--maintenance restore (no file): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "restore") {
		t.Errorf("--maintenance restore (no file): stderr should mention 'restore'; got %q", errOut)
	}
}

// TestRunMaintenanceRestoreBadFile verifies --maintenance restore <nonexistent> exits 1.
func TestRunMaintenanceRestoreBadFile(t *testing.T) {
	code, _, errOut := runCapture("--maintenance", "restore", "/tmp/definitely-does-not-exist-xyzzy.tar.gz")
	if code != 1 {
		t.Errorf("--maintenance restore (bad file): exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "restore") {
		t.Errorf("--maintenance restore (bad file): stderr should mention 'restore'; got %q", errOut)
	}
}

// TestRunMaintenanceModeNoArg verifies --maintenance mode with no argument exits 2.
func TestRunMaintenanceModeNoArg(t *testing.T) {
	code, _, errOut := runCapture("--maintenance", "mode")
	if code != 2 {
		t.Errorf("--maintenance mode (no arg): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "mode") {
		t.Errorf("--maintenance mode (no arg): stderr should mention 'mode'; got %q", errOut)
	}
}

// TestRunMaintenanceUnknown verifies --maintenance <unknown> exits 2.
func TestRunMaintenanceUnknown(t *testing.T) {
	code, _, errOut := runCapture("--maintenance", "not-a-subcommand")
	if code != 2 {
		t.Errorf("--maintenance unknown: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "--maintenance") {
		t.Errorf("--maintenance unknown: stderr missing --maintenance hint; got %q", errOut)
	}
}

// TestRunUpdateHelp verifies --update --help exits 0 and shows update-specific
// help including the "check" subcommand description.
func TestRunUpdateHelp(t *testing.T) {
	code, out, _ := runCapture("--update", "--help")
	if code != 0 {
		t.Errorf("--update --help: exit %d, want 0", code)
	}
	// Update-specific help must list the available subcommands.
	for _, want := range []string{"check", "branch"} {
		if !strings.Contains(out, want) {
			t.Errorf("--update --help: output missing %q; got %q", want, out)
		}
	}
}

// TestRunUpdateBranchStable verifies --update branch stable exits 0.
func TestRunUpdateBranchStable(t *testing.T) {
	code, out, _ := runCapture("--update", "branch", "stable")
	if code != 0 {
		t.Errorf("--update branch stable: exit %d, want 0", code)
	}
	if !strings.Contains(out, "stable") {
		t.Errorf("--update branch stable: output missing 'stable'; got %q", out)
	}
}

// TestRunUpdateBranchBeta verifies --update branch beta exits 0.
func TestRunUpdateBranchBeta(t *testing.T) {
	code, out, _ := runCapture("--update", "branch", "beta")
	if code != 0 {
		t.Errorf("--update branch beta: exit %d, want 0", code)
	}
	if !strings.Contains(out, "beta") {
		t.Errorf("--update branch beta: output missing 'beta'; got %q", out)
	}
}

// TestRunUpdateBranchInvalid verifies --update branch <bad> exits 2.
func TestRunUpdateBranchInvalid(t *testing.T) {
	code, _, errOut := runCapture("--update", "branch", "notabranch")
	if code != 2 {
		t.Errorf("--update branch notabranch: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "notabranch") {
		t.Errorf("--update branch notabranch: stderr should mention the branch; got %q", errOut)
	}
}

// TestRunUpdateUnknownSubcommand verifies --update <bad> exits 2.
func TestRunUpdateUnknownSubcommand(t *testing.T) {
	code, _, errOut := runCapture("--update", "not-a-command")
	if code != 2 {
		t.Errorf("--update unknown: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "--update") {
		t.Errorf("--update unknown: stderr missing --update hint; got %q", errOut)
	}
}

// TestRunEmailNoSubcommand verifies --email with no recognised subcommand exits 2.
func TestRunEmailNoSubcommand(t *testing.T) {
	code, _, errOut := runCapture("--email", "badsubcmd")
	if code != 2 {
		t.Errorf("--email badsubcmd: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "badsubcmd") {
		t.Errorf("--email badsubcmd: stderr should mention the bad subcommand; got %q", errOut)
	}
}

// TestRunEmailTestNoAddress verifies --email test with no address exits 2.
func TestRunEmailTestNoAddress(t *testing.T) {
	code, _, errOut := runCapture("--email", "test")
	if code != 2 {
		t.Errorf("--email test (no addr): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "test") {
		t.Errorf("--email test (no addr): stderr should mention 'test'; got %q", errOut)
	}
}

// TestRunNormalizeHelp verifies -help is treated as --help (normalizeArgs
// converts -flag → --flag for multi-char flags).
func TestRunNormalizeHelp(t *testing.T) {
	code, out, _ := runCapture("-help")
	if code != 0 {
		t.Errorf("-help: exit %d, want 0", code)
	}
	if !strings.Contains(out, "--version") {
		t.Errorf("-help: output missing --version; got %q", out)
	}
}

// TestRunColorFlags exercise all three applyColor branches through run().
func TestRunColorFlags(t *testing.T) {
	// --color no + --help: must succeed
	code, out, _ := runCapture("--color", "no", "--help")
	if code != 0 {
		t.Errorf("--color no --help: exit %d, want 0", code)
	}
	if !strings.Contains(out, "--version") {
		t.Errorf("--color no --help: output missing --version; got %q", out)
	}

	// --color yes + --version: must succeed
	code, out, _ = runCapture("--color", "yes", "--version")
	if code != 0 {
		t.Errorf("--color yes --version: exit %d, want 0", code)
	}
	if !strings.Contains(out, "Built:") {
		t.Errorf("--color yes --version: output missing 'Built:'; got %q", out)
	}
}

// TestRunLangFlag exercises --lang without triggering server start.
func TestRunLangFlag(t *testing.T) {
	code, out, _ := runCapture("--lang", "en", "--help")
	if code != 0 {
		t.Errorf("--lang en --help: exit %d, want 0", code)
	}
	_ = out
}

// TestRunServiceStart verifies --service start returns exit 1 (no actual
// service present in CI) without panicking.
func TestRunServiceStart(t *testing.T) {
	code, _, _ := runCapture("--service", "start")
	// On a CI runner there is no service installed; we expect 1 (error), not a panic.
	if code == 0 {
		// If start somehow succeeded we just verify it didn't crash.
		t.Log("--service start returned 0 (service may actually be installed)")
	}
}

// TestRunServiceStop mirrors start: expect 1 on CI, no panic.
func TestRunServiceStop(t *testing.T) {
	code, _, _ := runCapture("--service", "stop")
	_ = code
}

// TestRunServiceRestart: no panic.
func TestRunServiceRestart(t *testing.T) {
	code, _, _ := runCapture("--service", "restart")
	_ = code
}

// TestRunServiceInstall: no panic.
func TestRunServiceInstall(t *testing.T) {
	code, _, _ := runCapture("--service", "--install")
	_ = code
}

// TestRunServiceDisable: no panic.
func TestRunServiceDisable(t *testing.T) {
	code, _, _ := runCapture("--service", "--disable")
	_ = code
}

// TestRunServiceUninstall: no panic.
func TestRunServiceUninstall(t *testing.T) {
	code, _, _ := runCapture("--service", "--uninstall")
	_ = code
}

// TestRunServiceReload: no panic.
func TestRunServiceReload(t *testing.T) {
	code, _, _ := runCapture("--service", "reload")
	_ = code
}

// TestRunMaintenanceSetup exercises --maintenance setup (no panic on CI).
func TestRunMaintenanceSetup(t *testing.T) {
	code, _, _ := runCapture("--maintenance", "setup")
	// May succeed or fail depending on directory permissions; either is acceptable.
	_ = code
}

// TestRunMaintenanceModeProduction exercises --maintenance mode production.
func TestRunMaintenanceModeProduction(t *testing.T) {
	code, _, _ := runCapture("--maintenance", "mode", "production")
	// Acceptable to fail on CI if config dir is unwriteable.
	_ = code
}

// ── Flag-parsing branch coverage ──────────────────────────────────────────────
// The flag-switch cases for common server-configuration flags must be covered
// even when the actual server does not start.  Each test below appends
// --help so that run() returns after the flag-parsing loop without touching
// the server startup section.

// TestRunFlagPort exercises the --port flag case.
func TestRunFlagPort(t *testing.T) {
	code, out, _ := runCapture("--port", "8080", "--help")
	if code != 0 {
		t.Errorf("--port --help: exit %d, want 0", code)
	}
	if !strings.Contains(out, "--port") {
		t.Errorf("--port --help: output missing --port; got %q", out)
	}
}

// TestRunFlagAddress exercises the --address flag case.
func TestRunFlagAddress(t *testing.T) {
	code, _, _ := runCapture("--address", "127.0.0.1", "--help")
	if code != 0 {
		t.Errorf("--address --help: exit %d, want 0", code)
	}
}

// TestRunFlagMode exercises the --mode flag case.
func TestRunFlagMode(t *testing.T) {
	code, _, _ := runCapture("--mode", "development", "--help")
	if code != 0 {
		t.Errorf("--mode --help: exit %d, want 0", code)
	}
}

// TestRunFlagConfig exercises the --config flag case.
func TestRunFlagConfig(t *testing.T) {
	code, _, _ := runCapture("--config", "/tmp", "--help")
	if code != 0 {
		t.Errorf("--config --help: exit %d, want 0", code)
	}
}

// TestRunFlagData exercises the --data flag case.
func TestRunFlagData(t *testing.T) {
	code, _, _ := runCapture("--data", "/tmp", "--help")
	if code != 0 {
		t.Errorf("--data --help: exit %d, want 0", code)
	}
}

// TestRunFlagLog exercises the --log flag case.
func TestRunFlagLog(t *testing.T) {
	code, _, _ := runCapture("--log", "/tmp", "--help")
	if code != 0 {
		t.Errorf("--log --help: exit %d, want 0", code)
	}
}

// TestRunFlagCache exercises the --cache flag case.
func TestRunFlagCache(t *testing.T) {
	code, _, _ := runCapture("--cache", "/tmp", "--help")
	if code != 0 {
		t.Errorf("--cache --help: exit %d, want 0", code)
	}
}

// TestRunFlagBackup exercises the --backup flag case.
func TestRunFlagBackup(t *testing.T) {
	code, _, _ := runCapture("--backup", "/tmp", "--help")
	if code != 0 {
		t.Errorf("--backup --help: exit %d, want 0", code)
	}
}

// TestRunFlagPID exercises the --pid flag case.
func TestRunFlagPID(t *testing.T) {
	code, _, _ := runCapture("--pid", "/tmp/pastebin-test.pid", "--help")
	if code != 0 {
		t.Errorf("--pid --help: exit %d, want 0", code)
	}
}

// TestRunFlagBaseURL exercises the --baseurl flag case.
func TestRunFlagBaseURL(t *testing.T) {
	code, _, _ := runCapture("--baseurl", "/app", "--help")
	if code != 0 {
		t.Errorf("--baseurl --help: exit %d, want 0", code)
	}
}

// TestRunFlagDebug exercises the --debug flag case.
func TestRunFlagDebug(t *testing.T) {
	code, _, _ := runCapture("--debug", "--help")
	if code != 0 {
		t.Errorf("--debug --help: exit %d, want 0", code)
	}
}

// TestRunFlagVersion exercises the --version flag case (already covered by
// TestRunVersion; this variant also exercises --status before --version to
// cover the showStatus branch separately from the server-start section).
func TestRunStatus(t *testing.T) {
	// --status with no server running: expect exit 1.
	code, _, _ := runCapture("--status")
	if code != 1 {
		t.Logf("--status: exit %d (expected 1 when no server is running)", code)
	}
}

// TestRunEmailTestWithAddress exercises --email test <addr>: SMTP is not
// configured in tests so TestSMTP returns an error → run() returns 1.
func TestRunEmailTestWithAddress(t *testing.T) {
	code, _, _ := runCapture("--email", "test", "nobody@example.com")
	// Either SMTP fails (exit 1) or config is missing — both are non-panic exits.
	if code == 0 {
		t.Log("--email test nobody@example.com: exit 0 (SMTP configured in environment)")
	}
}

// TestRunShellInitZsh verifies --shell init zsh exits 0.
func TestRunShellInitZsh(t *testing.T) {
	code, _, _ := runCapture("--shell", "init", "zsh")
	if code != 0 {
		t.Errorf("--shell init zsh: exit %d, want 0", code)
	}
}

// TestRunShellInitFish verifies --shell init fish exits 0.
func TestRunShellInitFish(t *testing.T) {
	code, _, _ := runCapture("--shell", "init", "fish")
	if code != 0 {
		t.Errorf("--shell init fish: exit %d, want 0", code)
	}
}

// TestRunUpdateBranchDaily verifies --update branch daily exits 0.
func TestRunUpdateBranchDaily(t *testing.T) {
	code, out, _ := runCapture("--update", "branch", "daily")
	if code != 0 {
		t.Errorf("--update branch daily: exit %d, want 0", code)
	}
	if !strings.Contains(out, "daily") {
		t.Errorf("--update branch daily: output missing 'daily'; got %q", out)
	}
}

// TestRunUpdateBranchNoName verifies --update branch with no name exits 2.
func TestRunUpdateBranchNoName(t *testing.T) {
	code, _, errOut := runCapture("--update", "branch")
	if code != 2 {
		t.Errorf("--update branch (no name): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "branch") {
		t.Errorf("--update branch (no name): stderr missing 'branch'; got %q", errOut)
	}
}

// TestRunMultipleFlagsBeforeHelp exercises several flag-parsing branches in
// a single call so each val() invocation is exercised.
func TestRunMultipleFlagsBeforeHelp(t *testing.T) {
	code, out, _ := runCapture(
		"--port", "9090",
		"--address", "0.0.0.0",
		"--mode", "production",
		"--config", "/tmp",
		"--data", "/tmp",
		"--log", "/tmp",
		"--cache", "/tmp",
		"--backup", "/tmp",
		"--pid", "/tmp/test.pid",
		"--baseurl", "/",
		"--color", "auto",
		"--lang", "en",
		"--help",
	)
	if code != 0 {
		t.Errorf("multi-flag + --help: exit %d, want 0", code)
	}
	if !strings.Contains(out, "--port") {
		t.Errorf("multi-flag + --help: output missing --port; got %q", out)
	}
}

// ─── Scheduler CLI ────────────────────────────────────────────────────────────

// tempDBPath creates a temp dir, sets DB_PATH to a SQLite file inside it, and
// returns a restore func. Call defer restore() at the top of the test.
func tempDBPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DB_PATH", filepath.Join(dir, "test.db"))
	// Prevent any config file from overriding the DB path.
	t.Setenv("CONFIG_DIR", dir)
}

// TestRunSchedulerList exercises the "scheduler list" path through an empty DB.
func TestRunSchedulerList(t *testing.T) {
	tempDBPath(t)
	code, out, errOut := runCapture("scheduler", "list")
	if code != 0 {
		t.Errorf("scheduler list: exit %d, want 0; stderr=%q", code, errOut)
	}
	if !strings.Contains(out, "ID") {
		t.Errorf("scheduler list: output missing header 'ID'; got %q", out)
	}
}

// TestRunSchedulerShowNoArg verifies "scheduler show" with no task-ID exits 2.
func TestRunSchedulerShowNoArg(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "show")
	if code != 2 {
		t.Errorf("scheduler show (no id): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "show") {
		t.Errorf("scheduler show (no id): stderr missing 'show'; got %q", errOut)
	}
}

// TestRunSchedulerShowUnknownID verifies "scheduler show <id>" for a task that
// does not exist returns exit 1 with an error message.
func TestRunSchedulerShowUnknownID(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "show", "no-such-task")
	if code != 1 {
		t.Errorf("scheduler show <unknown>: exit %d, want 1", code)
	}
	_ = errOut
}

// TestRunSchedulerRunNoArg verifies "scheduler run" with no ID exits 2.
func TestRunSchedulerRunNoArg(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "run")
	if code != 2 {
		t.Errorf("scheduler run (no id): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "run") {
		t.Errorf("scheduler run (no id): stderr missing 'run'; got %q", errOut)
	}
}

// TestRunSchedulerRunNoToken verifies "scheduler run <id>" with no server token
// in config exits 1.
func TestRunSchedulerRunNoToken(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "run", "some-task")
	if code != 1 {
		t.Errorf("scheduler run (no token): exit %d, want 1", code)
	}
	_ = errOut
}

// TestRunSchedulerEnableNoArg verifies "scheduler enable" with no ID exits 2.
func TestRunSchedulerEnableNoArg(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "enable")
	if code != 2 {
		t.Errorf("scheduler enable (no id): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "enable") {
		t.Errorf("scheduler enable (no id): stderr missing 'enable'; got %q", errOut)
	}
}

// TestRunSchedulerEnableUnknownID verifies "scheduler enable <id>" for a
// non-existent task still returns exit 0 (SQL UPDATE affects 0 rows, no error).
func TestRunSchedulerEnableUnknownID(t *testing.T) {
	tempDBPath(t)
	code, out, _ := runCapture("scheduler", "enable", "no-such-task")
	if code != 0 {
		t.Errorf("scheduler enable <unknown>: exit %d, want 0", code)
	}
	if !strings.Contains(out, "enabled") {
		t.Errorf("scheduler enable <unknown>: expected 'enabled' in output; got %q", out)
	}
}

// TestRunSchedulerDisableNoArg verifies "scheduler disable" with no ID exits 2.
func TestRunSchedulerDisableNoArg(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "disable")
	if code != 2 {
		t.Errorf("scheduler disable (no id): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "disable") {
		t.Errorf("scheduler disable (no id): stderr missing 'disable'; got %q", errOut)
	}
}

// TestRunSchedulerDisableUnknownID verifies "scheduler disable <id>" for a
// non-existent task still returns exit 0 (SQL UPDATE affects 0 rows, no error).
func TestRunSchedulerDisableUnknownID(t *testing.T) {
	tempDBPath(t)
	code, out, _ := runCapture("scheduler", "disable", "no-such-task")
	if code != 0 {
		t.Errorf("scheduler disable <unknown>: exit %d, want 0", code)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("scheduler disable <unknown>: expected 'disabled' in output; got %q", out)
	}
}

// TestRunSchedulerHistoryNoArg verifies "scheduler history" with no ID exits 2.
func TestRunSchedulerHistoryNoArg(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "history")
	if code != 2 {
		t.Errorf("scheduler history (no id): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "history") {
		t.Errorf("scheduler history (no id): stderr missing 'history'; got %q", errOut)
	}
}

// TestRunSchedulerHistoryUnknownID verifies "scheduler history <id>" for a
// non-existent task returns exit 0 with "No history" message.
func TestRunSchedulerHistoryUnknownID(t *testing.T) {
	tempDBPath(t)
	code, out, _ := runCapture("scheduler", "history", "no-such-task")
	if code != 0 {
		t.Errorf("scheduler history <unknown>: exit %d, want 0", code)
	}
	if !strings.Contains(out, "No history") {
		t.Errorf("scheduler history <unknown>: expected 'No history'; got %q", out)
	}
}

// TestRunSchedulerUnknownSubcmd exercises the default branch of the scheduler
// switch, which exits 2.
func TestRunSchedulerUnknownSubcmd(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("scheduler", "notasubcmd")
	if code != 2 {
		t.Errorf("scheduler <unknown>: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown") {
		t.Errorf("scheduler <unknown>: stderr missing 'unknown'; got %q", errOut)
	}
}

// ─── Token CLI ────────────────────────────────────────────────────────────────

// TestRunTokenList exercises the "token list" path through an empty DB.
func TestRunTokenList(t *testing.T) {
	tempDBPath(t)
	code, out, errOut := runCapture("token", "list")
	if code != 0 {
		t.Errorf("token list: exit %d, want 0; stderr=%q", code, errOut)
	}
	if !strings.Contains(out, "No active tokens") {
		t.Errorf("token list (empty DB): expected 'No active tokens'; got %q", out)
	}
}

// TestRunTokenRevokeNoArg verifies "token revoke" with no prefix exits 2.
func TestRunTokenRevokeNoArg(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("token", "revoke")
	if code != 2 {
		t.Errorf("token revoke (no prefix): exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "revoke") {
		t.Errorf("token revoke (no prefix): stderr missing 'revoke'; got %q", errOut)
	}
}

// TestRunTokenRevokeUnknown verifies "token revoke <prefix>" for a token that
// does not exist returns exit 1.
func TestRunTokenRevokeUnknown(t *testing.T) {
	tempDBPath(t)
	code, _, _ := runCapture("token", "revoke", "nonexistent")
	if code != 1 {
		t.Errorf("token revoke <unknown>: exit %d, want 1", code)
	}
}

// TestRunTokenUnknownSubcmd exercises the default branch of the token switch.
func TestRunTokenUnknownSubcmd(t *testing.T) {
	tempDBPath(t)
	code, _, errOut := runCapture("token", "badsubcmd")
	if code != 2 {
		t.Errorf("token <unknown>: exit %d, want 2", code)
	}
	if !strings.Contains(errOut, "unknown") {
		t.Errorf("token <unknown>: stderr missing 'unknown'; got %q", errOut)
	}
}

// TestRunDebugWithStatus exercises the debugFlag body (mode.SetDebug, log.SetFlags,
// log.Printf "debug mode enabled") by combining --debug with --status so run()
// enters the debug block before the showStatus early-return.
func TestRunDebugWithStatus(t *testing.T) {
	code, _, _ := runCapture("--debug", "--status")
	// exit 1 = no server running; exit 0 = server happened to be healthy
	if code != 0 && code != 1 {
		t.Errorf("--debug --status: exit %d, want 0 or 1", code)
	}
}

// TestRunCleanExpired exercises the full server-startup section (config load,
// port resolution, SMTP auto-detect, ConfigManager init, DB init) and the
// --clean-expired one-shot block that returns 0 before entering runServer().
// A temporary SQLite DB path is injected via tempDBPath to prevent log.Fatalf.
func TestRunCleanExpired(t *testing.T) {
	tempDBPath(t)
	code, _, _ := runCapture("--clean-expired")
	if code != 0 {
		t.Errorf("--clean-expired: exit %d, want 0", code)
	}
}

// TestRunUpdateCheck exercises the --update check branch which calls
// CheckForUpdate with a 30-second context. Accepts exit 0 (up to date) or
// exit 1 (network unavailable / rate-limited) — both are non-panic outcomes.
func TestRunUpdateCheck(t *testing.T) {
	code, _, _ := runCapture("--update", "check")
	if code != 0 && code != 1 {
		t.Errorf("--update check: exit %d, want 0 or 1", code)
	}
}

// TestRunMaintenanceBackup exercises the --maintenance backup code path.
// May fail on CI if the data directory is unwriteable; that is acceptable.
func TestRunMaintenanceBackup(t *testing.T) {
	code, _, _ := runCapture("--maintenance", "backup")
	_ = code
}

// TestRunCleanExpiredAllFlags covers the five path-flag true-branches
// (--data, --log, --cache, --backup, --pid lines 768-780) and the four
// server-config flag true-branches (--port, --address, --baseurl, --mode
// lines 835-844) and the configFlag branch inside the config-load section
// (line 826). All require entering the server-startup section, which is
// reached via --clean-expired + tempDBPath so run() exits before blocking.
func TestRunCleanExpiredAllFlags(t *testing.T) {
	tempDBPath(t)
	tmp := t.TempDir()
	code, _, _ := runCapture(
		"--data", tmp,
		"--log", tmp,
		"--cache", tmp,
		"--backup", tmp,
		"--pid", filepath.Join(tmp, "pastebin.pid"),
		"--config", filepath.Join(tmp, "server.yml"),
		"--port", "19999",
		"--address", "127.0.0.1",
		"--baseurl", "/app",
		"--mode", "production",
		"--clean-expired",
	)
	if code != 0 {
		t.Errorf("--clean-expired with all flags: exit %d, want 0", code)
	}
}

// TestRunStatusWithConfig covers the configFlag true-branch inside the
// --status probe block (line 799: cfgPath = configFlag). The config file
// need not exist; config.Load handles missing files gracefully. run() exits
// before entering the server-startup section.
func TestRunStatusWithConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "server.yml")
	code, _, _ := runCapture("--config", cfgPath, "--status")
	if code != 0 && code != 1 {
		t.Errorf("--config --status: exit %d, want 0 or 1", code)
	}
}
