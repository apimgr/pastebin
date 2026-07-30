# Backend Rules (PART 9, 10, 11, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never expose stack traces, internal errors, or Go error chains in production responses (Tier 3 debug-only, `--debug`/`DEBUG=true` gated)
- Never invent a new error response shape — the canonical `{ok, data}` / `{ok, error, message, details}` JSON shape is defined in PART 14; never redefine it here
- Never retry non-retryable errors (4xx) — only retry transient errors (timeouts, 503, connection refused)
- Never use migration files or a schema-version tracking table — all schema changes are idempotent `CREATE TABLE IF NOT EXISTS` / `ALTER TABLE ADD COLUMN` run on every startup
- Never `DROP COLUMN`, `DROP TABLE`, or `DELETE` as part of a schema update — additive only
- Never rename a database column directly — add new column, migrate in app code, deprecate old (3-step process), keep old column forever
- Never open a DB connection without pooling (`SetMaxOpenConns`/`SetMaxIdleConns`/`SetConnMaxLifetime`/`SetConnMaxIdleTime`)
- Never run a database query without a `context.Context` timeout
- Never expose Tier 1 secrets publicly (DB credentials/DSN, internal IPs/hostnames, any user's tokens, PGP private keys, other users' PII, filesystem paths, account-existence signals, exact rate-limit thresholds) — not even in debug mode
- Never compare tokens, password hashes, HMACs, signatures, or TOTP codes with `==`, `bytes.Equal`, or `strings.EqualFold` — always `crypto/subtle.ConstantTimeCompare`
- Never return different auth-failure messages/timing for "wrong password" vs "no such user" vs "locked" vs "expired" — always identical message + status + padded timing (≥100ms floor)
- Never expose sequential integer primary keys in public URLs/JSON/logs — external IDs must be opaque (UUID v4/v7); internal `BIGSERIAL` PKs stay internal
- Never log a submitted password/token, even hashed — log only a stable identifier hash (`token_prefix_sha256[:8]`)
- Never cast user-controlled content (pastes, uploads, markdown-derived HTML) to `template.HTML` without sanitization; never render user-supplied HTML/SVG/XML inline — force `Content-Disposition: attachment` for active MIME types
- Never shell out with raw user content, filenames, or refs; never run hooks/build steps/interpreters/package managers on untrusted content
- Never log: private keys/TLS secrets, SMTP credentials, full API credentials, credit card/financial data
- Never emit `Server-Timing` or `_debug` fields in production — strip via `dev_only:"true"` tag middleware
- Never use the system/default Tor process or default Tor ports (9050/9051) — the app runs its OWN dedicated Tor process on runtime-detected ports
- Never hardcode or persist Tor SOCKS/control ports across restarts — always detected at runtime
- Never let a Tor failure prevent server startup — Tor is best-effort, always optional at the process level even though hidden-service support itself is mandatory when the Tor binary is present

