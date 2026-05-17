# Security

## Design Principles

- **No user accounts**: eliminates credential stuffing and privilege escalation entirely
- **Public anonymous service**: all endpoints are public; no authentication required
- **Fail closed**: invalid or missing tokens deny access; no silent allows
- **Crypto/rand everywhere**: paste IDs and delete tokens use `crypto/rand`, never `math/rand`

## Delete Tokens

- Generated using `crypto/rand` (32 bytes → 64-char hex string)
- Stored as SHA-256 hash only — plaintext never persisted
- Returned **once only** at paste creation
- Verified using constant-time comparison to prevent timing attacks
- Loss of token means paste cannot be deleted before natural expiry

## Paste Content

- HTML-escaped in all web views
- Chroma highlighting output is sanitized HTML
- Never executed or evaluated
- Size-capped at the HTTP layer before reading into memory (10MB max)
- Never logged

## Transport

- `Access-Control-Allow-Origin: *` on all responses (public API by design)
- HTTPS strongly recommended in production (use a reverse proxy or `--tls`)

## Rate Limiting

- Default: 30 paste creations per IP per minute
- Configurable via `rate_limit.create_per_minute` in `server.yml`
- Rate limiting protects the server against abuse — it is not a usage cap

## Reporting Vulnerabilities

See [SECURITY.md](https://github.com/apimgr/pastebin/blob/main/.github/SECURITY.md) for the responsible disclosure process.
Do NOT file public issues for security vulnerabilities.
