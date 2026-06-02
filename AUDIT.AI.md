# Project Audit

Started: 2026-05-24
Last reconciled: 2026-06-02 (scheduler API + CLI subcommands + IPv6-safe SMTP)
Scope: ALL PARTs of AI.md EXCEPT PART 27/CI workflows under `.github/workflows/` (out of scope).

## Pass 1: Security

No new violations. Argon2id N/A (no auth surface yet). SHA-256 used for delete-token storage. `crypto/rand` used for paste IDs and delete tokens. Constant-time-compare claim in IDEA.md vs. actual implementation still tracked under "Document threat model" (open).

## Pass 2: Code Quality

- [x] `src/main.go`: `--shell completions|init` IMPLEMENTED via `src/shell` package — invokes `shell.PrintHelp/PrintCompletions/PrintInit` for the server, and `shell.PrintClientCompletions` for `pastebin-cli` in `src/client/main.go`.
- [x] PID stale-detect/cleanup logic COMPLIANT with PART 8: `CheckPIDFile` (stale PID detect + remove), `WritePIDFile` (checks first, then writes), `RemovePIDFile` (deferred + signal handler). Platform files: `pid_unix.go` (signal 0 + /proc/exe + ps fallback), `pid_windows.go` (GetExitCodeProcess + QueryFullProcessImageName). Permissions fixed: `0644` root, `0600` user.

## Pass 3: Logic and Correctness

All previously listed `src/paths/paths.go` items remain fixed.

## Pass 4: Documentation Completeness — ALL RESOLVED

- [x] LICENSE.md third-party attributions present — spec PART 2 table covers all go.mod dependencies.
- [x] `man/pastebin.1` — NOT required by spec (AI.md has no man page requirement); removed from open list.
- [x] README.md `## Environment Variables` section exists (lines 128–166) covering all server env vars, client env vars, and a platform-defaults table.

## Pass 5: Spec and Rules Compliance

### Scheduler (PART 18) — RESOLVED
All 10 required tasks registered in `src/main.go`: `ssl_renewal`, `geoip_update`, `blocklist_update`, `cve_update`, `token_cleanup`, `log_rotation`, `backup_daily`, `backup_hourly` (disabled by default), `healthcheck_self`, `tor_health`. Implementations live in `src/task/task.go`.

### Docker (PART 26) — RESOLVED
`docker/Dockerfile`, `docker/Dockerfile.dev`, `docker/Dockerfile.build`, `docker/docker-compose.yml`, `docker/docker-compose.dev.yml`, `docker/docker-compose.test.yml` all present.

### Tests (PART 28/29) — RESOLVED
`tests/docker.sh`, `tests/incus.sh`, `tests/run_tests.sh` all present.

### Docs (PART 29) — RESOLVED
`docs/installation.md`, `docs/configuration.md`, `docs/integrations.md`, `docs/development.md` all present.

### Shell (PART 7/8) — RESOLVED
`src/shell/` package implements server and client completions and init; both `pastebin` and `pastebin-cli` route `--shell completions|init` through it.

### i18n (PART 30) — RESOLVED THIS PASS
All 7 locales (`en`, `fr`, `de`, `es`, `pt`, `ja`, `zh`) now have full key parity with `en.json`. Missing keys merged:
- `de.json`, `es.json`, `fr.json`, `pt.json`: +82 keys each (paste.*, home.*, recent.*, qr.*, footer.*, nav.create/recent/api, health.*)
- `ja.json`, `zh.json`: +91 keys each (same set + 9 plurals.* entries)
Templates in `src/server/templates/*.html` already use `{{t .Lang "key"}}` — no template changes needed.

### GeoIP (features-rules.md) — RESOLVED
`src/geoip/geoip.go` uses `github.com/oschwald/maxminddb-golang`. No `geoip2-golang` dependency.

### Client (PART 32) — RESOLVED
`src/client/main.go` `defaultServer` is intentionally empty `""` per PART 32: "no compiled-in default server". User must supply `--server` or `$PASTEBIN_SERVER`. Comment documents this explicitly.

