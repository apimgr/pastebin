# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- NEVER add sub-routes under `/server/healthz` (no `/server/healthz/db`, no `/server/healthz/**`)
- NEVER expose sensitive data in `/server/healthz`: DB connection strings, API keys/tokens, passwords, internal IPs, file paths, env vars, config contents, SMTP host, operator emails, stack traces, internal endpoints
- NEVER keep legacy/removed endpoints from this project "for compatibility" — DELETE them immediately, no redirects, no deprecation period, no backwards-compat shims
- NEVER layer new routes on top of old code — migrate the implementation to the new route tree and delete the old one
- NEVER use a singular resource name in a route (`/api/{api_version}/item` is wrong; use `/items`)
- NEVER use uppercase or underscores in routes — lowercase + hyphens only (`api-keys`, not `API_Keys` or `api_keys`)
- NEVER end a route with a trailing slash
- NEVER use verbs in routes (`getItems`) — routes are nouns, the HTTP method is the verb
- NEVER do client-side routing (SPA), client-side rendering (React/Vue), client-side data fetching for initial page load, or put business logic in JavaScript
- NEVER require JavaScript for a core feature — must degrade to full functionality without JS
- NEVER manually edit generated `openapi.json` or GraphQL schema files — both are build-time generated from code, never by hand
- NEVER let Swagger/GraphQL drift from the actual API or from each other
- NEVER put swagger/graphql implementation files in project root — always `src/swagger/` and `src/graphql/`
- NEVER emit a bare JSON array at API root — always wrap in `{ "data": [...] }`
- NEVER invent a custom error shape per endpoint — one canonical `{ok:false, error, message, details}` envelope everywhere
- NEVER duplicate the HTTP status code inside the JSON body (no `status` field in error responses)
- NEVER redirect an unversioned API alias (`/api/swagger`, `/api/graphql`, `/api/healthz`) to its versioned equivalent — mount the SAME handler at both paths
- NEVER set `DOMAIN` to an overlay network address (`.onion`, `.i2p`, `.exit`) — those are app-managed
- NEVER put icons/ASCII art/emojis in log output — logs are always raw text (banners on stdout may use them per terminal width)

## CRITICAL - ALWAYS DO
- ALWAYS version API routes as `/api/{api_version}/...` — never hardcode `v1`, use `APIBasePath()`
- ALWAYS use plural nouns for resources (`/items`, `/posts`)
- ALWAYS use lowercase + hyphens for multi-word routes
- ALWAYS prefer path params for resource identity, query params for filtering/sorting/pagination
- ALWAYS give every `/api/{api_version}/{resource}` a matching frontend route when it's a user-facing feature (CRUD parity)
- ALWAYS make frontend forms/CRUD/navigation work fully without JavaScript (progressive enhancement only)
- ALWAYS do validation, business logic, transformation, formatting, pagination, search, and rendering on the server
- ALWAYS return JSON indented 2 spaces with a single trailing `\n` for every `application/json` response
- ALWAYS end every text/HTML/YAML/JSON response and file with exactly one trailing newline
- ALWAYS implement all three API types: REST (primary), Swagger/OpenAPI, GraphQL — generated from code, kept in sync, JSON only (no YAML) for OpenAPI
- ALWAYS style Swagger UI and GraphiQL to match the project-wide light/dark/auto theme system
- ALWAYS support the standard content negotiation: `.txt` extension, `Accept` header, and User-Agent based client detection (our CLI → JSON; text browsers → no-JS HTML; HTTP tools → HTML2TextConverter formatted text; browsers → HTML+JS)
- ALWAYS accept an auth token from any supported header (PART 8 priority order) plus `?token=` query param on protected API endpoints
- ALWAYS implement built-in Let's Encrypt with HTTP-01, TLS-ALPN-01, and DNS-01 challenge support
- ALWAYS resolve `{fqdn}` using the documented priority order (reverse proxy headers → `DOMAIN` → `os.Hostname()` → `$HOSTNAME` → public IPv6 → public IPv4 → `localhost`)
- ALWAYS validate DNS-01 provider credentials on startup and before each certificate request; store encrypted at rest

## Key Rules Summary

