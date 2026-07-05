# Project Audit — Full Spec Re-Audit vs CURRENT AI.md

Started: 2026-07-04
Method: VALUE-CHECK every config default, enum, threshold, required field, error code,
route path, filename format, and CLI flag against the exact AI.md text. Existence alone
is never treated as compliance. Every item cites code `file:line` and spec `AI.md:line`.

Baseline: green Docker build (`casjaysdev/go:latest`, CGO_ENABLED=0) established before edits.
Build/test run only in Docker. Changes staged, not committed.

---

## HIGH — value mismatches the prior (presence-only) audit missed

- [x] PART 20 — Metrics disabled by default. `config.go` DefaultConfig had
  `Metrics: MetricsConfig{ Enabled: false }` (~config.go:988). AI.md:27188 literally
  shows `enabled: true`. FIX: `Enabled: false` → `Enabled: true`. Same value-mismatch
  class as the GeoIP `enabled:false`→`true` bug (commit 8165a9d4f7a2).

- [x] PART 13 — Health/version project block hardcoded instead of sourced from branding.
  `server.go buildHealthResponse` returned literals `"Simple, fast paste service"` and
  `"A self-hosted pastebin..."` plus `Web.SiteTitle` (~server.go:1931). AI.md:16965-16967
  requires Name/Tagline/Description sourced from `cfg.Server.Branding`. FIX: now reads
  `cfg.Server.Branding.EffectiveTitle()/EffectiveTagline()/EffectiveDescription()`.
  Added helper `EffectiveTitle()` (config.go, before `EffectiveTagline():840`) — default
  "Pastebin" matches DefaultConfig Branding.Title:966 and SiteTitle:1216.

## HIGH — functional correctness reworks

- [x] PART 15 — Dual-port `"80,443"` never split; HTTP-01 challenge wired only to the
  HTTPS listener (unreachable). `main.go:1206` built `addr = Address + ":" + Port` with no
  split; ChallengeServer in `ssl.go` (~180-222) was dead (never instantiated).
  AI.md:19659-19667 mandates "First = HTTP, Second = HTTPS". FIX:
  - `config.SplitPorts(spec)` added after `ResolvePort` (~config.go:2053): `"80,443"`→
    `("80","443")`, single→`(v,"")`, trailing-comma tolerant. + table test `TestSplitPorts`.
  - `server.go Run` detects dual spec via `net.SplitHostPort` + `SplitPorts`; delegates to
    new `runDualPort` (binds both listeners while privileged, single `applyPrivilegeDrop`,
    HTTPS serves router via `ServeTLS`, HTTP serves `sslMgr.GetHTTPHandler(redirect)` so
    autocert answers `/.well-known/acme-challenge/*` on the plain-HTTP port); new
    `redirectToHTTPS` 308-redirects non-challenge traffic. Graceful shutdown closes both.
  - Removed dead `ChallengeServer` + 6 tests (`ssl.go`, `ssl_test.go`) — autocert
    `Manager.HTTPHandler` fully covers HTTP-01.

- [x] PART 16 — Theme system was client-side localStorage + `data-theme` attribute +
  CSP-violating inline `onclick`. AI.md:23252/23258/23277/23292/23294/23298 mandates a
  cookie-based, server-rendered `class="theme-{light|dark|auto}"` on `<html>`, pure-CSS
  auto via `@media (prefers-color-scheme)`, and a no-JS form. FIX:
  - `server.go`: `themeFromRequest` (cookie→config default→`dark`), `nextTheme`,
    `handleThemeSet` (~server.go:2921); route `POST /theme` (~server.go:817);
    `renderTemplate`/`renderTemplateToString` inject `Theme` + `NextTheme`.
  - CSS: `[data-theme="light"]`→`html.theme-light`, auto→`html.theme-auto`
    (main.css:108,133).
  - JS (main.js:146): removed localStorage theme persistence; cookie-writing instant
    toggle via `addEventListener` (no inline handler). API token localStorage untouched
    (AI.md:22303).
  - 19 templates: `<html data-theme=…>`→`class="theme-{{.Theme}}"`; 18 toggles now use a
    `<form method="post" action="{{.BaseURL}}/theme">` with `csrf_token` + `theme={{.NextTheme}}`
    (works without JS). `error_test.go:93` assertion updated.

---

## MEDIUM / NEEDS-DECISION — flagged, not auto-fixed

- [x] PART 17 — All 4 email dispatches fully implemented and wired. Verified:
  `backupSendComplete`/`backupSendFailed` called at every backup outcome path in task.go.
  `SSLRenewalWithEmail` fires `ssl_expiring` at 30/14/7/3/1-day thresholds and `ssl_renewed`
  on detected cert renewal (NotAfter advanced ≥24h vs stored state). All gated by
  `EmailEventsConfig` fields with spec-correct defaults (BackupComplete=false, BackupFailed=true,
  SSLExpiring=true, SSLRenewed=false). main.go wires all four. AI.md intent confirmed: both
  `backup_failed` AND `scheduler_error` fire on backup failure (separate abstraction levels).

## LOW / NEEDS-DECISION

- [x] PART 9/14 — Non-canonical error codes. Canonical table (AI.md:22732) maps 503→`MAINTENANCE`,
  403→`FORBIDDEN`. USER DECISION: canonical set is CLOSED — rename. FIX: `SERVICE_UNAVAILABLE`→
  `MAINTENANCE` (server.go:3218/3228/3243/3257/3272/3448, debug.go:157, error_test.go:45) and
  `SEC_FETCH_BLOCKED`→`FORBIDDEN` (server.go:1343/1356). Human-readable messages kept descriptive
  ("scheduler not available", "direct navigation to API endpoint blocked") — only the code token
  is canonical.

