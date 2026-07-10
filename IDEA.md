## Project description

Pastebin is a full-stack Go web application for creating and sharing text snippets anonymously. It is a drop-in replacement for pastebin.com, microbin, lenpaste (fork: forksmgr/lcomrade-lenpaste), stikked, hastebin/haste-server, dpaste, the curl-upload family (0x0.st, sprunge.us, ix.io), and termbin/fiche (raw-TCP, enabled by default) — existing scripts, CLIs, and integrations targeting any of those services work against this server without modification. Users submit text or upload a file and receive a short shareable URL. No user accounts. Fully public. Deployed as a single self-contained static binary. The companion CLI (`pastebin-cli`) provides full API access from the terminal.

## Project variables

project_name: pastebin
project_org: apimgr
internal_name: pastebin
internal_org: apimgr
app_name: Pastebin
repo: https://github.com/apimgr/pastebin
official_site: https://pste.us
license: MIT
binary: pastebin
client_binary: pastebin-cli
api_version: v1
coverage_minimum: 80

## Business logic

### Product scope & non-goals

**In scope:**
- Anonymous paste creation via web form, JSON API, raw body (curl pipe), or multipart file upload
- Server-side syntax highlighting via Chroma — no client-side JS library required
- Language auto-detection from file extension on upload; manual selection otherwise
- Expiry options: `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never` (default), or a custom duration in seconds
- Burn after N reads: paste is permanently deleted once its view count reaches a user-set threshold (1–9999); `0` = disabled
- Visibility: public (listed in recent pastes) or unlisted (URL-only, not listed)
- Owner token: a `tok_`-prefixed cryptographically random token returned in every API create response; reusable across pastes created by the same caller; stored as a SHA-256 hash; required to delete the paste before natural expiry
- Token reuse: callers may supply an existing owner token on paste creation to link the new paste to that token; the web UI saves the token to `localStorage` key `pastebin_owner_token` and pre-fills it on subsequent creates
- Raw paste view, file download, iframe-embeddable view, QR code PNG
- View count tracking; automatic background cleanup of expired and burned pastes
- Full web frontend (server-side Go templates, dark/light/auto theme, PWA, mobile-first)
- Static info pages: about, help, health check, privacy, terms
- CLI client (`pastebin-cli`) — full API access from the terminal
- OpenAPI/Swagger docs
- GraphQL read-only query interface (create and delete are REST-only)
- Drop-in compat layer for pastebin.com, microbin, lenpaste, stikked, hastebin/haste-server, dpaste, the curl-upload family (0x0.st, sprunge.us, ix.io), and termbin/fiche (raw-TCP listener, enabled by default; disable via `server.termbin.enabled: false` or `TERMBIN_ENABLED=false`) — compat routes use their own delete-token mechanism (stored in paste row) separate from the owner token system
- Two-tier operator token: server operator may set a `server.token` in `server.yml`; this operator token allows deleting any paste unconditionally
- i18n: 7 supported locales (en, es, fr, de, zh, ar, ja); automatic language selection via `Accept-Language` header; RTL layout for Arabic (`dir="rtl"` on `<html>`); fallback to English when locale unknown
- Tor hidden service: auto-enabled when the `tor` binary is found on `$PATH` or in common locations; v3 .onion address with persistent ed25519 key; non-fatal when Tor is absent; uses `github.com/cretz/bine` (pure Go, CGO_ENABLED=0 preserved)
- GeoIP: MaxMind GeoLite2-Country database; auto-downloaded via jsDelivr CDN by the `geoip_update` scheduled task; applied per-request via middleware; country-based access control (allowlist or denylist); graceful fail-open when database unavailable
- Prometheus metrics at `/metrics` (`pastebin_` namespace); internal-only (IP allowlist + optional bearer token on same port); never exposed publicly
- Built-in task scheduler (internal `time.Ticker` engine, no external cron): 10 registered tasks — `ssl_renewal`, `geoip_update`, `blocklist_update`, `cve_update`, `token_cleanup`, `log_rotation`, `backup_daily`, `backup_hourly`, `healthcheck_self`, `tor_health` — plus `expire-pastes` for expiry and burn-after cleanup
- Backup and restore: AES-256-GCM encryption + Argon2id key derivation when a backup password is set; `sha256_manifest.txt` integrity file per archive; scheduled hourly and daily; CLI-triggerable; restore verifies manifest before extracting
- Self-update: checks GitHub Releases API for new versions; downloads to a temporary file in the binary's directory, verifies SHA-256 checksum, then performs an atomic `os.Rename` swap; `--update`, `--check`, and `--branch` flags
- Service lifecycle management: `--install`/`--uninstall` (requires root privilege; uninstall prompts interactive `[y/N]` confirmation before data purge); `--start`/`--stop`/`--restart`/`--reload`/`--disable`/`--enable`; systemd unit (Linux), launchd plist (macOS), SCM service (Windows)
- Rate limiting: per-endpoint, IP-based, configurable; proxy-aware (`X-Forwarded-For`, `CF-Connecting-IP`, `True-Client-IP`)
- IP/CIDR/country blocklist: loaded from a URL or local file; refreshed hourly by `blocklist_update` scheduled task
- Email notifications: SMTP; 7 template types (`security_alert`, `backup_complete`, `backup_failed`, `ssl_expiring`, `ssl_renewed`, `scheduler_error`, `test`); silently disabled when unconfigured

