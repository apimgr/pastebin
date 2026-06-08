## Project description

Pastebin is a full-stack Go web application for creating and sharing text snippets anonymously. It is a drop-in replacement for pastebin.com, microbin, and lenpaste (fork: forksmgr/lcomrade-lenpaste) — existing scripts, CLIs, and integrations targeting any of those services work against this server without modification. Users submit text or upload a file and receive a short shareable URL. No user accounts. Fully public. Deployed as a single self-contained static binary. The companion CLI (`pastebin-cli`) provides full API access from the terminal.

## Project variables

project_name: pastebin
project_org: apimgr
internal_name: pastebin
internal_org: apimgr
app_name: Pastebin
repo: https://github.com/apimgr/pastebin
license: MIT
binary: pastebin
client_binary: pastebin-cli

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
- Drop-in compat layer for pastebin.com, microbin, and lenpaste — compat routes use their own delete-token mechanism (stored in paste row) separate from the owner token system
- Two-tier operator token: server operator may set a `server.token` in `server.yml`; this operator token allows deleting any paste unconditionally

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

| `forksmgr/lcomrade-lenpaste` (protocol reference) | **Untrusted source** — wire protocol spec consulted at development time; no code imported at runtime | Failure mode: compat layer continues to function even if the upstream fork is deleted or unmaintained; the protocol is implemented independently from the published spec |

No external services called at runtime (GeoIP database fetched by scheduled task on operator's initiative, not on request path).

### Threat model & abuse cases

**Primary assets:** service availability; disk space; paste integrity (preventing unauthorized deletion).

**Attacker / abuser goals:**

| Threat | Required defense |
|--------|-----------------|
| DoS via high-rate paste creation to exhaust disk | Rate limiting on create endpoint |
| XSS via paste content rendered in browser | All paste content HTML-escaped; Chroma output is sanitized HTML |
| Owner token brute-force to delete others' pastes | `crypto/rand` generation; SHA-256 storage; constant-time comparison |
| Compat delete-token brute-force | Same protections as owner token |
| Storing illegal or abusive content | Operator responsibility; acceptable-use policy via `server.yml` |
| Large paste upload to exhaust memory | Maximum paste size enforced at HTTP body layer before reading into memory |

### Security decisions & exceptions

- **No authentication on any native endpoint:** intentional — fully public anonymous service.
- **`api_dev_key` and `api_user_key` silently ignored in compat layer:** intentional — compatibility shim; these fields have no meaning on a keyless public instance.
- **`POST /api/api_login.php` returns `"ANONYMOUS"`:** intentional — allows pastebin.com API scripts to proceed without modification.
- **Auth web routes redirect to `/` rather than 404:** intentional — scripts probing `/login` should not hard-fail.
- **Owner token reuse across pastes:** intentional — one token may authorize deletion of multiple pastes created by the same caller; the server validates the token is active before linking it to a new paste.
- **Owner token stored as SHA-256 hash only:** intentional — raw token shown once at creation; loss means the paste cannot be deleted early (compat-created pastes are unaffected, they use their own delete-token mechanism).
- **Compat layer uses separate delete-token mechanism (paste row `delete_token_hash`):** intentional — compat protocols have their own delete conventions incompatible with the owner token system; the two systems must not be mixed.
- **Paste content not encrypted at rest:** intentional for a public service; operators requiring encryption should use full-disk encryption at the host level.
- **`Access-Control-Allow-Origin: *` on all responses:** intentional — public API designed for cross-origin browser use.
- **Single SQLite instance, no cluster:** intentional — single-binary deployment; no external database dependency; see non-goals.
- **Container runs as root:** intentional — the container binds on port 80 (< 1024), which requires root or `CAP_NET_BIND_SERVICE`. Per PART 26, the exception applies; no non-root user is created in the runtime stage. Operators who wish to use an unprivileged port should set `PORT` and map externally, then add a non-root USER in a derived image.
