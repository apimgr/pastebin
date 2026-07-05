# Spec-Compliance Audit (AI.md PART 0–33)

Full read-only sweep across all 33 PARTs. Each item verified in-code with file:line + spec
reference. Fix items are ordered by severity; compliant PARTs are listed at the bottom.

Scope confirmed: AI.md has 33 numbered PARTs (no PART 34). `tui` CLI subcommand absence is
spec-correct (AI.md:40304 forbids it). i18n key parity PASSES (526 keys × 7 locales).

## Legend
- Status: `[ ]` open · `[x]` fixed · `[-]` deferred/needs-decision

---

## HIGH

- [x] **PART 12 — `PasteHandler.pasteURL` duplicated the simplified resolver** (`src/handler/paste.go:616`).
  Missing DOMAIN/FQDN fallback + proxy Host/prefix handling; same regression class as the
  already-fixed `server.baseURL`. FIXED: injected `SetBaseURLResolver(s.baseURL)`; handler
  now delegates to the canonical PART 12 chain. Regression test added.
- [x] **PART 12 — `CompatHandler.origin` duplicated the simplified resolver** (`src/handler/compat.go:800`).
  Same defect (raw/termbin URLs). FIXED via the same shared resolver (`c.ph.base(r)`).

## HIGH (cont.)

- [x] **PART 11 — Sec-Fetch `navigate` reject over-broad; blocked GET to `/api/*`** (`src/server/server.go:1346`).
  Code blocked `Sec-Fetch-Mode: navigate` on `/api/*` for ALL methods, so opening
  `/api/v1/server/healthz` in a browser returned `SEC_FETCH_BLOCKED`. AI.md:13931 validates
  `Sec-Fetch-Mode` ONLY on POST/PUT/PATCH/DELETE and EXPLICITLY allows GET/HEAD navigation
  ("opening an API URL in a browser returns the JSON normally"). FIXED: navigate reject now
  gated on state-changing methods only; comment/docstring corrected. Test case updated
  (GET navigate → 200, POST navigate → 403).

## HIGH — audit-error corrections (previously mis-marked "clean")

