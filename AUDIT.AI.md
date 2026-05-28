# Project Audit

Started: 2026-05-24
Last reconciled: 2026-05-28 (full-project audit pass)
Scope: ALL PARTs of AI.md EXCEPT PART 27/CI workflows under `.github/workflows/` (out of scope).

## Pass 1: Security

No new violations. Argon2id N/A (no auth surface yet). SHA-256 used for delete-token storage. `crypto/rand` used for paste IDs and delete tokens. Constant-time-compare claim in IDEA.md vs. actual implementation still tracked under "Document threat model" (open).

## Pass 2: Code Quality

- [x] `src/main.go`: `--shell completions|init` IMPLEMENTED via `src/shell` package — invokes `shell.PrintHelp/PrintCompletions/PrintInit` for the server, and `shell.PrintClientCompletions` for `pastebin-cli` in `src/client/main.go`.
- [ ] PID stale-detect/cleanup logic in `src/pid/` still needs full audit against PART 8 spec — open.

## Pass 3: Logic and Correctness

All previously listed `src/paths/paths.go` items remain fixed.

## Pass 4: Documentation Completeness

- [ ] LICENSE.md third-party attributions still incomplete — open.
- [ ] `man/pastebin.1` page not present — open.
- [ ] README.md "Environment Variables" section still missing — open.

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

### Client (PART 32)
- [ ] `src/client/main.go` hardcoded `defaultServer` still needs IDEA.md reconciliation — open.

### Live config reload (PART 5/PART 8)
- [ ] `fsnotify` hot-reload not wired up — open.

## Pass 6: Code Flow Trace

- [ ] `src/server/server.go`: native route shape mismatch with IDEA.md (singular vs. plural noun for CRUD) — open.
- [ ] lenpaste / pastebin.com compat dispatch paths in `src/server/server.go` need verification — open.
- [ ] Environment-variable audit in README.md — open.

## Completed (this pass)

- All 6 non-English locales brought to full key parity with `en.json`; build verified inside `golang:alpine` with `CGO_ENABLED=0`.
