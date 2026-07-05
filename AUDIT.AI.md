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

- [ ] PART 17 — 4 email templates never dispatched. `backup_complete`, `backup_failed`,
  `ssl_expiring`, `ssl_renewed` exist in `src/common/email/templates/` but no code sends
  them (only `scheduler_error` at main.go:1391 and `security_alert` at maintenance.go:176).
  AI.md:26204-26207 specifies `ssl_expiring` at 30/14/7/3/1 days before expiry and
  AI.md:26591-26594 defines per-event toggles under `email.events`. Net-new surface (SSL
  expiry-threshold monitor + `Email.Events` config struct + backup success/failure hooks)
  plus a behavioral question — does a backup failure send BOTH `scheduler_error` AND
  `backup_failed`, or only the latter? Not guessed. NEEDS USER DECISION on scope + semantics.

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
