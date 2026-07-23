# AI.md Compliance Audit — 2026-07-20

Line-by-line audit of code vs AI.md (source of truth), run as 9 parallel
read-only passes by PART range. Status tracked per item; delete this file
once every item is fixed and committed.

## MAJOR

- [x] **M1** LICENSE.md missing attribution for 4 direct deps (go-crypto,
      bluemonday, go-qrcode, goldmark). AI.md:5497,5629. `LICENSE.md`
- [x] **M2** Server exposes `scheduler`, `--clean-expired`, `--email`
      commands not in PART 8's declared closed command set (AI.md:10154).
      Needs cross-check vs PART 17/18 authorization before removing —
      PART 17-22 audit confirms `scheduler` task plumbing is legitimate
      (task.go, main.go:1343), so this is likely a PART 8 command-list
      documentation gap, not a code violation. `src/main.go`
- [x] **M3** DNS-01 ACME challenge config (`DNSProviderType`,
      `DNSCredentials`) exists but is dead — only HTTP-01/TLS-ALPN-01
      wired via autocert.Manager. AI.md:19376-19413. `src/ssl/ssl.go`
- [x] **M4** `/server/healthz` ignores JSON content negotiation, always
      renders HTML. AI.md:11934. `src/server/server.go:2427-2440,2743-2758`
- [x] **M5** Inline `<script>` blocks in all 19 templates violate "NO
      Inline CSS/JS". AI.md:21422. `src/server/templates/*.html`
- [x] **M6** CSP allows `'unsafe-inline'` for script-src/style-src —
      enabler for M5. `src/server/server.go:1370,1376`
- [x] **M7** `/server/about` missing required Version section.
      AI.md:25734. `src/server/templates/about.html`,
      `src/server/server.go` (aboutPageData)
- [x] **M8** Missing email template `ssl_renewal_failed.txt`.
      AI.md:26508-26532. `src/common/email/templates/`
- [x] **M9** `ssl_expiring` email sent at 30/14 days; spec requires
      those to be log-only (email only at 7/3/1). AI.md:26824-26827.
      `src/task/task.go:79,175-188`
- [x] **M10** `ssl_renewal_failed` email never sent + no
      `NotificationEvents` config toggle. AI.md:26827.
      `src/task/task.go`, `src/config/config.go:820-825`
- [x] **M11** Compliance-mode backup enforcement absent — spec requires
      blocking unencrypted backups + audit-log warning when
      `server.compliance.enabled`. AI.md:28951,28983-28989.
      `src/maintenance/maintenance.go`, `src/task/task.go`
- [x] **M12** Missing `su` (Linux/BSD) and `osascript` (macOS) privilege
      escalation fallbacks. AI.md:30049-30074.
      `src/service/privilege_unix.go:21,61`
- [x] **M13** OpenRC init script missing `name=`, `command_user=`,
      correct pidfile org dir, output/error logs, `depend()` dns/logger,
      `start_pre()` checkpath. AI.md:30716-30742.
      `src/service/service.go:414-426`
- [x] **M14** SysVinit init script missing `$remote_fs` dep,
      `DAEMON_USER`+`--chuid`, `LOGFILE` wiring, correct pidfile org dir;
      has undocumented extra `reload` case. AI.md:30765-30816.
      `src/service/service.go:469-516`
- [x] **M15** Test scripts declare `MIT` license header; spec requires
      `WTFPL` for all shell scripts. AI.md:36731.
      `tests/run_tests.sh`, `tests/incus.sh`, `tests/docker.sh`

## MINOR

