## Project description

Pastebin is a full-stack Go web application for creating and sharing text snippets anonymously. It is a drop-in replacement for pastebin.com, microbin, and lenpaste — existing scripts, CLIs, and integrations targeting any of those services work against this server without modification. Submit text or upload a file and receive a short shareable URL. Pastes support server-side syntax highlighting via Chroma, a wide range of expiry durations, burn-after-N-reads deletion, and a one-time delete token returned at creation. No user accounts. Fully public. All paste data is stored in an embedded SQLite database. Deployed as a single self-contained static binary.

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
- Syntax highlighting server-side via Chroma — no client-side JS library required
- Language auto-detection from file extension on upload; manual selection otherwise
- Expiry options: `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never` (default), or a custom duration in seconds
- **Burn after N reads**: paste is permanently deleted once its view count reaches a user-set threshold (1–9999); `0` = disabled (default). Shown as a separate input alongside expiry.
- **Visibility**: public (listed in recent pastes) or unlisted (URL-only, not listed)
- **Delete token**: a cryptographically random token returned once in the creation response; stored as SHA-256 hash; presenter must supply it to delete the paste before natural expiry
- Raw paste view at `/raw/{id}` and `/{id}/raw` — plain text
- Download at `/dl/{id}` — attachment
- Embedded view at `/emb/{id}` — iframe-embeddable
- View count tracking
- Automatic background cleanup of expired and burned pastes
- Full web frontend (server-side Go templates, dark/light/auto theme, PWA, mobile-first)
- Server pages: `/server/about`, `/server/help`, `/server/healthz`, `/server/privacy`, `/server/terms`
- CLI client (`pastebin-cli`) — fully spec-compliant; see CLI section
- OpenAPI/Swagger docs at `/api/{api_version}/server/swagger`
- GraphQL at `/graphql` (read-only: query and list pastes; creation and deletion are REST-only)
- **Full route compatibility** with pastebin.com API, microbin, and lenpaste — see Route Compatibility section

