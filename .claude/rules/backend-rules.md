# Backend Rules (PART 9, 10, 11, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Error Handling (PART 9)
- NEVER `log.Fatal` outside `main` package
- `os.Exit` only in `main` or legitimate fork/relaunch points
- All errors wrapped with context: `fmt.Errorf("operation: %w", err)`
- HTTP errors use canonical envelope: `{"ok":false,"error":"ERROR_CODE","message":"human readable"}`
- 5xx errors: log full detail server-side; return generic message to client (never leak internals)

## Caching (PART 9)
- Cache key prefix: `pastebin:` (lowercase, colon-separated)
- Backends: memory / valkey / redis / none
- Cache miss is non-fatal — always fall through to DB
- TTL fallback when backend unavailable
- Valkey/Redis: local cache ONLY — not used for clustering

## Database (PART 10)
- SQLite default: `{data_dir}/server.db`
- ALL queries MUST be parameterized with `?` — NEVER `fmt.Sprintf` into SQL
- ZERO `SELECT *` — always name columns explicitly
- Idempotent schema: `CREATE TABLE IF NOT EXISTS`, guarded `ALTER TABLE`
- Connection pool + per-query context timeouts
- libsql/Turso remote implementation available as alternative backend

## Security & Logging (PART 11)
- NEVER bcrypt, MD5, SHA-1 for passwords/hashes
- Config/backup passwords: **Argon2id** (NEVER bcrypt)
- API tokens stored as **SHA-256 hex** only — raw token NEVER persisted
- Token prefix: `tok_` on all owner and operator tokens
- Constant-time comparison: `crypto/subtle.ConstantTimeCompare` for ALL token/hash checks
- NEVER log raw token values — log token prefix only (e.g., `tok_abc...` → first 8 chars)
- Bearer token auth on `/metrics`: MUST use constant-time compare
- CSRF protection on all state-mutating endpoints
- XSS: `html/template` auto-escaping; NEVER `template.HTML()` on user content
- Path traversal: validate all file paths against allowed base dirs
- Input validation: server validates EVERYTHING — never trust client

## Tor Hidden Service (PART 31)
- ALL projects MUST have built-in Tor hidden service support — always enabled if the Tor binary is found, no enable/disable toggle
- External Tor binary via `github.com/cretz/bine` — keeps `CGO_ENABLED=0` compatibility (no embedded Tor)
- Server binary owns the Tor process lifecycle (start/stop/manage); hidden service maps `.onion:80` → `localhost:{server_port}`
- `HiddenServiceVersion 3` — v3 onion addresses (56 chars, ed25519) via `ADD_ONION`
- Control port: `127.0.0.1:auto` on all OSes — NEVER hardcode a control port, NEVER use default Tor ports (9050/9051)
- Completely isolated from any system Tor installation — app handles all dirs/files/permissions/torrc generation
- `SafeLogging` enabled — scrubs sensitive info from Tor logs
- Optional outbound network mode (`server.tor.use_network`) routes the server's own outbound HTTP requests through Tor's SOCKS5 proxy — separate from hidden-service hosting, default `false`
- Config keys: `server.tor.{binary,use_network,max_circuits,circuit_timeout,bootstrap_timeout,safe_logging,max_streams_per_circuit,close_circuit_on_stream_limit,bandwidth_rate,bandwidth_burst,max_monthly_bandwidth,num_intro_points,virtual_port}`
- Trust chain: Tor detection is priority 0 in FQDN resolution (PART 12) — evaluated before reverse proxy headers, always trusted, no IP check required

---
For complete details, see AI.md PART 9, 10, 11, 31