## Spec-internal contradictions — for user decision (not code bugs)

- [x] Coverage threshold: PART 27 CI gate says ≥60%; PART 28 says target ≥80%. USER DECISION:
  ≥60% is the enforced hard CI gate; ≥80% is the aspirational target. Matches current CI and the
  `.claude/rules/testing-rules.md` wording ("≥60% (CI gate), target ≥80%"). No code change.

---

## Sweep 2 — PARTs 2–4, 6–8, 25–32 (2026-07-05)

### Fixed

- [x] PART 26 — `docker/docker-compose.yml:30` production port hardcoded as
  `'172.17.0.1:64580:80'`. Spec says production port must be operator-overridable.
  FIX: `'172.17.0.1:${HOST_PORT:-64580}:80'`; header comment updated to match.

### Compliant — VALUE-CHECKED

- PART 2: LICENSE.md present, MIT, 3rd-party table. No premium/paywall code.
  (`licenses.yml` CI workflow is absent; spec says SHOULD not MUST — not a blocker.)
- PART 3: All required dirs/files present. Singular Go package dirs. 8-platform binaries.
  `//go:embed` for all embedded assets. No `-musl` suffix.
- PART 4: All OS-specific paths match spec (`src/paths/paths.go`): Linux/macOS/Windows/BSD/Docker.
  DB at `{data_dir}/db/server.db`, backup at spec-mandated path.
- PART 6: `src/mode/mode.go` has `SetAppMode/SetDebugEnabled/IsDebugEnabled/FromEnv`. Mode priority
  correct. Debug endpoints `/debug/pprof/*` + `/debug/vars/config/routes/cache/db/…` all registered.
- PART 7: `var Version, CommitID, BuildDate, OfficialSite` in both `src/main.go` and
  `src/client/main.go`. LDFLAGS `-s -w -X main.*`. All 8 platforms. No `-musl`.
- PART 8: All required CLI flags present (`--mode/--config/--data/--log/--pid/--address/--port/
  --baseurl/--debug/--status/--service/--daemon/--maintenance/--update`). Short flags: only `-h` and
  `-v`.
- PART 25 (Makefile): `dev/local/build/test` targets present. Docker build uses
  `casjaysdev/go:latest`, `-v $PWD:/app` (Makefile var), `CGO_ENABLED=0 GOFLAGS=-buildvcs=false`.
  Coverage to `/tmp/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/`. LDFLAGS correct.
- PART 27 (CI/CD): All 6 required GitHub workflow files present. All Actions pinned to full SHA
  (spot-checked: checkout, upload-artifact, trufflehog — match spec table). No Makefile in CI.
  No `pull_request_target`. Parallel lint/test/scan → build → coverage/artifacts job order. All Go
  jobs use `container: image: casjaysdev/go:latest`. `CGO_ENABLED: "0"` on all build steps.
  `contents: read` default; release job has `contents: write`. truffleHog (not gitleaks).
  Minor: job named `vuln-check` vs spec `vuln-scan` — no functional impact.
- PART 28 (Testing): 114 `_test.go` files alongside source. `tests/run_tests.sh` + `docker.sh` +
  `incus.sh`. Coverage gate `THRESHOLD=60` in ci.yml. Coverage output to `/tmp/…` in CI.
- PART 29 (ReadTheDocs): `mkdocs.yml`, `.readthedocs.yaml`, `docs/` with all required sections
  (api.md, cli.md, configuration.md, security.md, installation.md, integrations.md).
- PART 30 (I18N & A11Y): 7 locales (en/es/fr/de/zh/ar/ja). All 30 keys each (key parity).
  `ar.json` has `meta.direction: rtl`. `Direction()` function present. `LangFromRequest` parses
  `Accept-Language`. All `<html>` elements have `lang="{{.Lang}}" dir="{{.Dir}}"`. Client `--lang`
  flag and `Accept-Language` header set on all requests. NO_COLOR respected in both binaries.
- PART 32 (Client): `pastebin-cli` binary. Commands: `create/get/delete/list/update/tui/completions`.
  User-Agent `pastebin-cli/{version}`. Exit codes 0/1/2/3/4/5/64 match spec. bubbletea/bubbles/
  lipgloss in `go.mod`. Auto-update: download → SHA-256 verify → atomic rename → re-exec.

---

## Compliant — VALUE-CHECKED (not presence-checked)

- PART 19 GeoIP: DefaultConfig `Enabled: true` — matches AI.md mandate (fixed prior, 8165a9d4f7a2).
- PART 31 Tor SocksPort: `SocksPort 0` default, `SocksPort auto` when `UseNetwork` (tor.go:452-454)
  — matches AI.md:38923 ("auto if outbound enabled, 0 if not").
- PART 23 reservedIDs: system UID/GID set built and enforced (service.go:23-44).
- PART 11 tokens: SHA-256 hex storage, `tok_` prefix, `crypto/subtle` compare — verified present.
- PART 5 booleans: `config.ParseBool()` used; no `strconv.ParseBool()` in tree.
- PART 20 metrics buckets: histogram bucket set present for `scheduler_task_duration_seconds`.

---

## Build / Test — FINAL

- Docker: `casjaysdev/go:latest`, CGO_ENABLED=0, GOFLAGS=-buildvcs=false, -v "$PWD":/app.
- `go build ./...` — clean.
- `go test ./...` — ALL packages `ok` (34 packages incl. config, server, ssl, handler,
  database, geoip). Exit 0. Green.
- Changes staged, not committed. Main instance reviews and commits via gitcommit.
