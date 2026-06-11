# TODO.AI.md — pastebin

Outstanding items after bootstrap. Bootstrap completed PART 0–6 scaffolding; PART 7–32 application implementation remains. All items below are pending or require verification against the spec.

---

## [ ] Verify binary CLI flags are complete
Read: AI.md PART 7, PART 8

Both `pastebin` (server) and `pastebin-cli` (client) binaries must implement every required CLI flag defined in PART 7 and PART 8. Audit `src/main.go` and `src/client/main.go` against the full flag tables in those parts. No flag may be missing.

---

## [ ] Verify LDFLAGS variables are all present in both binaries
Read: AI.md PART 7

Confirm `Version`, `CommitID`, `BuildDate`, and `OfficialSite` are declared as `var` in `src/main.go` and `src/client/main.go`, and that the Makefile `LDFLAGS` sets all four via `-X`.

---

## [ ] Audit error handling and caching patterns
Read: AI.md PART 9

Verify that all error types, caching patterns, and in-memory cache configuration match the spec. Confirm no direct `fmt.Println`/`log.Fatal` in non-main packages; all errors surface through the error model defined in PART 9.

---

## [ ] Audit database layer
Read: AI.md PART 10

Confirm `src/database/` uses only parameterized queries (no string interpolation in SQL), no `SELECT *`, correct schema definitions, and all migrations are in-order. Verify the paste table schema matches the spec's required columns exactly.

---

## [ ] Audit security and logging layer
Read: AI.md PART 11

Verify: Argon2id used for password hashing, SHA-256 used for token storage (never plaintext), `tok_` prefix on owner tokens, constant-time comparison for token validation, no secrets in logs, structured logging format as specified.

---

## [ ] Verify server configuration (server.yml schema)
Read: AI.md PART 12

Confirm `src/config/` implements the complete `server.yml` schema from PART 12. All optional fields must have sane defaults. `config.ParseBool()` used everywhere (not `strconv.ParseBool`). No `.env` file loading anywhere in the codebase.

---

## [ ] Verify health and versioning endpoints
Read: AI.md PART 13

Confirm `/healthz`, `/readyz`, `/livez`, `/version`, and `/status` endpoints are present and return the correct response shapes as defined in PART 13. Metrics endpoint `/metrics` must be internal-only (not exposed on external port).

---

## [ ] Audit API routes and response format
Read: AI.md PART 14

Verify every route in the spec's route table is implemented: POST `/api/paste`, GET `/api/paste/{id}`, DELETE `/api/paste/{id}`, GET `/api/pastes`, GET `/raw/{id}`, GET `/p/{id}`, and all compat routes. Confirm JSON response envelope matches spec (`data`, `error`, `meta` fields). Confirm RFC 7807 error format used.

---

## [ ] Verify SSL/TLS and Let's Encrypt implementation
Read: AI.md PART 15

Confirm `src/ssl/` handles auto-cert via `golang.org/x/crypto/acme/autocert`, manual cert loading, and self-signed fallback as specified. Verify `--ssl-*` CLI flags all present.

---

## [ ] Audit web frontend templates and CSS
Read: AI.md PART 16

Verify: no client-side rendering (all templates rendered server-side via Go `html/template`), Chroma syntax highlighting applied server-side, CSS uses only custom properties (no hardcoded colors), mobile-first responsive layout, dark mode default. Owner token stored in `localStorage` per spec. Burn-after / visibility / expiry selectors present in create form.

---

## [ ] Verify email and notification implementation
Read: AI.md PART 17

Confirm `src/common/email/` implements all required email types from PART 17. All email subjects and bodies use i18n keys — no hardcoded English strings. SMTP configuration loaded from `server.yml`.

---

## [ ] Verify built-in scheduler tasks
Read: AI.md PART 18

Confirm `src/scheduler/` implements all required tasks: paste expiry cleanup, burn-after cleanup, GeoIP database refresh, metrics rollup, and any other tasks listed in PART 18. No external cron dependency — all scheduling is in-process.