- [x] **PART 19 — GeoIP disabled by default; violates the `enabled: true` mandate** (`src/config/config.go:998`).
  DefaultConfig set `GeoIP.Enabled: false`. AI.md PART 19 (AI.md:27032 "ALL projects MUST have
  built-in GeoIP support"; config example AI.md:27053 `enabled: true`) requires it enabled by
  default. Live symptom: `/api/v1/server/healthz` reported `"features": {"geoip": false}` because
  `s.geoipDB` is nil whenever `Enabled=false` (`geoip.Open()` always returns non-nil, so nil ⇒
  disabled). AUDIT PREVIOUSLY (wrongly) listed PART 19 as "clean" — only the GeoIP *code* was
  verified, never the config default. FIXED: default now `Enabled: true`; `Open()` fail-open keeps
  startup safe (DBs download on first run, warn-only). Build+test green.

## MEDIUM

- [x] **PART 6 — Missing debug API endpoints** (`src/server/server.go:835`). Spec (AI.md:8703-8761)
  mandated `/debug/config` (sanitized), `/debug/routes`, `/debug/cache`, `/debug/db`,
  `/debug/scheduler` (+ optional `/debug/memory`, `/debug/goroutines`); only `/debug/pprof` +
  `/debug/vars` were registered. FIXED: added `src/server/debug.go` with all seven handlers,
  registered via `registerDebugRoutes` inside the debug gate. `/debug/config` round-trips the
  live config through YAML and recursively redacts every secret key. Tests added.
- [x] **PART 15 — Overlay `.onion`/`.i2p` self-signed fallback missing** (`src/ssl/ssl.go`).
  Clearnet-error half is correct; overlay hosts had no cert path. FIXED: added `src/ssl/selfsigned.go`
  (`isOverlayHost`, `ensureSelfSignedCert`, `generateSelfSigned` — ECDSA P-256, 10-year, 0600, cached
  to `{cert_dir}/local/{fqdn}/`). `GetTLSConfig` gained an overlay-only branch placed AFTER the LE
  check and BEFORE the clearnet error, so clearnet hosts still error and NEVER self-sign. Tests added.
- [x] **PART 16 — `/server/about` hardcodes description + features** (`templates/about.html:41-69`,
  `server.go:2499/3147`). Must source Tagline/Description/Features/Links from IDEA.md/config
  Branding. Content is real (not placeholder); the sourcing mechanism is the violation.
  FIXED: extended `BrandingConfig` with `Features []string` + `Links []BrandingLink`; added
  IDEA.md-sourced `defaultBranding*` consts/vars and `EffectiveTagline/Description/Features/Links`
  accessors (never blank/placeholder); seeded DefaultConfig branding block. Added
  `Server.aboutPageData()` and wired `handleAbout` (text + HTML) to it; `about.html` now renders
  `.Tagline`/`.Description`/`.Features` (range)/`.Links` (range) from config.
- [x] **PART 18 — Graceful shutdown does not drain running tasks** (`src/scheduler/scheduler.go:185`).
  `Stop()` returns immediately; task goroutines detached. Add `sync.WaitGroup` + 30s bounded
  drain; mark interrupted tasks for retry (AI.md:26060, 27084).
  FIXED (commit 477c1f70): added `wg sync.WaitGroup`; every run path (`tick`, `runMissed`,
  `RunNow`) does `wg.Add(1)`/`defer wg.Done()`; `Stop()` closes `stop`, then `wg.Wait()` bounded by
  `shutdownDrainTimeout` (30s); on timeout `markInterrupted()` sets `lastStatus="interrupted"`,
  `nextRun=now`, persists — so the task re-runs within the catch-up window on next start.
- [x] **PART 27 — Missing `.gitea/workflows/ci.yml`** (dir has beta/daily/docker/release only).
  RESOLVED (user: full multi-provider). FIXED: added `.gitea/workflows/ci.yml` ported from the
  GitHub ci.yml (same jobs/stages, `gitea.*` context, `casjaysdev/go:latest` container, SHA-pinned
  actions, truffleHog, 60% coverage gate). YAML valid.
- [x] **PART 27 — Missing `.gitlab-ci.yml`** at root. RESOLVED (full multi-provider). FIXED: added
  root `.gitlab-ci.yml` (lint/test/build stages, `casjaysdev/go:latest`, CGO_ENABLED=0/-buildvcs=false,
  no Makefile, OFFICIALSITE/VERSION resolution). YAML valid.
- [x] **PART 27 — `release.yml` top-level `permissions: contents: write`** (`.github/workflows/release.yml:13`).
  Release job already scopes its own write perms (line 103). FIXED: top-level baseline lowered to
  `contents: read`; the release job retains its own `contents: write`. `act --list` passes.

## LOW

- [x] **PART 18 — `catch_up_window` not wired** (config lacks field; `SetCatchUpWindow` never called).
  Hardcoded 1h. Add `CatchUpWindow` yaml field → duration → `sched.SetCatchUpWindow`.
  FIXED (commit 477c1f70): `config.go:913` adds `catch_up_window` yaml field; `main.go:1065`
  parses the duration and calls `sched.SetCatchUpWindow` (falls back to the 1h default when empty).
- [x] **PART 18 — Scheduler `timezone` not wired; default not America/New_York**
  (`scheduler.go:84` uses `time.Local`). Add `Timezone` yaml field (default America/New_York),
  `time.LoadLocation`, `sched.SetLocation`.
  FIXED (commit 477c1f70): `config.go:909` adds `timezone` yaml field; `main.go:1053` resolves
  config → `TZ` env → `America/New_York` default via `time.LoadLocation` and calls `sched.SetLocation`.
- [x] **PART 26/27 — `OFFICIAL_SITE` build-arg not passed** to docker.yml / Makefile docker target /
  ci.yml artifact builds / Dockerfile.dev. `main.OfficialSite` resolves empty in images. Release
  path is correct. Plumb the arg for consistency.
  FIXED: `Dockerfile.dev` gains `ARG OFFICIAL_SITE` + `-X 'main.OfficialSite=${OFFICIAL_SITE}'`;
  `docker.yml` resolves `OFFICIALSITE` (site.txt → `secrets.OFFICIALSITE`) and passes
  `OFFICIAL_SITE` build-arg; `ci.yml` resolves the same and appends the `main.OfficialSite` ldflag
  to both server and CLI builds. Matches the existing `release.yml` pattern. `act --list` passes.
  Makefile already correct (`OFFICIALSITE` var → ldflag).
- [x] **PART 32 — CLI update temp path** (`src/client/main.go:939` used `exe + ".new"`). Spec
  (AI.md:40157) wants `${TMPDIR:-/tmp}/apimgr/pastebin-XXXXXX/cli.update.tmp`. Note: PART 22
  (server) contradicts this ("temp in binary dir"); PART 32 is client authority.
  FIXED: `downloadAndApplyUpdate` now downloads + SHA-256-verifies into
  `os.TempDir()/apimgr/pastebin-XXXXXX/cli.update.tmp` via new `updateTempDir()`, then
  `os.Rename` to the target with an `errors.Is(err, syscall.EXDEV)` fallback to
  `replaceCrossDevice()` (temp dir is usually a separate filesystem). Temp dir removed on exit.
- [x] **PART 27 — Empty `.forgejo/workflows/`**. Populate or remove (AI.md:32419 allows reusing
  Gitea workflows). RESOLVED (full multi-provider). FIXED: added `.forgejo/workflows/ci.yml` and
  `.forgejo/workflows/release.yml` (Forgejo Actions, reusing the Gitea pattern per AI.md:32419,
  SHA-pinned). YAML valid.
- [x] **PART 14 — Maintenance error code `MAINTENANCE_MODE` vs canonical `MAINTENANCE`**
  (`src/server/maintenance.go`). RESOLVED (user: AI.md is source of truth, not derived files):
  AI.md has TWO canonical error-code tables (PART 9 AI.md:12772, PART 16 AI.md:22848) plus a code
  example (AI.md:12816) all using `MAINTENANCE`; only one prose JSON example (AI.md:7315) said
  `MAINTENANCE_MODE`. `config-rules.md` is a derived summary and does not override AI.md. FIXED:
  `maintenance.go:53` + `maintenance_test.go:78` now use `MAINTENANCE` (matches `paste.go` which
  was already correct).
- [x] **PART 14 — Maintenance error uses compact JSON** (`maintenance.go:54` `json.NewEncoder`)
  vs the house `writeJSON`/`MarshalIndent` 2-space standard. FIXED: routed through `writeJSON`
  (Retry-After / X-Maintenance-* headers still set beforehand); dropped the now-unused
  `encoding/json` import. Error code `MAINTENANCE_MODE` left unchanged (still ambiguous, below).

---

## Compliant PARTs (verified, no action)

PART 0, 1, 2, 3, 4, 5 — clean. PART 6 (except debug endpoints). PART 7, 8, 9, 10, 11 — clean
(CGO off, all SQL parameterized, no SELECT *, SHA-256 tokens, tok_ prefix, constant-time compare,
no bcrypt/MD5/SHA-1, all CLI flags). PART 12 (server.baseURL fixed; handlers fixed above).
PART 13 — clean. PART 14 (except the two maintenance items). PART 15 (except overlay certs).
PART 16 (except about sourcing). PART 17, 20, 21, 22 — clean. PART 19 (except default `enabled`
above — code clean, config default was wrong). PART 18 (except items above).
PART 23, 24, 25, 26 — clean. PART 28, 29, 30, 31 — clean (i18n parity PASS, docs present,
NO_COLOR + --color, a11y). PART 32 (except temp path). PART 33 — reference, conforms.

## Condensed-rule drift (NOT code violations; AI.md is source of truth)
- `.claude/rules/features-rules.md` SMTP env var names differ from AI.md (code follows AI.md).
- `.claude/rules/backend-rules.md`: User-Agent `pastebin/` vs AI.md `pastebin-cli/` (code correct);
  lists `tui` command that AI.md forbids (code correct).
</content>
</invoke>
