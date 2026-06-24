# TODO.AI.md — pastebin

Outstanding items after bootstrap. Bootstrap completed PART 0–6 scaffolding, created all `.claude/rules/` files, and created `CLAUDE.md`. PART 7–32 application implementation remains. All items below are pending or require verification against the spec.

---

## [x] Verify binary CLI flags are complete
Read: AI.md PART 7, PART 8

Audited 2026-06-23. All PART 8 server flags present in `src/main.go`; `--lang` present in `src/client/main.go`. Fixed: `-h`/`-v` short flags fell through silently in `src/main.go` (now handled). Fixed: server internal HTTP User-Agent was `pastebin-cli/` -> `pastebin/`.

---

## [x] Verify LDFLAGS variables are all present in both binaries
Read: AI.md PART 7

Verified 2026-06-23. `Version`, `CommitID`, `BuildDate`, `OfficialSite` declared as `var` in both `src/main.go` (lines 36-39) and `src/client/main.go` (lines 40-45). Makefile sets all four via `-X`. CGO_ENABLED=0 in build env.

---

## [x] Audit error handling and caching patterns
Read: AI.md PART 9

Verified 2026-06-23. No `log.Fatal` in non-main packages; `os.Exit` occurrences are legitimate fork/relaunch points. Cache (`src/cache/cache.go`) supports memory/valkey/redis/none with colon-separated lowercase `pastebin:` key prefix and TTL fallback per spec. No fixes required.

---

## [x] Audit database layer
Read: AI.md PART 10

Verified 2026-06-23. All queries parameterized with `?`; zero `SELECT *` and zero `fmt.Sprintf`-into-SQL across `src/`. Idempotent schema (`CREATE TABLE IF NOT EXISTS`, guarded `ALTER TABLE`). Connection pool and per-query context timeouts match PART 10. No fixes required.

---

## [x] Audit security and logging layer
Read: AI.md PART 11

Verified 2026-06-23. No bcrypt/MD5/SHA-1 anywhere. Tokens stored as SHA-256 hex only (raw never persisted). `tok_` prefix on owner + operator tokens. Constant-time comparison (`crypto/subtle`) for all token/hash checks. No raw token values in logs. Fixed: metrics `/metrics` bearer token used plain `!=` -> now `subtle.ConstantTimeCompare` (`src/metrics/metrics.go`).

---

## [x] Verify server configuration (server.yml schema)
Read: AI.md PART 12

Verified 2026-06-23. `Validate()` warns-and-defaults (never fails startup). `config.ParseBool()` used; zero `strconv.ParseBool`. No `.env` file loading (env vars read individually as overrides, spec-permitted). `EncryptionKey` is AES-256 auto-generated via `crypto/rand`. Schema for limits/proxies/rate-limit/TLS/scheduler/headers present with defaults. Forward-looking schema (webhooks, contact roles) grows as later PARTs are built — not a current violation.

---

## [x] Verify health and versioning endpoints
Read: AI.md PART 13

Verified 2026-06-23. `/server/healthz`, optional `/healthz` (gated), `/api/v1/server/healthz`, `/api/healthz`, `/api/v1/server/version` wired with canonical field order. `/metrics` is internal-only per PART 13/20 model (firewall/IP restriction + optional bearer token on the same port — spec does not mandate a separate listener). No fixes required.

---

## [x] Audit API routes and response format
Read: AI.md PART 14

Verified 2026-06-23. Success envelope `{ok:true,data}` and error envelope `{ok:false,error,message}` correct; canonical error codes + HTTP-status mapping correct. `/api/swagger` and `/api/graphql` aliases serve handlers directly (no redirect). Fixed: GraphiQL UI posted to removed `/graphql` path -> `/api/graphql` (`src/graphql/graphql.go`). Fixed: OpenAPI spec omitted GraphQL endpoint -> added (`src/swagger/annotations.go`).

---

## [x] Verify SSL/TLS and Let's Encrypt implementation
Read: AI.md PART 15

Verified 2026-06-23. `src/ssl/ssl.go` uses `autocert` for auto-cert, 4-priority manual cert lookup, staging support, HTTP-01 challenge server. Self-signed fallback is spec-required only for overlay networks (.onion/.i2p); clearnet correctly errors when no cert and LE disabled. NOTE: AI.md does NOT define `--ssl-*` CLI flags — TLS is config-driven via `server.tls.*` keys and the dual-port rule (443=HTTPS, `--port 80,443`=dual). No flag work warranted; spec governs.

---