### Live config reload (PART 5/PART 8) — RESOLVED THIS PASS
`config.ConfigManager` implemented in `src/config/config.go`:
- 5-second ticker polling `os.Stat` modtime (NOT fsnotify — spec uses ticker)
- Hot-reload: `RateLimit.*`, `Web.Security.CORS`, `Web.SiteTitle`, `Web.Theme`, `Server.Logging.Level`
- Restart-required: `Server.Port`, `Server.Address`, `Database.*`, `Server.Tor.*` (logged as warnings)
- `Server.OnConfigChange` callback updates rate limiter limits live
- `Server.liveCfg()` returns manager's current config for per-request hot-reload

## Pass 6: Code Flow Trace — RESOLVED THIS PASS

- [x] `src/server/server.go`: native routes use singular `/api/v1/paste/{id}` for CRUD and plural `/api/v1/pastes` for list — matches IDEA.md spec exactly.
- [x] lenpaste (`/api/new`, `/api/get`, `/api/remove`, `/api/list`, `/api/v1/new`, `/api/v1/get`, `/api/v1/getServerInfo`) and pastebin.com (`/api/api_post.php`, `/api/api_raw.php`, `/api/api_login.php`) compat routes verified present and wired to `CompatHandler`.
- [x] README.md `## Environment Variables` section present — covers all server and client env vars with platform defaults table.

## Notes

- `man/pastebin.1` — NOT required by spec; removed from open list.
- LICENSE.md third-party attributions: spec PART 2 table covers all go.mod deps — resolved in prior pass.

## Pass 7: PART 15 (SSL/TLS) — RESOLVED

- [x] `src/ssl/ssl.go` completely rewritten: implements exact 4-path cert lookup order from PART 15 (system certbot literal, system certbot FQDN, app-managed LE, user-provided local)
- [x] `validateCertFile`: PEM decode loop, expiry check, `x509.VerifyHostname`
- [x] `getLetsEncryptTLSConfig`: autocert with DirCache at `{cert_dir}/letsencrypt/{fqdn}/`; staging CA via `acme.Client{DirectoryURL: LetsEncryptStagingURL}`
- [x] `ParseChallenge`: normalises http-01/tls-alpn-01/dns-01 variants
- [x] `server.go` Run(): ssl.Manager wired when TLS enabled; falls through to HTTP on error

## Pass 8: PART 23/24 (Service) — RESOLVED

- [x] `src/service/service.go`: systemd unit follows PART 24 template exactly — no User=/Group= (binary drops privileges), StandardOutput/StandardError=journal, ProtectHome=yes, PrivateTmp=yes, 4 ReadWritePaths including /var/cache, correct RestartSec=5
- [x] All launchd paths updated to `io.apimgr.pastebin` (launchdLabel constant) — Start, Stop, Disable, installLaunchd, uninstallLaunchd
- [x] launchd log paths: `/var/log/apimgr/pastebin/stdout.log` + `stderr.log` per PART 24
- [x] `strings.Title` deprecation fixed: `strings.ToUpper(appName[:1]) + appName[1:]`
- [x] PART 23 compliance: service install does NOT create service user (binary handles on startup per spec)
- [x] serviceUser, launchdLabel (io.apimgr.pastebin), serviceUID=300 constants documented

## Pass 9: PART 11 App Secrets + Sec-Fetch Validation — RESOLVED

