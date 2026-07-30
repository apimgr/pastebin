# Project Audit

Started: 2026-07-27

All auto-fixable findings are resolved (see Completed). Only the items in
Flagged remain — they are design decisions that require operator input and
were intentionally NOT auto-changed.

## Flagged (design decisions — NOT auto-fixed)
## Resolved (operator decisions)
- src/main.go: refactored CLI argument parsing from a hand-rolled for-loop +
  switch (previously lines 64-188) to a `flag.NewFlagSet(binaryName,
  flag.ContinueOnError)`-based parser, per `binary-rules.md` ("never
  hand-roll argument parsing for the server binary"). All primary flags
  (`--help`, `--version`, `--status`, `--daemon`, `--debug`, `--port`,
  `--address`, `--mode`, `--config`, `--data`, `--log`, `--cache`,
  `--backup`, `--pid`, `--baseurl`, `--color`, `--lang`, `--shell`,
  `--service`, `--maintenance`, `--update`) are registered via
  `BoolVar`/`StringVar`. Multi-positional subcommands (`--shell completions
  bash`, `--maintenance pgp generate`, `--maintenance token revoke <prefix>`,
  `--update branch stable`) are handled via `fs.Args()` leftover positionals
  after the flag that names the subcommand. `normalizeArgs()` is kept (still
  unit-tested by name in `main_helpers_test.go`) but simplified to only
  expand `-h`/`-v` short aliases — the `--flag=value` splitting branch was
  removed since stdlib `flag.Parse` accepts `=`-form natively. Exit codes
  (0/1/2) and the unknown-flag stderr message are unchanged. Verified via
  `make test` (Docker, `casjaysdev/go:latest`): all tests pass, coverage
  75.6% (>= 60% required).
- Dead-symbol cleanup pass (explicitly requested). A whole-program
  `golang.org/x/tools/cmd/deadcode ./...` run (casjaysdev/go:latest) reported
  ~60 "unreachable func" hits. Each was triaged individually with `grep -rn`
  across src/, *_test.go, tests/*.sh, docs/, and templates. Result: ZERO
  symbols were safe to delete — every hit is a false positive of the
  whole-program pass, which by design ignores tests, interface satisfaction,
  and build-tag variants. Nothing removed. Breakdown of why each was KEPT:
  - Test-covered package public API (majority): the deadcode pass ignores
    `_test.go`, so black-box/white-box tests keep these alive as legitimate
    library API. Verified test callers exist for: theme.DarkTheme/LightTheme;
    display.DetectDisplayEnv/CanUseANSI/NewSpinner/ShowProgress/IsDumbTerminal/
    IsAutoDetectDisplayMode{GUI,TUI,CLI,Headless}/autoDetectDisplayMode;
    i18n.TranslateFormat/TranslatePlural/LocaleFS/toString/pluralForm;
    terminal.NewSymbolSet; theme.GetThemePalette/IsSystemDarkTheme;
    config.MustParseBool/IsFalsy; maintenance.SetYAMLField;
    mode.IsDevelopment/IsProduction/Initialize/GetErrorDetail/GetCacheHeaders/
    GetLogLevel/ShouldCacheTemplates/ShouldEnableAutoReload/
    ShouldEnableProfiling/GetPanicRecoveryMode; tor.GetHTTPClient/UpdateConfig/
    RegenerateAddress/ApplyKeys/updateTorrc/writeIfChanged; task.SSLRenewal;
    server.BaseDomain/WildcardDomain/readPrivateKey/decryptSecurityReport;
    metric.New; pgp.Decrypt; handler.mapAPIErrorCodeToHTTPStatus/sendAPIError.
  - Interface satisfaction: display.TextSpinner/ANSISpinner
    Start/Stop/SetMessage implement the `Spinner` interface (spinner.go:10,
    returned by NewSpinner); health.Monitor.State is part of the monitor's
    State/service surface. Removing any would break the interface.
  - Build-tag platform variants: display.detectPlatformDisplay (detect_unix.go
    + detect_windows.go), terminal.OnResize (resize_unix.go +
    resize_windows.go), theme.isLinuxDarkTheme/isMacOSDarkTheme/
    isWindowsDarkTheme/isTerminalDarkTheme — each is a per-OS impl reached only
    on its target platform; the whole-program pass sees one build's view.
  - Internal helper of a tested public function: i18n.fmt_int has no direct
    test but is called by i18n.toString (i18n.go:279), which IS tested.
  - Unwired but spec-mandated (KEEP + wire, never delete): server.RotateKeypair
    (PGP keypair rotation, 30-day grace window) has zero callers but is a
    required cryptographic-key-rotation capability; handler.sendAPIError +
    mapAPIErrorCodeToHTTPStatus (PART 9 canonical error envelope) are
    implemented + unit-tested but no handler routes through them (handlers
    write the `{ok:false}` envelope inline at paste.go:847). Both logged to
    TODO.AI.md as wire-up tasks so they are not re-flagged as "dead" later.
  No code changed (docs only), so the build is unaffected; no `go vet`
  regression is possible from this pass.
- cache Get/Set/Delete appeared unused as of the audit start. Operator
  decision: keep `Delete` per AI.md/IDEA.md. Wired in commit 1a99645eb6d1
  ("Wire paste cache Get/Set/Delete into handler") — `pasteCacheKey`/
  `getCachedPaste`/`cachePaste`/`invalidatePasteCache` in
  `src/handler/paste.go`, wired into `loadLivePaste`, `GetPasteForWeb`,
  `DeletePaste`, and the burn-after-read deletion paths; burn-after-read
  pastes are never cached so the DB-authoritative Views count stays correct.
- security.yml vs ci.yml consolidation: `.github/workflows/security.yml` was
  a forbidden duplicate — cicd-rules.md requires security jobs to live
  inside `ci.yml` with no separate `security.yml`, and `ci.yml` already ran
  `secret-scan`/`workflow-policy`/`vuln-scan`. Deleted `security.yml`
  (commit 356047ddf224); no `act` verification was needed since nothing was
  restructured, only the duplicate file removed. Pushed; CI, Daily Build,
  and Docker Build all confirmed green on main afterward. A full spec sweep
  of `/server/security**` (handlers, templates, config, i18n, all 37 related
  tests) found no other issues — feature is spec-compliant.
- task/task.go: the `update_available` scheduler email was dead — PART 17
  defined only 8 email templates and the call used CamelCase keys that never
  substituted against the snake_case renderer. Operator decision: PART 17 now
  documents 10 templates; added `update_available`/`update_installed` embedded
  templates, fixed the key casing to snake_case, added config-gated
  `notifyAvailable`/`notifyInstalled` toggles wired from
  `server.notifications.email.events.{update_available,update_installed}`,
  and added an `update_installed` send before `RestartSelf()`. Unit tests
  added in task/task_email_test.go via extracted `updateSendAvailable`/
  `updateSendInstalled` helpers.

## Completed
- config/config.go: Save() now writes server.yml atomically (temp + fsync +
  rename); the two ignored Save errors (generated token/key persistence) now
  log a warning instead of silently dropping.
- maintenance/maintenance.go: SetYAMLField and SetMode write server.yml
  atomically via a shared atomicWriteFile helper.
- ssl/ssl.go + ssl/selfsigned.go: cert/key pairs written key-first and each
  write is atomic (DNS-01 and self-signed overlay paths) — no torn cert/key.
- handler/paste.go: ParseExpiry default case guards against int64 nanosecond
  overflow that made large expiries wrap to immediate expiry.
- handler/paste.go: HashToken doc comment corrected (no server-layer caller;
  exported for the black-box package test).
- pid/pid_unix.go: signal(0) EPERM now treated as live-but-other-owner, not
  dead (fixes false "not running" during the root→pastebin privilege drop).
- docs/configuration.md: environment-variable table expanded to the full
  config-override set (server, identity, DB/cache, SMTP_*, dir overrides).
- docker/docker-compose{,.dev,.test}.yml: converted env from list style to
  map style; removed MODE/DEBUG from prod (production defaults); dev/test set
  MODE: dev + DEBUG: 1 per PART 26.
