# Backend Rules (PART 9, 10, 11, 32)

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

## Client Binary (PART 32)
- Binary: `pastebin-cli`
- Commands: `create`, `get`, `delete`, `list`, `update`, `tui`, `completions`
- All requests send `User-Agent: pastebin/{version}` and `Accept-Language: {lang}`
- Config: `cli.yml` (same OS path logic as server, minus `/etc/`)
- Exit codes: 0=success, 1=general, 2=config, 3=connection, 4=auth, 5=not found, 64=usage
- TUI: bubbletea + bubbles + lipgloss (all required deps)
- Auto-update: download → SHA-256 verify → atomic `os.Rename` → `syscall.Exec` re-exec

---
For complete details, see AI.md PART 9, 10, 11, 32