**Non-goals:**
- No user accounts, registration, or login of any kind
- No admin web panel — server configured via `server.yml` only; no runtime config API
- No paste editing after creation — pastes are immutable
- No password-protected pastes (microbin per-paste encryption not implemented)
- No paid tiers, no rate-limited access tiers, no feature gating
- No cluster mode, no horizontal scaling, no node election — single instance only

### Compat targets

The server must be 100% wire-compatible with the following services — existing scripts, CLIs, and API integrations targeting these services must work without modification:

| Target | Guarantee |
|--------|-----------|
| **pastebin.com public API** | Paste creation, retrieval, and login-stub endpoints match the pastebin.com wire protocol (URL paths, request fields, response fields, HTTP status codes, content types) |
| **lenpaste** (protocol reference: `forksmgr/lcomrade-lenpaste`) | lenpaste REST API including server info endpoint matches the lenpaste wire protocol |
| **microbin** | microbin JSON API for paste creation and retrieval matches the microbin wire protocol |
| **stikked** | `POST /api/create` (form-urlencoded `text`/`title`/`name`/`lang`/`expire`/`private`; `apikey` accepted and ignored) returns the plain-text view URL `{base}/view/{id}` or `Error: <msg>`; `GET /view/raw/{id}` raw content; `GET /view/{id}` redirects to the native view; `GET /api/paste/{id}` returns the stikked JSON shape (`pid`, `title`, `name`, `created`, `lang`, `raw`, `hits`) |
| **hastebin / haste-server** | `POST /documents` (raw request body) returns `{"key":"<id>"}`; `GET /documents/{key}` returns `{"data":"...","key":"..."}` (404 `{"message":"..."}`); `GET /raw/{key}` served by the native raw route |
| **dpaste** | `POST /api/` and `POST /api/v2/` (form-urlencoded `content`/`lexer`/`syntax`/`filename`/`expires`/`format`) return the view URL — quoted by default, bare with `format=url`, or `{"url","content","lexer"}` with `format=json`; keyless |
| **curl-upload family** (0x0.st, sprunge.us, ix.io) | `POST /` dispatches by field: multipart `file` (0x0.st, returns an `X-Token` header), form `sprunge` (sprunge.us), or form `f:1` (ix.io); each returns a bare raw-content URL `{base}/raw/{id}` followed by a newline. Absent any of these fields, the request falls through to the native paste-create handler |
| **termbin / fiche** (raw-TCP) | Plain-TCP listener (enabled by default; disable via `server.termbin.enabled: false` / `TERMBIN_ENABLED=false`, default port `9999`): client connects, streams content, half-closes the write side; server stores the paste and responds with `{base}/{id}\n`, then closes. Max payload `server.termbin.max_size` (default 32768 bytes); idle/read timeout `server.termbin.timeout` (default `5s`). Wire-compatible with the `termbin.com` netcat workflow (`echo text \| nc host 9999`) and the fiche server protocol |

Compatibility is wire-level only: URL paths, request/response field names, HTTP status codes, and content types must match. Internal implementation details (storage, ID format, auth mechanism, delete convention) do not need to match. Compat-created pastes use each target's own deletion convention — not the native owner token system — and the two systems must never be mixed.

### Roles & permissions

No user roles exist. All native API endpoints are public. The only privilege distinction is the operator token configured server-side.

| Actor | Access |
|-------|--------|
| **Anonymous visitor (browser)** | Create pastes; view any public or unlisted paste by URL; browse recent public pastes; delete own paste with owner token |
| **Anonymous API client (curl / CLI)** | Create pastes; retrieve paste content; delete paste with owner token; list recent public pastes; token list and revoke |
| **Server operator** | Configures server via `server.yml`; holds operator token that can delete any paste unconditionally |

### Data model & sensitivity

**Paste record** (no PII stored):

