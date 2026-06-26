# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Health & Versioning (PART 13)
- `/server/healthz` — public health endpoint
- `/api/v1/server/healthz` — canonical versioned health
- `/api/healthz` — short alias
- `/api/v1/server/version` — version info
- `/healthz` — optional, gated by config
- Canonical field order in all health/version responses
- `/metrics` — internal-only (firewall/IP restriction + optional bearer token)

## API Structure (PART 14)
- Base: `/api/v1/`
- Success envelope: `{"ok":true,"data":{...}}`
- Error envelope: `{"ok":false,"error":"ERROR_CODE","message":"human readable"}`
- Canonical error codes with HTTP status mapping
- Content negotiation: `Accept: application/json` vs `text/html`
- Client-type detection: API clients get JSON; browser clients get HTML
- `/api/swagger` — Swagger UI (served directly, NOT a redirect)
- `/api/graphql` — GraphQL endpoint (served directly, NOT a redirect)
- `/server/docs/graphql` — GraphiQL explorer
- OpenAPI spec MUST include GraphQL endpoint annotation
- GraphiQL UI must POST to `/api/graphql` (canonical alias)

## REST API Routes
- `POST /api/v1/pastes` — create paste
- `GET /api/v1/pastes/{id}` — get paste
- `DELETE /api/v1/pastes/{id}` — delete paste (owner token required)
- `GET /api/v1/pastes` — list pastes (operator token required)
- `GET /raw/{id}` — raw paste content (no envelope, plain text)

## SSL/TLS & Let's Encrypt (PART 15)
- Port 80/443 → auto Let's Encrypt via `autocert`
- 4-priority manual cert lookup
- Staging support (`LE_STAGING=true`)
- HTTP-01 challenge server
- Self-signed fallback ONLY for overlay networks (.onion/.i2p)
- Clearnet: error when no cert and LE disabled (never silently self-sign)
- TLS config: `server.tls.*` keys in config (NOT `--ssl-*` CLI flags)
- Dual-port rule: `--port 80,443` enables dual HTTP+HTTPS

---
For complete details, see AI.md PART 13, 14, 15