- [x] `.claude/rules` PART-assignment headers mismatch spec groupings
      (binary-rules.md missing 32; backend-rules.md has 32 not 31; PART
      31 unassigned) — fixed: moved the "Client Binary (PART 32)"
      section from `backend-rules.md` to `binary-rules.md` (its
      canonical home per AI.md:5942's directory listing), and added the
      previously-missing "Tor Hidden Service (PART 31)" section to
      `backend-rules.md` (summarized from AI.md:39329-39422, PART 31).
      Headers now read `binary-rules.md` → PART 7, 8, 32 and
      `backend-rules.md` → PART 9, 10, 11, 31, matching AI.md:5942-5943
      exactly. AI.md:5942-5943, 39329-39422.
      `.claude/rules/binary-rules.md`, `.claude/rules/backend-rules.md`
- [x] Forbidden `github.com/mattn/go-sqlite3` checksum present in
      `go.sum` — CONFIRMED non-issue: `go mod why -m` shows it's a
      test-only transitive dependency of `modernc.org/sqlite` itself
      (its test suite compares against mattn/go-sqlite3), required by
      Go's module graph verification. Re-running `go mod tidy` keeps
      the entry; no source file imports it (`grep -rn` clean). We use
      `modernc.org/sqlite` correctly per AI.md:6421. Not a violation.
      AI.md:6421,6563
- [x] Windows "Privileged (Administrator)" paths unreachable — fixed:
      split `isRoot()` into `paths_unix.go` (`os.Geteuid() == 0`) and
      `paths_windows.go` (Administrator group membership via
      `golang.org/x/sys/windows`, matching the existing pattern in
      `service/privilege_windows.go`), replacing the old function that
      unconditionally returned `false` on Windows. Verified: gofmt,
      native build/vet, `go test ./src/paths/...`, and cross-compile
      build for windows/darwin/freebsd all pass. AI.md:6768-6779.
      `src/paths/paths.go`, `src/paths/paths_unix.go`,
      `src/paths/paths_windows.go`
- [x] Server `--help` output has extra Scheduler/Maintenance sections
      not in canonical spec block; also spec itself is internally
      inconsistent (`--help` vs bare `help`) — CONFIRMED non-issue on
      re-check: `printHelp()` in `src/main.go` has no Scheduler section
      and matches the canonical `Information/Shell Integration/Server
      Configuration/Service Management` block exactly (M2's dead
      `scheduler`/`--clean-expired`/`--email` commands are gone from
      the codebase entirely, not just undocumented). The only remaining
      delta is AI.md's own self-contradiction: the flags block
      (AI.md:10189,10202) declares `--shell {...,--help}` and
      `--maintenance {...,--help}`, while the "Server --help Output"
      example text (AI.md:10222,10245) shows bare `help` instead of
      `--help`. Code consistently follows the flags-block form
      (`--shell --help`, `<command> --help`) everywhere, which is the
      only self-consistent choice available. AI.md:10207-10246
- [x] `--color` accepts undocumented `always`/`never` aliases — CONFIRMED
      non-issue: `applyColor()` in both `src/main.go` and
      `src/client/main.go` treats `always`/`never` as synonyms for the
      canonical `yes`/`no` (AI.md:10195 `--color {auto|yes|no}`); this
      is a strict superset — all three canonical values still work
      exactly as spec'd, and the extra aliases (matching the common
      `git --color` convention) are additive, not a break. Not a
      violation. `src/main.go`, `src/client/main.go`