- [x] `database/database.go`: `app_secrets` table (CREATE TABLE IF NOT EXISTS); `EnsureAppSecret(key)` method generates 32 crypto/rand bytes on first call, stores base64-encoded, race-safe
- [x] `server.go New()`: initialises `installation_secret`, `cookie_signing_key`, `csrf_token_secret` at startup before any request
- [x] `secFetchMiddleware`: rejects Sec-Fetch-Site=cross-site on POST/PUT/PATCH/DELETE when no Bearer; rejects Sec-Fetch-Mode=navigate on /api/*; absence = pass-through (legacy compat); gated by `web.headers.sec_fetch_validation` (default: true)
- [x] `config/config.go`: `HeadersConfig{SecFetchValidation}`, `CSRFConfig{Enabled, TokenLength, CookieName, HeaderName, Secure, ExemptPaths}` added to WebConfig with spec defaults
- [ ] CSRF token validation middleware — deferred until session/auth surface is implemented (no cookie auth exists yet)

## Pass 10: PART 9/12 Cache Layer — RESOLVED

- [x] `src/cache/cache.go`: Cache interface, ErrCacheMiss sentinel, Config/DefaultConfig, New() factory
- [x] Three drivers: memoryCache (sync.RWMutex + 5-min background reaper), noopCache, redisCache (go-redis/v9, ping on construction)
- [x] `config/config.go`: CacheConfig added to ServerConfig — YAML-friendly string durations; DefaultConfig type=memory prefix=pastebin: TTL=1h
- [x] `server.go New()`: cache initialisation after torManager — maps string durations to time.Duration, falls back to memory on remote error
- [x] `server.go Run()`: defer s.cacheStore.Close()
- [x] go.mod: github.com/redis/go-redis/v9 v9.7.0 + transitive deps

## Pass 11: PART 24 Windows Service + PART 9 Error Codes + PART 16 Reserved Slugs + PART 12 Compression/Base URL + Healthz Accuracy — RESOLVED

- [x] `src/service/winsvc_windows.go`: Windows service via golang.org/x/sys/windows/svc; `IsWindowsService()`, `RunAsWindowsService()`, `windowsService.Execute()` — START_PENDING→RUNNING→STOP_PENDING lifecycle; AcceptStop|AcceptShutdown
- [x] `src/service/winsvc_other.go`: stub build tag `!windows` — `IsWindowsService()` returns false, `RunAsWindowsService` is no-op
- [x] `src/main.go`: `runServer` closure extracted; `service.IsWindowsService()` check before `runServer()`
- [x] `src/handler/paste.go`: `httpErrCode()` covers METHOD_NOT_ALLOWED(405) and MAINTENANCE(503); `mapAPIErrorCodeToHTTPStatus()` maps all 13 PART 9 error codes; `sendAPIError()` convenience wrapper added
- [x] `src/server/server.go`: `reservedSlugs` map + `isReservedSlug()` (PART 16 guard); `handleViewPaste()` returns 404 for reserved slugs before DB lookup
- [x] `src/server/server.go`: `middleware.Compress(5, ...)` added to setupRoutes(); `baseURL()` expanded to PART 12 full priority chain (X-Forwarded-Prefix > X-Forwarded-Path > X-Script-Name)
- [x] `src/database/database.go`: `CountPastes() (int64, error)` added to DB interface + implemented on SQLiteDB
- [x] `src/server/server.go buildHealthResponse()`: real cache ping with 2s timeout (`checks.Cache="error"` on failure); `PastesTotal` populated from `db.CountPastes()`
- [ ] CSRF token validation middleware — deferred until session/auth surface is implemented
- [x] PART 12: `LimitsConfig` (max_body_size, read/write/idle timeouts) + `TrustedProxiesConfig` (additional IPs/CIDRs) added to ServerConfig and DefaultConfig()
- [x] PART 12: `Validate(cfg)` — warns and replaces invalid values with defaults, called in `Load()`; never crashes startup
- [x] PART 12: `isTrustedPeer(r)` — gates X-Forwarded-* headers on loopback/private/additional list; `baseURL()` only trusts forwarded headers from trusted peers
- [x] PART 12: HTTP server timeouts read from `cfg.Server.Limits` with safe fallbacks in `Run()`

## Pass 12: PART 13 Healthz Completeness + PART 6 Debug Endpoints + PART 25 Makefile — RESOLVED

- [x] PART 13: `scheduler.Running()` method added; `schedHealthFn` callback + `SetSchedulerHealthFn()` wired into Server
- [x] PART 13: `MarkPendingRestart(key)` + `pendingRestartKeys` added to Server; `OnConfigChange()` calls it for TLS/DB/address/Tor changes
- [x] PART 13: `HealthResponse` — `PendingRestart bool`, `RestartReason []string` fields added
- [x] PART 13: `ChecksInfo` — `Scheduler string`, `Tor string (omitempty)` fields added; `buildHealthResponse()` populates them
- [x] PART 13: `buildHealthResponse()` emits `"degraded"` when cache/scheduler/Tor fail (non-fatal), `"unhealthy"` only for DB/disk
- [x] PART 6: `mode.SetDebug(bool)` + `mode.IsDebug()` added; `ShouldShowDebugEndpoints()` and `ShouldEnableProfiling()` fixed to use `IsDebug()` (not `IsDevelopment()`)
- [x] PART 6: `src/server/server.go` mounts `/debug/pprof` (net/http/pprof) and `/debug/vars` (expvar) when `mode.ShouldShowDebugEndpoints()`
- [x] PART 6: `src/main.go` calls `mode.SetDebug(true)` when `--debug` flag is set
- [x] PART 25: `Makefile` — `lint` (golangci-lint via Docker) and `help` (print targets) targets added; both in `.PHONY`
- [ ] CSRF token validation middleware — deferred until session/auth surface implemented
- PART 17 email: ✓ auto-detect SMTP, template override, silent disable — already compliant
- PART 20 metrics: ✓ Prometheus `pastebin_` prefix, token auth, all metric categories — already compliant
- PART 22 updater: ✓ stable/beta/daily branches, checksum verify, platform replace — already compliant
- PART 31 Tor: ✓ bine, ControlPort auto, ADD_ONION v3, auto-enable on binary found — already compliant

## Pass 13: PART 10 Database + PART 14 API Structure + PART 21 Backup — RESOLVED

### PART 10 (Database) — RESOLVED
- [x] `src/database/database.go`: connection pool settings added to `newSQLiteDB` — `SetMaxOpenConns(5)`, `SetMaxIdleConns(2)`, `SetConnMaxLifetime(5m)`, `SetConnMaxIdleTime(1m)` (SQLite WAL mode)
- [x] Query timeouts: `dbReadTimeout=5s`, `dbWriteTimeout=10s`, `dbComplexTimeout=15s`, `dbBulkTimeout=60s`; `dbCtx()` helper; all methods converted to `*Context` variants
- [x] Schema: `CREATE TABLE IF NOT EXISTS` with idempotent `ALTER TABLE` updates — compliant with PART 10 schema rules

### PART 14 (API Structure) — VERIFIED COMPLIANT
- Routes: plural nouns (IDEA.md spec explicitly uses singular `/api/v1/paste/{id}` for CRUD + plural `/api/v1/pastes` for list — IDEA.md overrides convention per project spec; Pass 6 already confirmed)
- JSON responses: `writeJSON()` uses `json.MarshalIndent(v, "", "  ")` + `w.Write([]byte("\n"))` — 2-space indent + single trailing newline per spec
- API types: REST, Swagger, GraphQL — all present

### PART 21 (Backup & Restore) — RESOLVED
- [x] `src/maintenance/maintenance.go`: `VerifyBackup(path, password)` — 6 post-creation checks: file exists, size>0, decrypt test, manifest present/parseable, all entries extractable to temp dir, SQLite magic header check on `.db` files
- [x] `src/task/task.go`: `BackupDaily(BackupConfig)` — uses `maintenance.Backup` for full dated archive AND daily incremental (`{project}-daily.tar.gz[.enc]`); verifies both immediately after creation; applies `applyRetention()` with priority-ordered tiers (yearly > monthly > weekly > daily)
- [x] `BackupHourly(BackupConfig)` — uses `maintenance.Backup` + `VerifyBackup`; atomic rename
- [x] `BackupConfig` struct: ProjectName, ConfigDir, DataDir, BackupDir, AppVersion, Password, Retention
- [x] `BackupRetention` struct: MaxBackups, KeepWeekly, KeepMonthly, KeepYearly
- [x] `applyRetention()`: newest-first sort, classify by tier, keep counts per tier, delete excess oldest-first
- [x] `main.go` wired to pass `configDir`, `Version`, and `BackupRetention{MaxBackups:1}` to both tasks
- [ ] Backup checksum in manifest cannot self-verify (archive2 has different bytes than archive1 whose checksum is in manifest) — tracking as deferred; all other 6 verification checks pass
- [ ] CSRF token validation middleware — deferred (no auth surface yet)

## Pass 14: PART 4 OS Paths + PART 11 Missing DB Tables + PART 32 Client — RESOLVED

### PART 4 (OS-specific paths) — RESOLVED
- [x] `src/paths/paths.go`: `GetConfigDir` — macOS root now `/Library/Application Support/apimgr/{app}`; BSD root now `/usr/local/etc/apimgr/{app}`
- [x] `GetDataDir` — macOS root `/Library/Application Support/apimgr/{app}/data`; BSD root `/var/db/apimgr/{app}`
- [x] `GetLogsDir` — macOS root `/Library/Logs/apimgr/{app}`
- [x] `GetCacheDir` — macOS root `/Library/Caches/apimgr/{app}`
- [x] `GetDBPath(appName)` added: container → `/data/db/sqlite/server.db`; native → `{dataDir}/db/server.db`
- [x] `src/main.go`: DB path now uses `paths.GetDBPath(appName)` instead of inline filepath.Join

### PART 11 (Missing DB tables) — RESOLVED
- [x] `src/database/database.go` `ensureSchema()`: added `config`, `config_meta`, `rate_limits`, `audit_log`, `backups`, `api_tokens` tables + indexes
- [x] Three separate triggers for `config_version_bump` (INSERT / UPDATE / DELETE) — SQLite cannot chain events with OR in a single trigger
- [x] All 10 required tables now present: pastes, scheduler_tasks, scheduler_history, app_secrets, config, config_meta, rate_limits, audit_log, backups, api_tokens

### PART 32 (Client) — PARTIAL RESOLUTION
- [x] `User-Agent: pastebin-cli/{version}` set on all HTTP requests (GET, POST, DELETE, autodiscover)
- [x] `cli.yml` config loading from `GetConfigDir("pastebin")/cli.yml` with YAML unmarshal; defaults: update.channel=stable, display.mode=auto
- [x] `saveCLIConfig()`: writes cli.yml with 0o644 permissions, creates parent dirs (0o700)
- [x] `SaveIfEmptyOrInvalid()`: persists `--server` to cli.yml when current server config is empty or invalid
- [x] `detectMode()`: returns "tui" when interactive terminal + no command args (PART 32 mode detection rules)
- [x] `runTUI()`: mode-detection stub — shows setup guide when no server, enforces min-version, defers full TUI
- [x] `checkCLIUpdate()`: queries `/api/autodiscover`, enforces `cli_min_version` (fatal), logs notice on newer available version
- [x] `filepath.Base(os.Args[0])` used for all display (help, version, log prefix, shell completions)
- [x] `--update check|yes` command: queries autodiscover, compares versions, prompts or downloads
- [ ] Full bubbletea TUI implementation deferred — tracked here; `runTUI()` is a stub that shows guidance and exits

## Pass 15: PART 16 JavaScript — RESOLVED

- [x] `src/server/static/js/main.js`: theme load applies only explicit `dark`/`light` localStorage values; `auto` is not written — CSS `prefers-color-scheme` handles auto mode at CSS level without JS intervention
- [x] Copy-to-clipboard handler: `[data-copy]` buttons use `navigator.clipboard.writeText`; fallback selects text via `document.createRange` when clipboard API unavailable
- [x] Submit-button loading state: disables on submit, preserves `minWidth`, maps verb labels (`create→Creating…`, `save→Saving…`, etc.), re-enables when response arrives (browser restores on page load)
- [x] Service worker registration: silent `.catch(() => {})` — no console noise on unsupported browsers
- [x] `fetchAPI` error parsing: reads RFC 7807 `detail` field first, falls back to `error`, then generic message

## Pass 16: PART 4/5 Middleware Compliance — RESOLVED

### Missing middleware (PART 5 middleware order) — RESOLVED
- [x] `src/server/security_middleware.go`: `pathSecurityMiddleware` — blocks `..` traversal (decoded + %2e), normalizes path via `path.Clean`, preserves trailing slash for later redirect
- [x] `src/server/security_middleware.go`: `allowlistMiddleware` — parses `cfg.Web.Security.Allowlist` (IPs expanded to /32 or /128, CIDRs accepted), sets `ctxKeyAllowlisted` in request context
- [x] `src/server/security_middleware.go`: `blocklistMiddleware` — loads all `*.txt` files from `{data_dir}/security/blocklists/`, rejects matched IPs with 403; skips when `isAllowlisted(ctx)` is true
- [x] `src/server/ratelimit.go`: `rateLimitMiddleware` checks `isAllowlisted(ctx)` and skips rate limiting for allowlisted IPs
- [x] `src/server/server.go`: geoip config wired with `cfg.Web.Security.Allowlist` so GeoIP also bypasses country-blocking for allowlisted IPs

### Middleware execution order — FIXED
- Old order: RealIP → Logger → Recoverer → CleanPath → Compress → SecurityHeaders → SecFetch → CORS → noTrailingSlash → GeoIP
- New order per PART 5: RealIP → Recoverer → CleanPath → noTrailingSlash(URLNormalize) → pathSecurity → SecurityHeaders → SecFetch → CORS → Allowlist → Blocklist → GeoIP → Logger → countRequests → metricsCollector → Compress

### URL normalization — FIXED
- `noTrailingSlash`: added file-extension exception — paths whose last segment contains `.` are not redirected (e.g. `/static/app.js/` stays as-is per PART 16 spec)

### Config additions
- [x] `config.ServerConfig.DataDir` added — set at startup from `paths.GetDataDir(appName)`
- [x] `config.SecurityConfig.Allowlist []string` added — feeds allowlist middleware and geoip config

## Pass 17: PART 16 PWA — RESOLVED

### Service worker (PART 16 spec) — RESOLVED
- [x] `server.go handleServiceWorker`: version-based cache name (`pastebin-cache-{version}`)
- [x] INSTALL event: pre-caches `/`, `/create`, `/recent`, `/offline`, CSS/JS/icons + `self.skipWaiting()`
- [x] ACTIVATE event: deletes stale `pastebin-cache-*` caches + `self.clients.claim()`
- [x] MESSAGE event: handles `SKIP_WAITING` for instant updates
- [x] FETCH event: skip non-GET + cross-origin; skip `/api/` and `/graphql` (network-only); static assets cache-first; HTML network-first with offline fallback; default network-first with cache fallback
- [x] `Cache-Control: no-cache` on sw.js response so browser always checks for new version

### App update notification — RESOLVED
- [x] `main.js`: SW registration now async with `updatefound` listener → calls `showUpdateBanner()`
- [x] `showUpdateBanner()`: injects fixed-position banner with "Update Now" / dismiss buttons
- [x] `applyUpdate()`: posts `SKIP_WAITING` to waiting SW; `controllerchange` listener reloads page
- [x] Hourly update check via `setInterval(() => registration.update(), 3600000)`

### Offline page — RESOLVED
- [x] `server.go handleOffline()`: serves offline.html template at `/offline`
- [x] `templates/offline.html`: minimal offline fallback page with "Try Again" reload button
- [x] `/offline` route registered in setupRoutes

## Completed (cumulative)

- All 6 non-English locales brought to full key parity with `en.json`
- PART 15 SSL/TLS cert lookup and ACME autocert implementation
- PART 23/24 service.go: systemd unit, launchd label/paths, privilege-drop pattern
- PART 11: app_secrets table + generation, Sec-Fetch middleware, CSRF config structs
- PART 9/12: cache layer (memory/noop/redis), config integration, server init/close
- PART 24: Windows service (winsvc_windows.go / winsvc_other.go)
- PART 9: all 13 error codes in httpErrCode/mapAPIErrorCodeToHTTPStatus/sendAPIError
- PART 16: reservedSlugs guard in handleViewPaste()
- PART 12: response compression + full base URL + trusted proxies + LimitsConfig + Validate()
- PART 13: scheduler/Tor healthz checks + pending restart tracking
- PART 6: debug endpoints gated on IsDebug() + pprof/expvar routes
- PART 25: Makefile lint + help targets
- PART 10: connection pool + query timeouts on all DB methods
- PART 14: API response formatting verified (writeJSON: indent + newline)
- PART 21: VerifyBackup (6 checks) + BackupDaily/BackupHourly with maintenance.Backup + retention policy
- PART 4: OS-specific paths fixed + GetDBPath added
- PART 11: all 10 DB tables present (config, config_meta, rate_limits, audit_log, backups, api_tokens + 3 triggers)
- PART 32 client: User-Agent, cli.yml read/save, SaveIfEmptyOrInvalid, mode detection, auto-update check, binary name display
- PART 16 web frontend: mobile-first CSS (min-width queries), prefers-color-scheme auto theme, PWA icon handlers, service worker fix, content negotiation in handleViewPaste/handleHome/handleRecent
- PART 16 JS: theme load (explicit only), copy-to-clipboard with fallback, submit loading state, fetchAPI RFC 7807 error parsing
- PART 4/5 middleware: PathSecurity, Allowlist, Blocklist middleware added; execution order fixed; noTrailingSlash file-extension exception; DataDir + Allowlist added to config
- PART 16 PWA: service worker rewrite (versioned cache, install/activate/fetch/message events), app update banner, offline.html page
- PART 16 iOS meta tags: manifest link + apple-mobile-web-app meta + apple-touch-icon added to all 12 non-embed templates; icon-180 handler added

## Pass 18: PART 18 Scheduler API + CLI Subcommands — RESOLVED

### Scheduler REST API (PART 18) — RESOLVED
- [x] `src/server/server.go`: `SchedulerAPI` interface — `GetTasks/GetTask/RunNow/EnableTask/DisableTask` — satisfied by `*scheduler.Scheduler`
- [x] `Server.SetSchedulerAPI(api)` injects the scheduler post-construction; called from `main.go` after both server and scheduler are built
- [x] `/api/v1/scheduler` routes mounted in `setupRoutes()`:
  - `GET /api/v1/scheduler` — list all tasks
  - `GET /api/v1/scheduler/{id}` — show one task
  - `POST /api/v1/scheduler/{id}/run` — trigger run
  - `POST /api/v1/scheduler/{id}/enable` — enable task
  - `POST /api/v1/scheduler/{id}/disable` — disable task
  - `GET /api/v1/scheduler/{id}/history` — recent execution history
- [x] All handlers return RFC 7807 error bodies on failure and `{"ok":true,"data":...}` on success per PART 14
- [x] `handleSchedulerHistory` queries `db.ListTaskHistory(id, 20)` — bounded result set
- [x] All handlers guard on `schedulerAPI == nil` returning 503 (service unavailable)

### Scheduler CLI subcommands (PART 18) — RESOLVED
- [x] `src/main.go` parses positional `scheduler <subcommand> [id]`:
  - `scheduler list` — reads `db.ListSchedulerTasks()` directly, tabular print
  - `scheduler show <id>` — reads `db.GetSchedulerTask(id)`
  - `scheduler run <id>` — POSTs to running server's `/api/v1/scheduler/{id}/run`
  - `scheduler enable <id>` — POSTs to `/api/v1/scheduler/{id}/enable`
  - `scheduler disable <id>` — POSTs to `/api/v1/scheduler/{id}/disable`
  - `scheduler history <id>` — reads `db.ListTaskHistory(id, 20)` directly
- [x] `--help` output documents all six subcommands (lines 1062–1067 of `src/main.go`)
- [x] `database.DB.ListTaskHistory(taskID, limit)` added to interface + `SQLiteDB`

### Other findings this pass
- [x] `src/common/email/email.go`: `go vet` reported `fmt.Sprintf("%s:%d", host, port)` IPv6-unsafe — replaced with `net.JoinHostPort(host, strconv.Itoa(port))` at L82 and L135
- [x] `IDEA.md` Native REST API table updated with the six new `scheduler/*` endpoints
- [x] `go vet ./...` clean; `go test ./...` passes for all packages with tests (`config`, `database`, `handler`, `paths`)
- [x] No new `TODO`/`FIXME`/`HACK` markers in `src/`; no bcrypt; no `strconv.ParseBool`; no plural source dirs; no Dockerfile in root; no `.env` files in repo; LICENSE.md present

### Still deferred (carry-forwards, unchanged from prior passes)
- [ ] CSRF token validation middleware — deferred until session/auth surface exists (config structs in place)
- [ ] Backup checksum self-verification (archive2 differs from archive1) — 6 other verification checks pass
- [ ] Full bubbletea TUI implementation — `runTUI()` is a guidance stub
