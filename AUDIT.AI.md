# Project Audit

Started: 2026-05-24
Last reconciled: 2026-05-28 (config hot-reload + route audit pass)
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

## Completed (this pass)

- All 6 non-English locales brought to full key parity with `en.json`; build verified inside `golang:alpine` with `CGO_ENABLED=0`.