| Field | Sensitivity |
|-------|-------------|
| `id` | Public |
| `title` | Public (user-controlled) |
| `content` | Public (user-controlled; may contain sensitive data — service makes no confidentiality guarantee) |
| `language` | Public |
| `visibility` | Public |
| `burn_after` | Public |
| `expires_at` | Public |
| `delete_token_hash` | Internal — used only by compat layer deletion; never returned after creation |
| `views` | Public |
| `created_at` | Public |

**API token record** (owner token system, separate from compat delete tokens):

| Field | Sensitivity |
|-------|-------------|
| `token_hash` | Internal — SHA-256 hex of the raw token; never returned |
| `token_prefix` | Semi-public — first 12 chars of the raw token; used for CLI revoke lookup |
| `resource_type` | Internal |
| `resource_id` | Internal |
| `created_at`, `expires_at`, `last_used_at`, `revoked_at` | Internal |

**Owner token delivery** — returned in `owner_token` field of every native API create response; shown once in web UI with copy button; saved to `localStorage` in browser; never logged.

### Trust boundaries & external services

| Boundary | Trust | Notes |
|----------|-------|-------|
| Embedded SQLite | Trusted — local disk only | No network-accessible database |
| Incoming HTTP requests | **Untrusted** | Paste size capped at HTTP layer before reading |
| Paste content | **Untrusted** | Stored as opaque text; never executed; HTML-escaped in all web views |
| Multipart file uploads | **Untrusted** | Content extracted as text only; filename used only for language detection |
| Owner token (inbound) | **Untrusted** | Hashed before comparison; constant-time compare enforced |
| Operator token (inbound) | **Untrusted** | Hashed; constant-time compare against cached hash |
| Compat delete token (inbound) | **Untrusted** | Hashed before comparison |
| GitHub Releases API | **Untrusted network source** | Used by self-update; SHA-256 checksum verified before applying the downloaded binary; update is operator-initiated, not automatic |
| Tor binary (host-installed) | Trusted — operator-controlled binary | Forked as subprocess via `github.com/cretz/bine`; non-fatal when absent |
| SMTP server (operator-configured) | Semi-trusted — operator-controlled | Used for email notifications; credentials in `server.yml`; silently disabled when unconfigured |
| GeoIP CDN (jsDelivr) | Semi-trusted network source | GeoLite2-Country database downloaded by `geoip_update` scheduled task over HTTPS; no separate checksum beyond HTTPS; fail-open on download error |

| `forksmgr/lcomrade-lenpaste` (protocol reference) | **Untrusted source** — wire protocol spec consulted at development time; no code imported at runtime | Failure mode: compat layer continues to function even if the upstream fork is deleted or unmaintained; the protocol is implemented independently from the published spec |

### Threat model & abuse cases

**Primary assets:** service availability; disk space; paste integrity (preventing unauthorized deletion).

**Attacker / abuser goals:**

| Threat | Required defense |
|--------|-----------------|
| DoS via high-rate paste creation to exhaust disk | Rate limiting on create endpoint |
| XSS via paste content rendered in browser | All paste content HTML-escaped; Chroma output is sanitized HTML |
| Owner token brute-force to delete others' pastes | `crypto/rand` generation; SHA-256 storage; constant-time comparison |
| Compat delete-token brute-force | Same protections as owner token |
| Operator / metrics token timing attack | `crypto/subtle.ConstantTimeCompare` for all token and hash comparisons — plain `==` is forbidden |
| Storing illegal or abusive content | Operator responsibility; acceptable-use policy via `server.yml`; country blocklist for geographic restrictions |
| Large paste upload to exhaust memory | Maximum paste size enforced at HTTP body layer before reading into memory |
| Malicious backup restore (corrupted or tampered archive) | `VerifyBackup()` checks `sha256_manifest.txt` before any extraction; restore aborts if verification fails |
| Backup key compromise via weak derivation | Backup encryption password derives key via Argon2id (not bcrypt, not PBKDF2) |
| Unauthorized service install / data purge | `Install()` and `Uninstall()` require root privilege; `Uninstall()` requires interactive `[y/N]` confirmation |
| Self-update supply-chain attack (tampered release binary) | SHA-256 checksum verified against the release asset list before `os.Rename`; update aborted on mismatch |
| Country-based abuse or legal/regulatory blocking requirement | Country denylist via GeoIP middleware; allowlist mode also supported; bypassed for RFC1918 and loopback |

### Security decisions & exceptions

