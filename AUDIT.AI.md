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
- [ ] **PART 16 — `/server/about` hardcodes description + features** (`templates/about.html:41-69`,
  `server.go:2499/3147`). Must source Tagline/Description/Features/Links from IDEA.md/config
  Branding. Content is real (not placeholder); the sourcing mechanism is the violation.
- [ ] **PART 18 — Graceful shutdown does not drain running tasks** (`src/scheduler/scheduler.go:185`).
  `Stop()` returns immediately; task goroutines detached. Add `sync.WaitGroup` + 30s bounded
  drain; mark interrupted tasks for retry (AI.md:26060, 27084).
- [-] **PART 27 — Missing `.gitea/workflows/ci.yml`** (dir has beta/daily/docker/release only).
  NEEDS DECISION: is this project GitHub-only, or multi-provider? (remote is github/apimgr/pastebin)
- [-] **PART 27 — Missing `.gitlab-ci.yml`** at root. Same multi-provider decision.
- [x] **PART 27 — `release.yml` top-level `permissions: contents: write`** (`.github/workflows/release.yml:13`).
  Release job already scopes its own write perms (line 103). FIXED: top-level baseline lowered to
  `contents: read`; the release job retains its own `contents: write`. `act --list` passes.

## LOW

- [ ] **PART 18 — `catch_up_window` not wired** (config lacks field; `SetCatchUpWindow` never called).
  Hardcoded 1h. Add `CatchUpWindow` yaml field → duration → `sched.SetCatchUpWindow`.
- [ ] **PART 18 — Scheduler `timezone` not wired; default not America/New_York**
  (`scheduler.go:84` uses `time.Local`). Add `Timezone` yaml field (default America/New_York),
  `time.LoadLocation`, `sched.SetLocation`.
- [ ] **PART 26/27 — `OFFICIAL_SITE` build-arg not passed** to docker.yml / Makefile docker target /
  ci.yml artifact builds / Dockerfile.dev. `main.OfficialSite` resolves empty in images. Release
  path is correct. Plumb the arg for consistency.
- [ ] **PART 32 — CLI update temp path** (`src/client/main.go:939` uses `exe + ".new"`). Spec
  (AI.md:40157) wants `${TMPDIR:-/tmp}/apimgr/pastebin-XXXXXX/cli.update.tmp`. Note: PART 22
  (server) contradicts this ("temp in binary dir"); PART 32 is client authority.
- [ ] **PART 27 — Empty `.forgejo/workflows/`**. Populate or remove (AI.md:32419 allows reusing
  Gitea workflows). Tied to the multi-provider decision.
- [-] **PART 14 — Maintenance error code `MAINTENANCE_MODE` vs canonical `MAINTENANCE`**
  (`src/server/maintenance.go`). GENUINE SPEC CONTRADICTION: PART 9 table (AI.md:12761) says
  `MAINTENANCE`; AI.md:7315 AND project config-rules.md say `MAINTENANCE_MODE`. Do NOT change
  without spec-owner decision — condensed rule confirms current code.
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
PART 16 (except about sourcing). PART 17, 19, 20, 21, 22 — clean. PART 18 (except items above).
PART 23, 24, 25, 26 — clean. PART 28, 29, 30, 31 — clean (i18n parity PASS, docs present,
NO_COLOR + --color, a11y). PART 32 (except temp path). PART 33 — reference, conforms.

## Condensed-rule drift (NOT code violations; AI.md is source of truth)
- `.claude/rules/features-rules.md` SMTP env var names differ from AI.md (code follows AI.md).
- `.claude/rules/backend-rules.md`: User-Agent `pastebin/` vs AI.md `pastebin-cli/` (code correct);
  lists `tui` command that AI.md forbids (code correct).
</content>
</invoke>