---

## [ ] Audit GeoIP implementation
Read: AI.md PART 19

Confirm `src/geoip/` uses `github.com/oschwald/maxminddb-golang` (NOT `geoip2-golang`). Auto-download of GeoLite2 database. Country-based blocklist enforcement on all API and web endpoints. GeoIP disabled gracefully when database unavailable.

---

## [ ] Verify Prometheus metrics
Read: AI.md PART 20

Confirm `src/metrics/` exposes all required metrics from PART 20 at `/metrics`. Verify the endpoint is bound to internal port only (not exposed externally). Confirm metric names match the spec exactly.

---

## [ ] Verify backup and restore implementation
Read: AI.md PART 21

Confirm `--backup` and `--restore` CLI flags are present and functional. Backup produces a portable archive as specified. Restore validates and applies the archive.

---

## [ ] Verify update command implementation
Read: AI.md PART 22

Confirm `--update` / `--check-update` flags are present. Update mechanism checks the official site (if `site.txt` exists) and the GitHub release API. Self-update writes atomically (write-to-temp, rename).

---

## [ ] Verify privilege escalation and service lifecycle
Read: AI.md PART 23, PART 24

Confirm `--service start|stop|restart|reload|--install|--uninstall|--disable` all work correctly. Systemd unit at `/etc/systemd/system/pastebin.service` with `Type=simple`, `Restart=always`, `RestartSec=5`. Binary detects effective UID and prompts for sudo when needed for `--install`/`--uninstall`.

---

## [ ] Verify Tor hidden service
Read: AI.md PART 31

Confirm `src/tor/` uses `github.com/cretz/bine` (not the C `tor` library) for hidden service management. Tor binary auto-detected. HiddenServiceVersion 3. SafeLogging enabled. No default Tor ports (9050/9051) used. Hidden service auto-enabled whenever Tor binary is found.

---

## [ ] Verify client binary completeness
Read: AI.md PART 32

Confirm `src/client/` implements all required commands: `paste create`, `paste get`, `paste delete`, `paste list`, `paste raw`, and any other commands from PART 32. TUI mode where specified. `--lang` flag present. `Accept-Language` header sent on all API requests.

---

## [ ] Add `pt.json` or confirm Portuguese is not required
Read: AI.md PART 30

The project had `pt.json` (Portuguese) but PART 30 specifies 7 required locales: `en, es, fr, de, zh, ar, ja`. Portuguese is not in the required list. `pt.json` has been replaced by `ar.json` during bootstrap. Confirm this is correct and remove `pt.json` if it still exists.

---

## [ ] Remove pt.json locale file
Read: AI.md PART 30

`pt.json` is not a required locale per PART 30. It should be removed, or kept as an optional extra locale with `pt` added to `supportedLangs`. Decision: remove it to match the spec exactly, OR keep it (extra locale, not required). Confirm with project owner.

---

## [ ] Verify GraphQL implementation
Read: AI.md PART 14

Confirm `src/graphql/` implements the schema from PART 14. All type descriptions use i18n keys. GraphQL playground accessible at `/graphql` (development mode only).

---

## [ ] Verify Swagger/OpenAPI implementation
Read: AI.md PART 14

Confirm `src/swagger/` serves the Swagger UI at `/api/docs` and the OpenAPI spec at `/api/openapi.json`. All endpoint descriptions use i18n keys.

---

## [ ] Run integration tests
Read: AI.md PART 28

`./tests/run_tests.sh` — run once code implementation is verified complete. These tests require Docker or Incus. Shell-based; do not run on host without container.

---

## [ ] Verify 60% coverage threshold enforced
Read: AI.md PART 28

`make test` enforces ≥60% overall coverage and 100% for `src/server/`. Run coverage report and confirm both gates pass before any release commit.

---

## [ ] Create site.txt when official deployment URL is known
Read: AI.md PART 25, PART 6

`site.txt` is optional but used by Makefile and update command. Create once the official deployment URL is known. Never guess — only create when the URL is confirmed.

---
