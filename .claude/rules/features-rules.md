# Features Rules (PART 17-22)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Email & Notifications (PART 17)
- SMTP loaded from `server.yml`; silent disable when unconfigured
- 7 env var overrides: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM`, `SMTP_TLS`, `SMTP_SKIP_VERIFY`
- 7 required email templates (file-based, embedded English default subjects + `{variable}` substitution):
  - `security_alert`, `backup_complete`, `backup_failed`, `ssl_expiring`, `ssl_renewed`, `scheduler_error`, `test`
- NEVER use i18n keys for email subjects — use embedded English templates

## Built-in Scheduler (PART 18)
- Internal `time.Ticker` engine — NEVER external cron
- Custom cron parser + DB-backed state + catch-up window
- 10 required tasks: `ssl_renewal`, `geoip_update`, `blocklist_update`, `cve_update`,
  `token_cleanup`, `log_rotation`, `backup_daily`, `backup_hourly`, `healthcheck_self`,
  `tor_health` (+ `expire-pastes`)

## GeoIP (PART 19)
- Library: `github.com/oschwald/maxminddb-golang` (NOT geoip2-golang)
- Auto-download from jsDelivr CDN; atomic rename; fail-open
- RFC1918/allowlist bypass (private IPs skip GeoIP check)
- Middleware applied router-wide (web + API)
- Graceful disable when DB unavailable

## Prometheus Metrics (PART 20)
- Namespace: `pastebin_`
- `/metrics` internal-only: firewall/IP restriction + optional bearer token (constant-time compare)
- Required histogram buckets for `scheduler_task_duration_seconds`: `{0.1,0.5,1,5,10,30,60,300,600}`

## Backup & Restore (PART 21)
- Filename format: `{name}_backup_YYYY-MM-DD_HHMMSS.tar.gz[.enc]`
- SHA-256 manifest included in every backup
- Encryption: AES-256-GCM + Argon2id key derivation (when password set)
- File permissions: 0o600
- Restore: MUST call `VerifyBackup()` before extracting — abort on verification failure
- NEVER store backup password in plaintext

## Update Command (PART 22)
- `CheckForUpdate`: GitHub release API; 404 = no update available
- `DoUpdate`: download to temp in binary dir → SHA-256 verify → atomic `os.Rename`
- `RestartSelf`: `syscall.Exec` for in-place restart
- `--update [check|yes|branch {stable|beta|daily}]`
- Self-update source: GitHub releases ONLY (no site.txt for self-update)

---
For complete details, see AI.md PART 17-22
