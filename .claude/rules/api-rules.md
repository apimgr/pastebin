# API Rules (PART 13, 14, 15)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Health & Versioning (PART 13)
- `/server/healthz` — public health endpoint (HTML/JSON/text via content negotiation)
- `/api/{api_version}/server/healthz` — canonical versioned health (JSON)
- `/api/healthz` — direct alias to current `{api_version}` (NOT a redirect)
- `/healthz` — optional root alias, gated by `server.healthz.root.enabled`
- No separate version-info endpoint — version fields live inside the health response
- Canonical field order in all health/version responses
- `/metrics` — internal-only (firewall/IP restriction + optional bearer token)

## API Structure (PART 14)
- Base: `/api/{api_version}/` — never hardcode `v1`; use `APIBasePath()`
- Success envelope: `{"ok":true,"data":{...}}`
- Error envelope: `{"ok":false,"error":"ERROR_CODE","message":"human readable"}`
- Canonical error codes with HTTP status mapping
- Content negotiation: `Accept: application/json` vs `text/html`
- Client-type detection: API clients get JSON; browser clients get HTML
- `/server/docs/swagger` — Swagger UI (interactive; fetches spec from `/api/swagger`)
- `/api/swagger` — OpenAPI JSON spec, direct alias for current `{api_version}` (NOT a redirect)
- `/server/docs/graphql` — GraphiQL explorer; POSTs to `/api/graphql`
- `/api/graphql` — GraphQL endpoint, direct alias for current `{api_version}` (NOT a redirect)
- Old paths removed: `/openapi`, `/openapi.json`, root `/graphql` no longer served

## REST API Routes
- `POST /api/{api_version}/pastes` — create paste
- `GET /api/{api_version}/pastes/{id}` — get paste
- `DELETE /api/{api_version}/pastes/{id}` — delete paste (owner token required)
- `GET /api/{api_version}/pastes` — list pastes (operator token required)
- `GET /raw/{id}` — raw paste content (no envelope, plain text)

## SSL/TLS & Let's Encrypt (PART 15)
- ALL projects MUST have built-in Let's Encrypt support
- 3 challenge types: HTTP-01 (port 80, default), TLS-ALPN-01 (port 443), DNS-01 (wildcard, all lego providers)
- 4-priority cert lookup: system certbot → app-managed LE → local/user certs
- Staging: `server.ssl.letsencrypt.staging` config key (config file, NOT an env var)
- App-managed LE certs auto-renew 7 days before expiry (daily check)
- Self-signed fallback ONLY for overlay networks (.onion/.i2p)
- TLS config: `server.tls.*` keys in config (NOT `--ssl-*` CLI flags)
- Dual-port rule: `--port 80,443` enables dual HTTP+HTTPS

---
For complete details, see AI.md PART 13, 14, 15