- **No authentication on any native endpoint:** intentional — fully public anonymous service.
- **`api_dev_key` and `api_user_key` silently ignored in compat layer:** intentional — compatibility shim; these fields have no meaning on a keyless public instance.
- **`POST /api/api_login.php` returns `"ANONYMOUS"`:** intentional — allows pastebin.com API scripts to proceed without modification.
- **Auth web routes redirect to `/` rather than 404:** intentional — scripts probing `/login` should not hard-fail.
- **Owner token reuse across pastes:** intentional — one token may authorize deletion of multiple pastes created by the same caller; the server validates the token is active before linking it to a new paste.
- **Owner token stored as SHA-256 hash only:** intentional — raw token shown once at creation; loss means the paste cannot be deleted early (compat-created pastes are unaffected, they use their own delete-token mechanism).
- **Compat layer uses separate delete-token mechanism (paste row `delete_token_hash`):** intentional — compat protocols have their own delete conventions incompatible with the owner token system; the two systems must not be mixed.
- **stikked `apikey` / dpaste keyless / hastebin keyless creation accepted without auth:** intentional — these protocols are keyless on a public instance; any supplied API key is accepted and ignored so existing scripts proceed without modification.
- **`POST /` is a multiplexed dispatcher for the curl-upload family:** intentional — sprunge.us (`sprunge`), ix.io (`f:1`), and 0x0.st (multipart `file`) all upload to the bare root path with mutually exclusive field names; the handler dispatches on the present field and falls through to the native paste-create handler when none match, so native form posts are unaffected.
- **curl-upload family returns raw-content URLs (`{base}/raw/{id}`):** intentional — these tools expect the returned URL to serve content directly (for piping back into a shell), unlike stikked/dpaste which return human view URLs.
- **0x0.st `X-Token` header returned on create:** intentional — mirrors the 0x0.st management-token convention; the value is the paste's compat delete token, consistent with the separate compat delete-token mechanism above.
- **Compat request bodies capped at 10 MiB (`maxCompatBody`):** intentional — raw-body compat creators (hastebin) read the full body into memory; the cap bounds memory use and is enforced via `http.MaxBytesReader`.
- **All token and hash comparisons use `crypto/subtle.ConstantTimeCompare`:** intentional — prevents timing-based side-channel attacks on owner tokens, operator tokens, compat delete tokens, and metrics bearer tokens; plain `==` is never used for security-sensitive comparisons.
- **Backup encryption uses Argon2id (not bcrypt, not PBKDF2):** intentional — Argon2id is memory-hard, resists GPU brute-force; bcrypt is forbidden for this use case per spec.
- **`VerifyBackup()` is required before restore:** intentional — ensures backup archive integrity via `sha256_manifest.txt`; restore aborts on any mismatch rather than silently applying a corrupt or tampered archive.
- **`Install()` and `Uninstall()` require root privilege:** intentional — writing systemd units, launchd plists, and SCM entries requires elevated access; both commands check `isPrivileged()` and exit with an error if not root/admin.
- **`Uninstall()` requires interactive `[y/N]` confirmation:** intentional — destructive operation; cannot be triggered accidentally or by a non-interactive process without explicit acknowledgment.
- **Self-update uses atomic `os.Rename` after SHA-256 verification:** intentional — prevents a partially-written binary from replacing the running binary; aborts the update on checksum mismatch before `os.Rename` is called.
- **Paste content not encrypted at rest:** intentional for a public service; operators requiring encryption should use full-disk encryption at the host level.
- **`Access-Control-Allow-Origin: *` on all responses:** intentional — public API designed for cross-origin browser use.
- **Single SQLite instance, no cluster:** intentional — single-binary deployment; no external database dependency; see non-goals.
- **GeoIP loaded from local database file, not queried per-request to an external API:** intentional — avoids network latency on every request, preserves availability when CDN is unreachable, and prevents data leakage of client IPs to third parties.
- **Container runs as root:** intentional — the container binds on port 80 (< 1024), which requires root or `CAP_NET_BIND_SERVICE`. Per PART 26, the exception applies; no non-root user is created in the runtime stage. Operators who wish to use an unprivileged port should set `PORT` and map externally, then add a non-root USER in a derived image.
- **termbin port 9999 binds all interfaces in production compose (no `172.17.0.1:` prefix):** intentional — the PART 26 bridge-IP rule exists so a reverse proxy handles external HTTP traffic, but termbin/fiche is a raw-TCP protocol that an HTTP reverse proxy cannot front; netcat clients must reach the listener directly, so the mapping is `${TERMBIN_PORT:-9999}:9999`. Only the HTTP port keeps the `172.17.0.1:` bridge binding. Operators can firewall or disable it via `TERMBIN_ENABLED=false`.
