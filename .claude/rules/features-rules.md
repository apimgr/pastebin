# Features Rules (PART 17-22)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Covers: Email & Notifications, Scheduler, GeoIP, Metrics, Backup & Restore, Update Command.

## CRITICAL - NEVER DO
- Never send email without valid, working SMTP — no queueing "for later", no "would have sent" log lines
- Never show email-dependent UI when SMTP is not configured
- Never use an external scheduler (cron, crond, systemd timers, Task Scheduler, launchd, Kubernetes CronJob, cloud schedulers) — the built-in scheduler is the ONLY scheduler, no exceptions
- Never treat GeoIP/country as the sole access-control gate — it is a risk signal only, never a replacement for auth/rate-limiting
- Never block a request solely because GeoIP lookup failed (fail-open for GeoIP, fail-closed for real auth)
- Never expose `/metrics` publicly without considering firewall/proxy/token protection — it is INTERNAL ONLY
- Never use a raw client IP (or any high-cardinality value) as a Prometheus label — unbounded cardinality is a memory-DoS vector
- Never store the backup encryption password — it is never persisted, only used on-demand
- Never accept a backup password via CLI flag — interactive prompt only (flags leak via shell history/process list)
- Never delete old backups before the new backup passes ALL verification checks
- Never inject update/version status into API responses, headers, or any public endpoint (Tier 3 info — operator-only)
- Never re-send the same "update available" notice repeatedly — fires once per newly-eligible version
- Never let `update_check` (notify-only) touch the binary unless `auto_install: true`
- Never surface operator events (backups, SSL, scheduler, updates) in the public WebUI — operators only see logs/email/CLI
- Never allow more than one operator notification for the same failure (e.g. `backup_failed` suppresses `scheduler_error` for that execution)

## CRITICAL - ALWAYS DO
- Auto-detect local SMTP on first run (loopback, docker bridge, gateway, FQDN, global IPv4, mail./smtp. subdomains); test configured SMTP on every startup
- Provide sane, working defaults for every email template out of the box; allow full customization via `{config_dir}/template/email/`
- Validate email templates before saving (unknown variables, empty subject/body, syntax errors)
- Run the built-in scheduler continuously from startup to shutdown; persist task state in `server.db`; catch up missed tasks within `catch_up_window`
- Implement all required built-in scheduled tasks: `ssl_renewal`, `geoip_update`, `blocklist_update`, `cve_update`, `update_check`, `token_cleanup`, `log_rotation`, `backup_daily`, `backup_hourly` (disabled by default), `healthcheck_self`, `tor_health`
- Download GeoIP databases (sapics/ip-location-db, MMDB format) on first run and update via scheduler — NEVER embed them in the binary
- Use `github.com/oschwald/maxminddb-golang` for GeoIP lookups — NOT `geoip2-golang` (ip-location-db's custom `database_type` strings break geoip2's parser)
- Expose Prometheus-compatible metrics at `/metrics` using `github.com/prometheus/client_golang`, prefixed `{project_name}_`, following Prometheus naming (snake_case, `_total`/`_seconds`/`_bytes` suffixes, base units)
- Verify every backup immediately after creation (file exists, size>0, checksum, decrypt test, manifest parse, content extraction, DB integrity) — delete on any failure, keep existing backups
- Enforce backup encryption (AES-256-GCM, Argon2id key derivation) as mandatory when `server.compliance.enabled: true`
- Support `{project_name} --update check|yes|branch {stable|beta|daily}`; treat channels as cumulative (beta/daily never lag behind stable)
- Honor `update.defer_days` for the scheduled `update_check` task only — manual `--update check`/`--update yes` always see the true latest

## Key Rules Summary

### Email
| Aspect | Rule |
|---|---|
| Requirement | Valid SMTP required; no SMTP = no email, period |
| Template format | `Subject: ...` line, `---` separator, plain-text body with `{variable}` syntax |
| Storage | Defaults embedded in binary; overrides in `{config_dir}/template/email/` |
| From address default | `no-reply@{fqdn}` (or `no-reply@localhost`) |
| Required templates | security_alert, backup_complete, backup_failed, ssl_expiring, ssl_renewed, ssl_renewal_failed, scheduler_error, update_available, update_installed, test |
| Env override | `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_TLS`, `SMTP_FROM_NAME`, `SMTP_FROM_EMAIL` |

### Notification Channels
| Channel | Audience | Availability |
|---|---|---|
| Public WebUI (toast/banner) | Visitors | Always, client-side only |
| Logs | Operators | Always |
| Email | Operators | Requires SMTP |

No notification center/bell/history exists — API projects have no accounts. Banner dismissal lives in the `dismissed_announcements` cookie.

### Scheduler
| Rule | Detail |
|---|---|
| Format | Standard cron, or `@hourly`/`@daily`/`@weekly`/`@monthly`/`@every Xm`/`@every Xh` |
| State | Persisted in `server.db`: last_run, last_status, next_run, run_count, fail_count |
| Shutdown | Wait up to 30s for running tasks, then force-release and mark for retry |
| CLI | `{project_name} scheduler list|show <id>|run <id>|enable <id>|disable <id>|history <id>` |
| Implementation | Go `time.Ticker`, no external cron library |

### GeoIP
| Rule | Detail |
|---|---|
| Source | sapics/ip-location-db via jsDelivr CDN, no API key |
| Databases | asn.mmdb, country.mmdb (geo-whois-asn), dbip-city-ipv4/ipv6.mmdb; WHOIS = combined ASN+Country lookup (no separate file) |
| Blocking modes | `deny_countries` (blocklist) or `allow_countries` (allowlist); if both set, allowlist wins |
| Exemptions | Allowlisted IPs and private/RFC1918 IPs always bypass country blocking |

### Metrics
| Rule | Detail |
|---|---|
| Endpoint | `/metrics`, Prometheus text format |
| Auth | Optional bearer token via `server.metrics.token` |
| Required metrics | `{project_name}_app_info`, `_app_uptime_seconds`, `_http_requests_total`, `_http_request_duration_seconds`, DB metrics if DB used, auth attempt/session metrics |
| Cardinality | Low-cardinality labels only (method, status, path with `:id` normalization); never per-IP or per-user-ID labels |

### Backup & Restore
| Rule | Detail |
|---|---|
| Command | `{project_name} --maintenance backup [filename]` / `--maintenance restore <file>` |
| Naming | `{project_name}_backup_YYYY-MM-DD[_HHMMSS].tar.gz[.enc]` (full), `{project_name}-daily.tar.gz[.enc]` / `-hourly.tar.gz[.enc]` (incremental) |
| Encryption | AES-256-GCM + Argon2id; mandatory under compliance mode; password never stored, never a CLI flag |
| Retention | `max_backups`, `keep_weekly`, `keep_monthly`, `keep_yearly`, `max_total_size` (size cap overrides count limits); priority: yearly > monthly > weekly > daily |
| Restore auth | Empty DB or root: allowed; service user: requires `server.token`; random user: denied |

### Update
| Command | Behavior |
|---|---|
| `--update check` | Check only, no privileges required |
| `--update yes` (default) | Check + in-place update + restart |
| `--update branch {stable\|beta\|daily}` | Set/persist release channel to `server.yml` |
| `update.auto_install` | Default `false` — `update_check` task only notifies unless explicitly enabled |
| `update.defer_days` | Gates the scheduled task only; manual commands always see true latest |

For complete details, see AI.md PART 17, 18, 19, 20, 21, 22.
