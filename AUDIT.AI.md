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

- [ ] `.claude/rules` PART-assignment headers mismatch spec groupings
      (binary-rules.md missing 32; backend-rules.md has 32 not 31; PART
      31 unassigned). AI.md:5942-5943
- [ ] Forbidden `github.com/mattn/go-sqlite3` checksum present in
      `go.sum` (unused, stale transitive entry). AI.md:6421,6563
- [ ] Windows "Privileged (Administrator)" paths unreachable —
      `isRoot()` always false on Windows. AI.md:6768-6779.
      `src/paths/paths.go:23-28`
- [ ] Server `--help` output has extra Scheduler/Maintenance sections
      not in canonical spec block; also spec itself is internally
      inconsistent (`--help` vs bare `help`). AI.md:10207-10246
- [ ] `--color` accepts undocumented `always`/`never` aliases (superset,
      not a break). `src/main.go`, `src/client/main.go`
- [ ] `compat.go` legacy endpoints use non-canonical error shape —
      likely intentional (legacy dpaste-compat surface), needs
      confirmation. `src/handler/compat.go`
- [ ] `tracking.go` Google fallback branch skips `G-` prefix validation
      applied on primary branch (operator-config, low risk).
      `src/server/tracking.go:32-38`
- [ ] `HealthResponse.Maintenance` field placed mid-struct instead of
      after `Stats` per "add custom fields at end" ordering.
      AI.md:17108-17144. `src/server/server.go:110`
- [ ] `ChecksInfo.Config` field inserted between `Disk`/`Scheduler`
      instead of after `Tor`. AI.md:17190-17204.
      `src/server/server.go:160-167`
- [ ] `/api/v1/server/version` payload omits `commit`/`go_version`/
      `build.date` despite tracking them. `src/server/server.go:2496-2498`
- [ ] localStorage owner-token key mismatch: create.html writes
      `pastebin_owner_token`, remove.html reads `api_token` — breaks
      pre-fill UX. `src/server/templates/create.html:207`,
      `remove.html:106,109`
- [ ] CSS dark palette (`#1e1e2e`/`#cdd6f4`, Catppuccin) matches neither
      AI.md CSS var reference (`#1a1a2e`) nor Go theme struct
      (`#1a1b26`). `src/server/static/css/main.css:9-16`
- [ ] `--service --help` missing "Current status" block (Service/State/
      Auto-start/PID). AI.md:30149-30169. `src/service/service.go:984-1003`
- [ ] Service Description hardcoded `"pastebin API Server"` instead of
      `{app_name}` placeholder — depends on IDEA.md value, needs check.
      `src/service/service.go:416,476`
- [ ] mkdocs.yml missing `pymdownx.arithmatex`/`pymdownx.magiclink`
      extensions. AI.md:37275,37290. `mkdocs.yml:55-91`
- [ ] Skip-link text hardcoded English instead of `t()` key (spec's own
      example does the same — low priority). `src/server/templates/*`
- [ ] `writeIfChanged` dead code, only referenced from tests.
      `src/tor/tor.go:434-440`
- [ ] `Auth.TokenFile`/`auth.token_file` config field declared but never
      read; `--token-file` documented in spec help but unimplemented.
      AI.md:42972. `src/client/main.go:83`

## Not violations (confirmed compliant, noted only)
- PWA icons served via dynamic handlers, no static dir needed — fine.
- systemd `NoNewPrivileges=yes` — extra hardening beyond PART 24
  template, matches project's own service-rules.md — fine.
- PART 27 security jobs in separate `security.yml` vs spec's embedded
  layout — matches project's own cicd-rules.md — fine.
