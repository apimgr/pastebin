# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never write inline YAML comments (`port: 8080  # port`) — comments always go ABOVE the setting. Exception: GitHub Actions SHA-pin annotations (`uses: owner/action@{sha}  # vX.Y.Z`) stay inline for Renovate.
- Never accept unvalidated/unnormalized paths anywhere — config paths, HTTP request paths, file paths, API params. Always run through `normalizePath`/`validatePath`/`SafePath`.
- Never use `strconv.ParseBool()` directly for any boolean input (env vars, config, CLI flags, API params, form inputs, query strings) — always use `config.ParseBool()` / `config.IsTruthy()`.
- Never hardcode host, IP, or port anywhere in project code (`localhost`, `127.0.0.1`, `0.0.0.0`, `[::1]`, any static host/IP).
- Never display a bare path (`GET /api/`) without the full detected URL.
- Never fail startup on an invalid config value — warn and replace with the default instead.
- Never let `--debug`/`DEBUG=true` bypass authentication or security checks — debug affects verbosity/diagnostics ONLY, in every mode including production.
- Never expose debug/pprof/expvar endpoints unless `--debug` or `DEBUG=true` is explicitly set (otherwise 404).
- Never store user accounts or operator-editable configuration in the database — `server.yml` is the sole configuration source of truth; database holds resource state, owner tokens, audit log only.
- Never treat more than two error classes as startup-critical — only DB connection failure and inability to write files (disk full/permissions) trigger maintenance mode; everything else is recoverable.
- Never put maintenance-mode operational metadata (`Retry-After`, reason codes) in ad-hoc top-level JSON body fields — use the canonical error body plus standard/`X-Maintenance-*` headers.
- Never trust `X-Forwarded-*` headers from a peer outside `trusted_proxies` (private ranges + configured `additional` allow-list) — drop them before URL/CORS/CSP resolution.
- Never evaluate the proxy-trust gate against a rewritten `r.RemoteAddr` — real-IP middleware must preserve the original TCP peer for `isTrustedPeer()`; only client-IP consumers (rate limiting, GeoIP, blocklists) use the resolved IP.
- Never re-resolve the backup directory path at cleanup time — use the value resolved and cached at startup.
- Never hand-roll primary flag parsing for the server binary (no manual `os.Args`/`switch` loops) — use stdlib `flag`.

## CRITICAL - ALWAYS DO

- Store all settings in `server.yml`; the operator edits it directly (no admin web UI).
- Validate every config value on load; on invalid value, log a warning and substitute the built-in default.
- Use `{proto}://{fqdn}:{port}/path` format for every displayed/generated URL, with `{proto}`, `{fqdn}`, `{port}` detected from request context (reverse-proxy headers preferred), and strip default ports (`:80` HTTP, `:443` HTTPS).
- Select a random unused port in `64000-64999` on first run when none is configured, then persist it to `server.yml` for all future restarts.
- Run `PathSecurityMiddleware` early in the middleware chain (position #3: after URL normalize + request-ID, before auth/routing).
- Drop to a dedicated service user after privileged setup/port binding — permanent root/Administrator only for a project-specific, IDEA.md-documented exception.
- Enter maintenance mode (read-only + operator guidance) on a true critical error, and continuously attempt self-healing with backoff until resolved, then auto-recover.
- Accept the full truthy/falsy vocabulary (yes/no, on/off, enable/disable, si/non, etc., case-insensitive) for every boolean input; empty/unset uses the default; invalid value is an error, never a silent default.
- Auto-migrate `server.yaml` → `server.yml` on startup if the old name is found.
- Resolve `baseurl` in priority order: `X-Forwarded-Prefix` → `X-Forwarded-Path` → `X-Script-Name` → config/CLI `--baseurl` → default `/`.
- Bypass the trusted-proxy gate entirely for Tor requests (`Host` matches `tor.onion_address`) — resolve FQDN/proto/port from `tor.*` config instead.

## Key Rules Summary

**Mode/Debug precedence:**
| Aspect | Priority order |
|---|---|
| Mode | `--mode` flag > `MODE` env > default `production` |
| Debug | `--debug` flag > `DEBUG` env (truthy) > `--mode debug` alias > default `false` |

`--mode debug`/`MODE=debug` = `development` + debug on, unless `--debug`/`DEBUG` explicitly overrides.

**Four operational states:** Production, Production+Debug, Development, Development+Debug — see PART 6 table for per-state logging/caching/rate-limit behavior.

**Config file:**
| User | Path |
|---|---|
| Root | `/etc/apimgr/pastebin/server.yml` |
| Regular | `~/.config/apimgr/pastebin/server.yml` |

Design rules: clean/intuitive, everything configurable, sane built-in defaults (no 1000-line configs), comments single-line <140 chars.

**Port handling:**
| Port | Behavior |
|---|---|
| `80` | Enable Let's Encrypt HTTP-01 |
| `443` | Enable Let's Encrypt TLS-ALPN-01, auto-enable SSL |
| `0` | OS assigns any available port |
| `64000-64999` | Default random-selection range |
| `<1024` | Requires privileged startup + drop, or fallback to `>1024` |

**Env vars (runtime, always checked):** `NO_COLOR`, `TERM`, `DOMAIN`, `MODE`, `DATABASE_DRIVER`, `DATABASE_URL`, `SMTP_*`.
**Env vars (init-only, first run):** `CONFIG_DIR`, `DATA_DIR`, `LOG_DIR`, `DATABASE_DIR`, `BACKUP_DIR`, `PORT`, `LISTEN`, `APPLICATION_NAME`, `APPLICATION_TAGLINE`.

**Trusted proxies (always trusted, no config):** loopback, RFC 1918, RFC 4193, link-local, same `/24` as listen address. Add public upstreams via `trusted_proxies.additional` (IP/CIDR/DNS, refreshed every 5 min).

**Request limits defaults:** `max_body_size: 10MB`, `read_timeout/write_timeout: 30s`, `idle_timeout: 120s`.

**Middleware execution order (1→10):** URL normalize → request ID → path security → security headers → allowlist → blocklist → rate limit → GeoIP → auth → logging.

For complete details, see AI.md PART 5, 6, 12.