## CRITICAL - ALWAYS DO
- Log every error with context (`error_code`, `request_id`, `http_status`, internal error) — `Error()` for 5xx, `Warn()` for 4xx
- Use exponential backoff (0s,1s,2s,4s,8s, cap 30s) for retryable transient errors
- Use hierarchical lowercase colon-separated cache keys (`{type}:{id}`, `rate:{type}:{key}`, versioned `v1:user:123`)
- Set `Cache-Control` per content class: static assets `public, max-age=31536000, immutable`; HTML `no-store`; public API `public, max-age=60`; anything authenticated/private `private, no-store`
- Run `EnsureSchema` on every startup — idempotent create + idempotent alter, ignore "duplicate column" errors
- Give new columns a `DEFAULT` or make them nullable
- Use parameterized queries ONLY — never `fmt.Sprintf` SQL with user input (SQL injection defense-in-depth Layer 2)
- Wrap multi-statement writes in a transaction (`WithTransaction`) with rollback on error
- Use optimistic locking (`version` column) or `sql.LevelSerializable` + retry for contended writes
- Apply the Public Endpoint Safety Principle to every no-auth surface: Tier 1 never public, Tier 2 always public (version, commit_hash, build_date, uptime, mode, db_type, aggregate metrics), Tier 3 only under `--debug`
- Defend every threat (SQLi, XSS, enumeration, timing oracles, credential stuffing, path traversal, token leakage, CSRF) in ALL four layers: input validation, data access, output, transport — never assume another layer catches it
- Render all user content as escaped text or sanitized markdown; syntax highlighting adds wrapper spans only AFTER escaping
- Serve binary/active-type downloads with exact `Content-Type` + `X-Content-Type-Options: nosniff`
- Store cryptographic secrets (`installation_secret`, `cookie_signing_key`, `csrf_token_secret`, `server.security.encryption_key`) per their documented storage location, rotation cadence, and audit-log event name; never let any appear in a request, response, or log
- Rotate secrets only through `--maintenance secret rotate <name>`, authorized like other sensitive operations (re-prompt `server.token`, log to `audit.log`)
- Audit-log every event with: UTC timestamp with milliseconds, IP address, actor identity, success/failure result, unique event ID
- Mask tokens/IDs in logs to first 8 characters or a separate non-value ID field
- Start a dedicated, self-managed Tor process (own binary invocation) whenever the Tor binary is found on the host — hidden service is always-on in that case, no config toggle
- Keep Tor directories under the app's own dirs: `{config_dir}/tor/`, `{data_dir}/tor/`, `{log_dir}/tor.log`
- Run the Tor process as the same user the server runs as, after privilege drop
- Restart Tor on documented trigger events (config change, port conflict, crash) without restarting the whole server

## Key Rules Summary

### Error handling
| Aspect | Rule |
|---|---|
| Response shape | Canonical `{ok, data}` / `{ok, error, message, details}` — see PART 14 |
| Logging | Every error logged with `error_code`, `request_id`, `http_status`; 5xx = Error, 4xx = Warn |
| Retry | Exponential backoff, only for retryable (timeout/503/refused) errors, never 4xx |

### Caching
| Driver | Use case |
|---|---|
| `memory` | Dev / small deployments (default) |
| `valkey` | Production (preferred) |
| `redis` | Production (full compat) |

TTL defaults: API tokens = no expiry; rate limits = 1 min; user profile = 5 min; config = 1 min; static hash = 24h; GeoIP = 7 days; blocklist = 1h; page cache = 5 min; API response = 30s.

### Database
| Rule | Detail |
|---|---|
| Schema | `CREATE TABLE IF NOT EXISTS` + idempotent `ALTER TABLE`, no migration files |
| Pool sizing | dev 5/2, small 25/5, medium 50/10, large 100/20 (max_open/max_idle) |
| Query timeouts | simple SELECT 5s, complex/JOIN 15s, write 10s, bulk 60s, migrations 5m, reports 2m |
| Column rename | Never — add new, dual-write, deprecate old, keep column forever |

### Security tiers (public endpoint safety)
| Tier | Examples | Exposure |
|---|---|---|
| 1 | DB creds, internal IPs, any user's tokens, PGP private keys, other users' PII | Never, even in debug |
| 2 | app_name, version, commit_hash, build_date, uptime, mode, db_type, aggregate metrics | Always public |
| 3 | Stack traces, CSP/CSRF/CORS violation context, validation detail, rate-limit counters | `--debug`/`DEBUG=true` only, stripped in production via `_debug`/`dev_only:"true"` |

### Cryptographic keys
| Key | Length | Storage | Rotation |
|---|---|---|---|
| `installation_secret` | 32B | `server.db` | Manual, `--maintenance secret rotate installation_secret` |
| `cookie_signing_key` | 32B | `server.db` | Auto every 90 days |
| `csrf_token_secret` | 32B | `server.db` | Auto on API key regen + every 180 days |
| `server.security.encryption_key` | 32B AES-256-GCM | `server.yml` | Manual, `--maintenance secret rotate encryption_key` |

### Tor
| Rule | Detail |
|---|---|
| Enable | Always-on if Tor binary found, no config toggle |
| Ports | Never 9050/9051 — runtime-detected localhost ports only |
| Process | App's own dedicated Tor process, never system Tor |
| Failure mode | Server startup never blocked by Tor failure |
| Dirs | `{config_dir}/tor/`, `{data_dir}/tor/`, `{log_dir}/tor.log` |

For complete details, see AI.md PART 9, 10, 11, 31.