- [x] `compat.go` legacy endpoints use non-canonical error shape — CONFIRMED
      intentional: IDEA.md:68-79 requires 100% wire-compatibility with
      pastebin.com/dpaste/stikked/etc. wire protocols ("existing scripts...
      must work without modification"); canonical envelope would break
      that. Not a violation. `src/handler/compat.go`
- [x] `tracking.go` Google fallback branch skips `G-` prefix validation
      applied on primary branch — CONFIRMED non-issue: canonical
      `ValidateTracking()` (AI.md:16420-16429,
      `src/config/config.go:2489`) already enforces
      `^(UA-\d+-\d+|G-[A-Z0-9]+)$` on `cfg.Server.Tracking.ID` at
      config load/reload (warn-and-disable on mismatch, PART 5). The
      only call site (`server.go:638`, via `s.liveCfg()`) always
      passes an already-validated config, so by the time
      `renderTrackingScript`'s fallback (UA-XXXX legacy) branch runs,
      the ID is guaranteed to already match one of the two valid
      formats — re-checking the `G-` prefix there would be dead code.
      Not a violation. `src/server/tracking.go:32-38`
- [x] `HealthResponse.Maintenance` field placed mid-struct instead of
      after `Stats` per "add custom fields at end" ordering — fixed:
      moved `Maintenance` to the end of the struct, after `Stats`,
      matching AI.md:17142's "APP-SPECIFIC: Add custom fields here"
      slot. Construction site (`server.go:2381`) uses a keyed struct
      literal, so the reorder is safe. AI.md:17108-17144.
      `src/server/server.go:98-114`
- [x] `ChecksInfo.Config` field inserted between `Disk`/`Scheduler`
      instead of after `Tor` — fixed: moved `Config` to the end of
      the struct, after `Tor`, matching AI.md:17202's "APP-SPECIFIC:
      Add your checks here" slot. Construction site
      (`server.go:2295`) uses a keyed struct literal, so the reorder
      is safe. AI.md:17190-17204. `src/server/server.go:161-168`
- [x] `/api/v1/server/version` payload omits `commit`/`go_version`/
      `build.date` despite tracking them — fixed: `handleVersion()` now
      returns `version`/`go_version`/`build{commit,date}`, reusing the
      canonical `BuildInfo` shape defined for `/server/healthz`
      (AI.md:17118-17123,17156-17162, PART 13) instead of only `version`.
      AI.md has no dedicated JSON schema for this specific endpoint (it
      falls under the `/api/{api_version}/server/*` public info-endpoints
      wildcard, AI.md:19206), so this is an additive consistency fix, not
      a hard-schema violation. `src/server/server.go:2529-2542`
- [x] localStorage owner-token key mismatch — fixed: `remove.js` now
      uses the same `pastebin_owner_token` key as `create.js` (canonical
      per IDEA.md:34 and AI.md:11799), instead of the stray `api_token`
      key it read/wrote before. `src/server/static/js/remove.js`
- [x] CSS dark palette (`#1e1e2e`/`#cdd6f4`, Catppuccin) matches neither
      AI.md CSS var reference (`#1a1a2e`) nor Go theme struct
      (`#1a1b26`) — fixed via full wiring: rather than hand-syncing a
      second hardcoded hex literal, `src/common/theme/colors.go`'s
      `ThemePalette`/`GetThemePalette()` is now the single source of
      truth for every consumer, per AI.md:24320 ("Colors are defined
      ONCE in Go and used everywhere - Web CSS, TUI, CLI, GUI").
      - `src/server/static/css/{main.css -> main.css.tmpl}`: renamed
        and converted to a `text/template` that renders both
        dark/light palettes as CSS custom properties from template
        data instead of literal hex values
      - `src/server/theme_css.go`: new file — lazily parses the
        embedded `main.css.tmpl`, renders per-request with
        `theme.ThemePaletteDark`/`ThemePaletteLight`
      - `src/server/server.go`: exact chi route for
        `/static/css/main.css` registered ahead of the `/static/*`
        wildcard file server
      - `src/client/tui/theme.go`/`theme_test.go`: `tuiThemeFromPalette`
        maps the canonical palette onto the TUI's lipgloss-facing
        `TUITheme` fields; `darkTheme`/`lightTheme` derived from
        `theme.ThemePaletteDark`/`ThemePaletteLight`
      - `src/swagger/theme.go`, `src/graphql/theme.go`: `CSS()`
        renders CSS custom properties (including Swagger's 5
        `--method-*` HTTP-verb colors) from the same canonical
        palette instead of hardcoded hex blocks
      Verified: `go build ./...`, `go vet ./...`, `go test ./...`
      (all touched packages `ok`), and `go-lint` all pass clean inside
      the `casjaysdev/go:latest` Docker toolchain.
- [ ] `--service --help` missing "Current status" block (Service/State/
      Auto-start/PID). AI.md:30149-30169. `src/service/service.go:984-1003`
- [ ] Service Description hardcoded `"pastebin API Server"` instead of
      `{app_name}` placeholder — depends on IDEA.md value, needs check.
      `src/service/service.go:416,476`
- [ ] mkdocs.yml missing `pymdownx.arithmatex`/`pymdownx.magiclink`
      extensions. AI.md:37275,37290. `mkdocs.yml:55-91`
- [x] Skip-link text hardcoded English instead of `t()` key — fixed:
      all 18 templates now use `{{t .Lang "nav.skip_to_content"}}`,
      reusing the existing (previously unused) locale key present with
      parity across all 7 locale files. AI.md:22006, PART 30 key-parity
      requirement. `src/server/templates/*.html`
- [ ] `writeIfChanged` dead code, only referenced from tests.
      `src/tor/tor.go:434-440`
- [x] `Auth.TokenFile`/`auth.token_file` config field declared but never
      read; `--token-file` documented in spec help but unimplemented —
      fixed: added `--token-file` flag, wired into the PART 32 token
      priority chain (`--token` → `--token-file` → `PASTEBIN_TOKEN` env
      → cli.yml `auth.token` → cli.yml `auth.token_file`), added
      `readTokenFile()` helper (trims whitespace, errors on empty
      file), persists `--token-file` to cli.yml via the same
      `saveIfUnset` pattern as `--token`, documented in `--help`
      output, and added to the TUI-launch config-flag maps in
      `detectMode`. AI.md:42972,42451. `src/client/main.go`

## Not violations (confirmed compliant, noted only)
- PWA icons served via dynamic handlers, no static dir needed — fine.
- systemd `NoNewPrivileges=yes` — extra hardening beyond PART 24
  template, matches project's own service-rules.md — fine.
- PART 27 security jobs in separate `security.yml` vs spec's embedded
  layout — matches project's own cicd-rules.md — fine.