**Non-goals:**
- No user accounts, registration, or login of any kind
- No admin web panel (server configured via `server.yml` only)
- No paste editing after creation (pastes are immutable)
- No password-protected pastes (microbin's per-paste encryption is not implemented)
- No paid tiers, no API keys, no rate-limited access tiers

### Roles & permissions

There are no user roles. All endpoints are public and require no authentication.

| Actor | Access |
|-------|--------|
| **Anonymous visitor (browser)** | Create pastes; view any public or unlisted paste by URL; browse recent public pastes; delete paste with token |
| **Anonymous API client (curl/CLI)** | Create pastes; retrieve paste content by ID; delete paste with token; list recent public pastes |
| **Server operator** | Configures server via `server.yml` only (max paste size, default expiry, retention policy, rate limits); no web management interface |

### Data model & sensitivity

**Paste record** (stored in embedded SQLite, no PII):

| Field | Type | Sensitivity |
|-------|------|-------------|
| `id` | string — short URL-safe identifier generated with `crypto/rand` | Public |
| `title` | string — optional paste title (blank → `Untitled`) | Public |
| `content` | string — paste body text | Public (user-controlled) |
| `language` | string — Chroma language identifier (blank → `text`) | Public |
| `is_public` | boolean — visible in recent pastes list | Public |
| `burn_after` | integer — delete after this many views; `0` = disabled | Public |
| `expires_at` | timestamp or null — auto-deletion deadline | Public |
| `delete_token` | string — SHA-256 hash of the creator's delete token | **Never returned after creation; verified on delete** |
| `views` | integer — view count | Public |
| `created_at` | timestamp | Public |
| `updated_at` | timestamp | Public |

**Creation response fields** (returned once; plaintext token never stored):

| Field | Notes |
|-------|-------|
| `id` | Short paste identifier |
| `url` | Full shareable URL |
| `delete_token` | Raw token — shown once only; lose it and the paste cannot be deleted early |

**Sensitivity note:** paste content is user-controlled and may contain sensitive data. The service makes no confidentiality guarantee. Paste content is never logged.

### Trust boundaries & external services

| Boundary | Trust level | Notes |
|----------|-------------|-------|
| Embedded SQLite database | Trusted — local disk only | No network-accessible database |
| Incoming HTTP requests | **Untrusted** | Paste content size-capped at HTTP layer before reading; all inputs validated |
| Paste content | **Untrusted** | Stored as opaque text; never executed or evaluated; HTML-escaped in all web views |
| Multipart file uploads | **Untrusted** | Content extracted as text only; filename used for language detection only; never executed |
| Delete token (inbound) | **Untrusted** | SHA-256 hashed before comparison; constant-time compare |

No external services called at runtime.

### API

#### Native REST API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/{api_version}/paste` | Create paste — returns `id`, `url`, `delete_token` |
| `GET` | `/api/{api_version}/paste/{id}` | Retrieve paste metadata and content as JSON |
| `DELETE` | `/api/{api_version}/paste/{id}` | Delete paste — `Authorization: Bearer <token>` or `?token=` |
| `GET` | `/api/{api_version}/paste/{id}/raw` | Raw paste text |
| `GET` | `/api/{api_version}/pastes` | List recent public pastes (paginated: `?page=`, `?limit=`) |
| `GET` | `/api/{api_version}/server/swagger` | OpenAPI / Swagger documentation |
| `GET` | `/server/healthz` | Health check |

**Create request body fields** (JSON or form):

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `content` | string | required | Paste body |
| `title` | string | `""` | Optional title |
| `language` | string | auto / `text` | Chroma language identifier |
| `is_public` | boolean | `true` | `false` = unlisted |
| `expires_in` | string | `never` | `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never`, or seconds as a string |
| `burn_after` | integer | `0` | Delete after N views; `0` = disabled; max `9999` |

#### GraphQL (`/graphql`)

- `query paste(id: ID!)` — retrieve a single paste
- `query pastes(page: Int, limit: Int)` — list recent public pastes
- Mutations are intentionally absent; use REST for create and delete

### Route Compatibility

This service is a drop-in replacement for pastebin.com, microbin, and lenpaste. All routes below are implemented in addition to the native API.

#### pastebin.com web routes

| Method | Path | Behaviour |
|--------|------|-----------|
| `GET` | `/{key}` | View paste (alias for `/{id}`) |
| `GET` | `/raw/{key}` | Raw paste text |
| `GET` | `/archive` | Alias for recent public pastes |
| `GET` | `/trends` | Alias for recent public pastes |
| `GET` | `/u/{username}` | Redirects `302` to `/` (no user profiles) |

#### pastebin.com API (`/api/api_post.php` and friends)

All `api_dev_key` and `api_user_key` fields are accepted but ignored — this is a public instance with no developer or user keys.

**`POST /api/api_login.php`** — returns the string `"ANONYMOUS"` with HTTP 200. Scripts that store this as `api_user_key` and pass it to subsequent calls will continue to work, since `api_user_key` is ignored.

**`POST /api/api_post.php`** — behaviour by `api_option`:

| `api_option` | Behaviour | Field mapping |
|---|---|---|
| `paste` | Create paste | `api_paste_code`→`content`, `api_paste_name`→`title`, `api_paste_format`→`language`, `api_paste_private` (`0`=public, `1`=unlisted, `2`=unlisted), `api_paste_expire_date`→`expires_in` (see table below) |
| `list` | Return recent public pastes as XML `<paste>` elements | `api_results_limit` honoured (1–1000) |
| `delete` | Delete paste using `api_paste_key` as ID; `api_user_key` is treated as the delete token if it is non-empty and non-`"ANONYMOUS"` | |
| `userdetails` | Returns a stub XML user record (username `"anonymous"`, no email, no avatar) | |

`api_paste_expire_date` mapping:

| pastebin.com value | Our `expires_in` |
|---|---|
| `N` | `never` |
| `10M` | `600` (10 min) |
| `1H` | `1h` |
| `1D` | `1d` |
| `1W` | `1w` |
| `2W` | `1209600` (14 days) |
| `1M` | `1m` |
| `6M` | `6m` |
| `1Y` | `1y` |

**`POST /api/api_raw.php`** — returns raw paste text for `api_paste_key`; `api_user_key` ignored.

#### microbin routes

| Method | Path | Behaviour |
|--------|------|-----------|
| `POST` | `/upload` | Create paste (multipart); field mapping below |
| `GET` | `/upload/{id}` | View paste (alias for `/{id}`) |
| `GET` | `/p/{id}` | View paste (short URL alias) |
| `GET` | `/raw/{id}` | Raw paste text |
| `GET` | `/url/{id}` | If paste content is a URL, redirect to it; otherwise view paste |
| `GET` | `/u/{id}` | Short redirect alias for `/url/{id}` |
| `GET` | `/file/{id}` | Download paste content as file (alias for `/dl/{id}`) |
| `GET` | `/qr/{id}` | QR code PNG for paste URL |
| `GET` | `/list` | List recent public pastes |
| `GET` | `/remove/{id}` | Show delete form (prompts for delete token) |
| `POST` | `/remove/{id}` | Delete paste — `password` field used as delete token |
| `GET` | `/guide` | Redirects to `/server/help` |

microbin `POST /upload` field mapping:

| microbin field | Our field |
|---|---|
| `content` | `content` |
| `privacy` (`public`/`readonly`/`secret`) | `is_public: true`; `private` → `is_public: false` |
| `expiration` | mapped to `expires_in` (see table below) |
| `burn_after` | `burn_after` (0 = disabled; any non-zero value honoured up to 9999) |
| `syntax_highlight` | `language` |
| `file` | file upload content |
| `plain_key`, `random_key`, `encrypted_random_key` | ignored (no encryption support) |
| `uploader_password` | ignored |

microbin `expiration` mapping:

| microbin value | Our `expires_in` |
|---|---|
| `1min` | `60` |
| `10min` | `600` |
| `1hour` | `1h` |
| `24hour` | `1d` |
| `3days` | `259200` |
| `1week` | `1w` |
| `1month` | `1m` |
| `6months` | `6m` |
| `1year` | `1y` |
| `2years` | `2y` |
| `4years` | `126144000` |
| `8years` | `252288000` |
| `16years` | `504576000` |
| `never` | `never` |

#### lenpaste routes

| Method | Path | Behaviour |
|--------|------|-----------|
| `POST` | `/` | Create paste (form); field mapping below; redirects to `/{id}` |
| `GET` | `/{id}` | View paste |
| `GET` | `/raw/{id}` | Raw paste text |
| `GET` | `/dl/{id}` | Download paste as file |
| `GET` | `/emb/{id}` | Iframe-embeddable paste view |
| `GET` | `/emb_help/` | Redirect to `/server/help` |
| `GET` | `/about` | Redirect to `/server/about` |
| `POST` | `/api/v1/new` | Create paste (form/JSON); returns `{"id":"..."}` |
| `GET` | `/api/v1/get` | Get paste JSON — `?id=`, `?openOneUse=true` |
| `GET` | `/api/v1/getServerInfo` | Server info JSON |

lenpaste `POST /` / `POST /api/v1/new` field mapping:

| lenpaste field | Our field |
|---|---|
| `body` | `content` |
| `title` | `title` |
| `syntax` | `language` |
| `expiration` | `expires_in` (seconds string, `"0"` → `never`) |
| `oneUse` | `burn_after: 1` when `"true"` |
| `lineEnd` | ignored |

`GET /api/v1/get` response mirrors lenpaste format:

```json
{
  "id": "abc12345",
  "title": "...",
  "body": "...",
  "syntax": "text",
  "createTime": 1700000000,
  "deleteTime": 0,
  "oneUse": false
}
```

When `burn_after == 1` and `?openOneUse=true` is not set, only `{"id":"...","oneUse":true}` is returned (body withheld, consistent with lenpaste behaviour).

`GET /api/v1/getServerInfo` response:

```json
{
  "version": "...",
  "titleMaxlength": 100,
  "bodyMaxlength": 10485760,
  "maxLifeTime": -1,
  "serverAbout": "...",
  "serverRules": "...",
  "adminName": "",
  "adminMail": "",
  "syntaxes": ["text", "go", "python", ...]
}
```

#### Auth route stub behaviour

These routes exist for compatibility with scripts that probe or hit them; no authentication is ever performed.

| Method | Path | Response |
|--------|------|----------|
| `GET` | `/login` | `302` → `/` |
| `GET` | `/register` | `302` → `/` |
| `POST` | `/login` | `302` → `/` |
| `POST` | `/register` | `302` → `/` |
| `POST` | `/logout` | `302` → `/` |
| `GET` | `/u/{username}` | `302` → `/` |
| `GET` | `/settings` | `302` → `/` |
| `GET` | `/auth/{id}` | `302` → `/{id}` (microbin password-gate; no encryption support) |
| `GET` | `/auth_raw/{id}` | `302` → `/raw/{id}` |
| `GET` | `/auth_remove_private/{id}` | `302` → `/remove/{id}` |

### CLI (`pastebin-cli`)

Fully spec-compliant with the AI.md CLI specification.

| Command | Description |
|---------|-------------|
| `pastebin-cli create [file]` | Create paste from stdin or file; prints URL and delete token |
| `pastebin-cli get <id>` | Fetch and print raw paste content |
| `pastebin-cli delete <id> <token>` | Delete paste using its delete token |
| `pastebin-cli list [--limit <n>]` | List recent public pastes |

**Flags for `create`:**

| Flag | Description |
|------|-------------|
| `--lang <lang>` | Chroma language identifier |
| `--expiry <duration>` | `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never`, or seconds |
| `--burn <n>` | Delete after N views (1–9999) |
| `--unlisted` | Create as unlisted |
| `--title <title>` | Optional paste title |

**Global flags:** `--server <url>` targets a custom server instance; `--json` for machine-readable output.

**Shell pipeline examples:**
```
cat file.go | pastebin-cli create --lang go
pastebin-cli create --burn 1 --expiry 1h secret.txt
pastebin-cli get abc12345
pastebin-cli delete abc12345 <delete-token>
```

### Web UI pages

| Page | Route(s) | Description |
|------|----------|-------------|
| Home / Create | `/` | Text area, language selector, expiry picker, burn-after input, visibility toggle |
| Paste view | `/{id}`, `/p/{id}`, `/upload/{id}` | Syntax-highlighted content; raw/download/embed links; expiry and view count; delete form |
| Raw view | `/raw/{id}`, `/{id}/raw` | Plain text |
| Download | `/dl/{id}`, `/file/{id}` | Download as attachment |
| Embedded | `/emb/{id}` | Minimal iframe-embeddable view |
| QR code | `/qr/{id}` | QR code PNG for the paste URL |
| Recent pastes | `/recent`, `/list`, `/archive` | Paginated public paste list |
| Delete | `/remove/{id}` | Delete form — prompts for delete token |
| Server pages | `/server/about`, `/server/help`, `/server/healthz`, `/server/privacy`, `/server/terms` | Standard info pages |

### Threat model & abuse cases

**Primary assets:** service availability; storage disk space.

**Attacker/abuser goals:**
- DoS via high-rate paste creation to exhaust disk
- XSS via paste content rendered in the browser
- Delete token brute-force to delete others' pastes
- Storing malicious or illegal content — operator responsibility to enforce acceptable-use policy

**Defenses:**
- Rate limiting on paste creation and delete endpoints
- Maximum paste size enforced at the HTTP body layer before reading into memory
- Paste content HTML-escaped in all web views; Chroma output is sanitized HTML
- `crypto/rand` for all ID and token generation
- Delete token stored as SHA-256 hash; constant-time comparison at verification
- Automatic background cleanup of expired and burned-out pastes
- No user accounts eliminates credential stuffing and privilege escalation entirely

### Security decisions & exceptions

- **No authentication on any endpoint**: intentional. Fully public anonymous paste service.
- **`api_dev_key` and `api_user_key` silently ignored**: intentional. Compatibility shim — these fields have no meaning on a keyless public instance.
- **`POST /api/api_login.php` returns `"ANONYMOUS"`**: intentional. Allows existing pastebin.com API scripts to proceed without modification; the returned value is never validated elsewhere.
- **Auth web routes redirect to `/` rather than 404**: intentional. Scripts and health-checks that probe `/login` should not hard-fail; a silent redirect is least surprising.
- **Delete token issued once and never stored in plaintext**: intentional. Only a SHA-256 hash persists. Loss of the token means the paste cannot be deleted before natural expiry.
- **Paste content not encrypted at rest**: intentional for a public service; operators requiring encryption should use full-disk encryption at the host level.
- **Chroma server-side highlighting only**: intentional. No client-side JS library; highlighting works without JavaScript.
- **All responses include `Access-Control-Allow-Origin: *`**: intentional. Public API designed for cross-origin browser use.
- **SQLite for storage**: intentional. Single-binary deployment; no external database dependency.
- **`crypto/rand` for all ID and token generation**: mandatory. `math/rand` must never be used for any paste ID or delete token.