### Health endpoint (`/server/healthz`, PART 13)
- Routes: `/server/healthz` (frontend, content-negotiated), optional `/healthz` root alias (only if `server.healthz.root.enabled: true`, same handler, no redirect), `/api/{api_version}/server/healthz` (JSON default), `/api/healthz` (unversioned alias).
- Canonical field order: `project → status → version/build → uptime/mode/timestamp → features → checks → stats → app-specific`.
- `checks.*` values are `"ok"`/`"error"` only — never leak connection strings or paths.
- Global structure is comprehensive; apps extend `features.*`, `stats.*`, `checks.*`, or add a new top-level field — never reinvent it.

### Versioning (PART 13)
- SemVer for stable releases: `MAJOR.MINOR.PATCH`, start at `1.0.0`, no `v` prefix in the version string (git tags do get `v`), no leading zeros.
- Beta format: `YYYYMMDDHHMMSS-beta`; daily format: `YYYYMMDDHHMMSS`.
- Version source priority: `release.txt` → git tag → `dev`.

### Route structure (PART 14)
| Rule | Correct |
|------|---------|
| Versioned | `/api/{api_version}/items` |
| Plural | `/api/{api_version}/items` |
| Lowercase+hyphen | `/api/{api_version}/api-keys` |
| No trailing slash | `/api/{api_version}/items` |
| Nouns only | `GET /api/{api_version}/items` |

Route scopes: `/server/*` (server-owned, no ID) and project routes at `/*` (open, project-specific). Nested resource IDs use descriptive names (`item_id`, `comment_id`).

### Content negotiation priority (PART 14)
- API routes (`/api/**`): `.txt` extension → `Accept: text/plain` → non-interactive client (curl/wget) → default JSON.
- Frontend routes (`/**`): `Accept: text/html` → `Accept: text/plain` → User-Agent browser detection → CLI/curl (text) → default HTML.
- Client types: our CLI (`{project_name}-cli` UA) gets JSON and renders its own TUI/GUI; text browsers (lynx/w3m/links/elinks) get no-JS HTML; HTTP tools (curl/wget/httpie/no UA) get `HTML2TextConverter()` formatted text; regular browsers get HTML+JS.

### Response envelopes (PART 14)
- Single item: return directly, no wrapper.
- Action (create/update/delete): `{ "ok": true, "data": { "id": ..., "message": "..." } }`.
- Error (all 4xx/5xx): `{ "ok": false, "error": "CODE", "message": "...", "details": {} }` — the one canonical shape, see also PART 9/11.
- Pagination default limit: 250. Shape: `{ "data": [...], "pagination": { "page", "limit", "total", "pages" } }`.

### Standard endpoints (PART 14)
`/`, `/server/healthz`, optional `/healthz`, `/server/docs/swagger`, `/server/docs/graphql`, `/metrics`, `/api/autodiscover`, `/api/swagger`, `/api/graphql`, `/api/healthz`, `/api/{api_version}/server/swagger`, `/api/{api_version}/server/graphql`, `/api/{api_version}/server/healthz`, `/api/{api_version}/server/*`.
Unversioned aliases (`/api/swagger`, `/api/graphql`, `/api/healthz`) are served directly by the same handler as their versioned route — never a redirect (breaks POST semantics, doubles caching, adds latency).

### External API compatibility (PART 14)
| Compatibility target | Action |
|---|---|
| Full protocol (Matrix, XMPP, ActivityPub) | Implement complete spec |
| RFC-defined standard (WebDAV, CalDAV) | Follow RFC exactly |
| Simple web service (pastebin.com-style) | Create/init endpoint only, skip the rest |
| Unclear | Ask the user |

### TLS/Let's Encrypt (PART 15)
- Challenge types: HTTP-01 (port 80, default), TLS-ALPN-01 (port 443), DNS-01 (wildcard, any DNS provider via `server.tls.dns_provider` + encrypted `dns_credentials.*`).
- `{fqdn}` resolution order: reverse proxy headers (`X-Forwarded-Host`, etc.) → `DOMAIN` env var (comma-separated, first = primary) → `os.Hostname()` → `$HOSTNAME` → public IPv6 → public IPv4 → `localhost`.
- Dev TLDs (`{project_name}`, `.local`, `.test`, `.example`, `.invalid`, `.lan`, `.internal`, etc.) fall back to displaying the global IP instead of the dev TLD.
- Startup banner adapts to terminal width (full ASCII+icons ≥80 cols, icons only 60-79, minimal 40-59, single line <40); plain text (no emojis) when `NO_COLOR`/`TERM=dumb`. Logs themselves are always plain text regardless of banner style.

For complete details, see AI.md PART 13, 14, 15.