## [x] Audit web frontend templates and CSS
Read: AI.md PART 16

Verified 2026-06-23. Server-side `html/template` only; server-side Chroma highlighting (`HighlightedContent()`); owner token in `localStorage`; create form has expiry/visibility/burn-after; progressive enhancement holds (`POST /create` works without JS); long strings use `word-break`. Fixed: hardcoded `lang="en"` in all 13 templates -> `lang="{{.Lang}}" dir="{{.Dir}}"` (i18n + Arabic RTL). Fixed: `i18n.Direction()` added; `server.go` injects `.Dir`. Fixed: `healthz.html` hardcoded colors -> CSS custom properties.

---

## [x] Verify email and notification implementation
Read: AI.md PART 17

Verified 2026-06-23. All 7 required templates present (security_alert, backup_complete, backup_failed, ssl_expiring, ssl_renewed, scheduler_error, test) with correct subjects. SMTP loaded from `server.yml`; all 7 `SMTP_*` env overrides wired; silent disable when unconfigured. CORRECTION: this item's "use i18n keys" requirement contradicts AI.md PART 17, which mandates file-based templates with embedded English default subjects + `{variable}` substitution. AI.md (read-only source of truth) governs — templates correctly use embedded English, NOT i18n keys. No fix.

---

## [x] Verify built-in scheduler tasks
Read: AI.md PART 18

Verified 2026-06-23. Built-in `time.Ticker` engine + custom cron parser + DB-backed state + catch-up window. No external cron. All 10 required tasks registered (ssl_renewal, geoip_update, blocklist_update, cve_update, token_cleanup, log_rotation, backup_daily, backup_hourly, healthcheck_self, tor_health) plus `expire-pastes` (expiry + burn-after). No fixes required.

---

## [x] Audit GeoIP implementation
Read: AI.md PART 19

Verified 2026-06-23. Uses `github.com/oschwald/maxminddb-golang` (NOT geoip2-golang). Auto-download from jsDelivr CDN with atomic rename, fail-open, RFC1918/allowlist bypass. Middleware applied router-wide (`src/server/server.go:476`) covering web + API. Graceful disable when DB unavailable. No fixes required.

---

## [x] Verify Prometheus metrics
Read: AI.md PART 20

Verified 2026-06-23. `pastebin_` namespace, all required metric families present, names match spec. `/metrics` access model (firewall/IP + optional bearer token) matches PART 20's internal-only definition. Fixed: `scheduler_task_duration_seconds` histogram buckets corrected to spec values `{0.1,0.5,1,5,10,30,60,300,600}`. Fixed: bearer token now uses `subtle.ConstantTimeCompare`.

---

## [x] Verify backup and restore implementation
Read: AI.md PART 21

Verified 2026-06-23. `Backup()` produces `{name}_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]` with sha256 manifest, AES-256-GCM + Argon2id encryption when password set, 0o600 perms. Fixed: `Restore()` now calls `VerifyBackup()` before extracting and aborts on failure (spec: only restore if all verification passes).

---

## [x] Verify update command implementation
Read: AI.md PART 22

Verified 2026-06-23. `CheckForUpdate` (GitHub API, 404=no update), `DoUpdate` (download to temp in binary dir, sha256 verify, atomic `os.Rename`), `RestartSelf` via `syscall.Exec`. `--update`/`--check`/`--branch` wired. CORRECTION: PART 22 specifies only the GitHub release API for self-update; `site.txt` is for CLI `--server` resolution, NOT self-update. No site.txt logic in updater — spec governs.

---

## [x] Verify privilege escalation and service lifecycle
Read: AI.md PART 23, PART 24

Verified 2026-06-23. `installSystemd()` generates unit with `Type=simple`, `RestartSec=5`, hardening directives. Start/Stop/Restart/Reload/Disable correct. CORRECTION: PART 24 specifies `Restart=on-failure` (not `Restart=always` as this item stated) — spec governs, kept on-failure. Fixed: `Install()` gained privilege guard + post-install start; `Uninstall()` gained privilege guard + `[y/N]` destructive confirmation + data/user purge; `isPrivileged()` helpers added for unix/windows.

---

## [x] Verify Tor hidden service
Read: AI.md PART 31

Verified 2026-06-23. `src/tor/tor.go` uses `github.com/cretz/bine` (NOT C tor); CGO_ENABLED=0 preserved (hand-rolled PATH lookup). Tor binary auto-detected (config -> PATH -> common locations). v3 hidden service (ed25519, persistent key). SafeLogging default on. No default ports (ControlPort auto, SocksPort 0/auto, ORPort/DirPort 0; 9050/9051 explicitly forbidden). Auto-enabled when Tor found; non-fatal when absent. No fixes required.

---

## [~] Verify client binary completeness
Read: AI.md PART 32

Partially verified 2026-06-23. Commands create/get/delete/list present (`get` serves raw via `/raw/{id}`); `--lang` flag present; LDFLAGS vars declared as `var`. Fixed: `Accept-Language` added to DELETE/update/checkCLIUpdate requests (now all 5 request sites send it). Fixed: removed stale `AUDIT.AI.md` reference and "not yet implemented" stub phrasing in `runTUI`; replaced `log.Fatal("not implemented")` in `cmdUpdate` with manual-download instructions.

STILL OPEN — large interdependent feature builds requiring decisions/new deps (see "Client feature gaps" item below).

---

## [ ] Client feature gaps (PART 32) — require decisions / new dependencies
Read: AI.md PART 32

Flagged during 2026-06-23 audit, NOT implemented (not corrections — net-new feature builds):
1. Full bubbletea TUI — PART 32 marks it NON-NEGOTIABLE but `runTUI` falls back to help; `github.com/charmbracelet/bubbletea` is not in `go.mod`.
2. Interactive setup wizard (`RunSetupWizard`) — currently print-only; depends on (1).
3. Native GUI (GTK/Cocoa/Win32) — absent; spec's GUI is cgo-based, which conflicts with project-wide `CGO_ENABLED=0` unless gated behind a `gui` build tag. NEEDS DESIGN DECISION before implementation.
4. CLI auto-update download/verify/swap (PART 32 steps 3-6) — currently prints manual instructions; tied to PART 22 helpers.

---

## [x] Confirm Portuguese locale decision
Read: AI.md PART 30

Resolved 2026-06-23: no `pt.json` exists. `src/common/i18n/locales/` contains exactly the 7 required locales (en, es, fr, de, zh, ar, ja) and `supportedLangs` in `src/common/i18n/i18n.go` matches them exactly. Nothing to add or remove.

---

## [x] Verify GraphQL implementation
Read: AI.md PART 14

Verified 2026-06-23. Schema implemented; GraphiQL explorer at `/server/docs/graphql`. Fixed: GraphiQL `runQuery()` posted to removed `/graphql` path (404) -> now `/api/graphql` (canonical alias served directly).

---

## [x] Verify Swagger/OpenAPI implementation
Read: AI.md PART 14

Verified 2026-06-23. Swagger UI and OpenAPI spec served (self-contained, no CDN, dark-default themed). Fixed: OpenAPI spec omitted the GraphQL endpoint -> added `POST /api/v1/server/graphql` annotation (Swagger/GraphQL sync per PART 14).

---

## [ ] Run integration tests
Read: AI.md PART 28

`./tests/run_tests.sh` — run once code implementation is verified complete. These tests require Docker or Incus. Shell-based; do not run on host without container.

---

## [x] Verify 60% coverage threshold enforced
Read: AI.md PART 28

`make test` enforces ≥80% overall coverage. Gate passes at 80.0% as of 2026-06-23. Confirmed via `make test` after clearing test cache.

---

## [ ] Create site.txt when official deployment URL is known
Read: AI.md PART 25, PART 6

`site.txt` is optional but used by Makefile and update command. Create once the official deployment URL is known. Never guess — only create when the URL is confirmed.

---

## [x] Create GitHub Actions CI workflow (ci.yml)
Read: AI.md PART 27

Completed 2026-06-23. ci.yml rewritten to match spec exactly: checkout v7.0.0 SHA,
build needs: [lint, test], vuln-scan renamed to vuln-check, if:!=schedule guards on
non-security jobs, PROJECTNAME env var, secret-scan and workflow-policy included.

---

## [x] Create GitHub Actions release.yml and optional workflows
Read: AI.md PART 27

Completed 2026-06-23. All workflows updated and created:
- `.github/workflows/release.yml` — checkout v7 SHA, -trimpath, softprops SHA 718ea10b
- `.github/workflows/security.yml` — checkout v7 SHA, vuln-scan → vuln-check
- `.gitea/workflows/release.yml` — Gitea stable release (GITEA_ENV/GITEA_REF_NAME)
- `.gitea/workflows/beta.yml` — Gitea beta branch release
- `.gitea/workflows/daily.yml` — Gitea daily build with API delete of previous daily
- `.gitea/workflows/docker.yml` — Gitea Docker image build (standard + aio)

---
