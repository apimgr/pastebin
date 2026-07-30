# Binary Rules (PART 7, 8, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use CGO (`CGO_ENABLED=0` always; pure Go only)
- Embed security databases (GeoIP, blocklists, CVE, Trivy) in the binary — download on first run, update via scheduler
- Hand-roll argument parsing for the server binary (single-command → stdlib `flag`; never manual `switch`/`os.Args` loops)
- Use `cobra`/`viper` for the server binary — those are for `pastebin-cli` only (multi-command client)
- Create a PID file inside a container (`isContainer()` true → skip PID file entirely)
- Re-resolve `~`/`$HOME` after privilege drop — service account HOME points at `{data_dir}`; a late lookup nests user-style paths inside the system data dir
- Hardcode `localhost`, `127.0.0.1`, `0.0.0.0`, `[::1]`, or any static host/IP/port anywhere in project code or output
- Display bare paths like `GET /api/` without the full resolved URL
- Include `:80` (HTTP) or `:443` (HTTPS) in displayed URLs
- Store `server.token` (the operator token) in the `api_tokens` DB table — it lives only in `server.yml`, hashed in memory
- Store any API token in plaintext — SHA-256 hash only, raw token shown once
- Use `strconv.ParseBool()` for boolean flags/config — always `config.ParseBool()` / `config.IsTruthy()`
- Implement manual UI-mode flags (`--tui`, `--gui`, `--cli`) — mode is auto-detected from environment
- Attempt GUI over SSH/mosh — remote sessions always use TUI or CLI
- Use OS system directories for CLI runtime state — client config/data/cache/logs always in the invoking user's XDG/profile dirs, even if the binary runs as root
- Let CLI help (`--help` at any level, any subcommand) trigger `sudo` or require root/admin
- Force ANSI colors, emojis, cursor movement, or TUI when `TERM=dumb` — force CLI mode, plain text, `[OK]`/`[ERROR]`/`[WARN]`

## CRITICAL - ALWAYS DO
- Ship server + client as a **single static binary** each, assets embedded via Go `embed`
- Auto-create `server.yml` with defaults and required directories on first run; show banner with URLs + version
- Handle SIGTERM/SIGINT/SIGHUP properly; always remove PID file in signal handlers
- Detect display environment (GUI/TUI/CLI/Headless) on every binary; server only ever shows status banners — CLI is the only binary with GUI/TUI/setup wizard
- Respect `NO_COLOR` (https://no-color.org) on all binaries
- Detect stale PID files (crash/kill -9 leaves them behind)
- Lock directory mode (system vs user) ONCE from EUID at startup, before any privilege drop; cache for process lifetime
- Detect FQDN/proto/port dynamically per request (reverse-proxy aware); strip default ports
- Hash tokens with SHA-256, compare with `subtle.ConstantTimeCompare`
- Give browser clients dual token delivery: shown once in response AND `HttpOnly+Secure+SameSite=Strict` cookie named `{project_name}_owner_token_XXXXXX`
- Restrict `/api/...` routes to `Authorization` header only (never cookies); web management forms may fall back to the owner-token cookie
- Make `--debug` a required flag on all binaries (not optional)
- Validate CLI config before saving; never save invalid values; never clear existing valid values
- Keep client (`pastebin-cli`) runtime state user-scoped even when installed system-wide

## Key Rules Summary

### Binary naming
| Role | Binary | Notes |
|------|--------|-------|
| server | `pastebin` | daemon/service, single command (no subcommands) |
| client | `pastebin-cli` | required companion, CLI/TUI/GUI, multi-command (cobra/viper) |

Internal identifiers always use `pastebin` (hardcoded, never changes); displayed binary name must reflect the actual (possibly renamed) executable.

### Display mode hierarchy
| Mode | When | Binaries |
|------|------|----------|
| GUI | native display, no SSH/mosh | CLI only |
| TUI | TTY / SSH / mosh / screen / tmux | CLI (default), server (status banner only) |
| CLI | command given or piped output | both |
| Headless | no display, no TTY | server (default/daemon) |

`TERM=dumb` forces CLI mode on all binaries — no ANSI, no emojis, no spinners/progress bars (use text), ASCII table borders.

### Server binary commands (single-command, stdlib `flag`)
| Flag | Purpose |
|------|---------|
| `--help`, `--version` | anyone can run |
| `--completion [shell]`, `--init [shell]` | shell integration |
| `--mode` | application mode |
| `--config`, `--data`, `--cache`, `--log`, `--backup` | directory overrides (auto-create) |
| `--pidfile` | PID file path |
| `--listen`, `--port`, `--baseurl` | network/routing |
| `--status` | health check, exit 0=healthy/1=unhealthy |
| `--daemon` | detach from terminal (ignored under systemd/launchd/runit/s6/container = always foreground; ignored under SysV/rc.d = always daemonize) |
| `--debug` | required, verbose logging + debug endpoints |
| `--color`, `--lang` | auto by default |
| `--update` | check/perform updates |

### Client (`pastebin-cli`) essentials
- No auth required to use public server endpoints (open API, pastebin model); tokens are for **ownership**, not access
- `--server URL` required if not set in `cli.yml`
- Exit-immediately flags never launch TUI: `-h/--help`, `-v/--version`
- Config file `cli.yml` at `~/.config/apimgr/pastebin/cli.yml` (or platform XDG equivalent), user-only permissions
- `--config` selects alternate profile files (`dev.yml`, `staging.yml`, or absolute path); must exist or error
- Env var overrides: `PASTEBIN_{SECTION}_{KEY}` or `PASTEBIN_{KEY}`
- Auto-update follows the same pattern as server self-update

### URL display
| Rule | Example |
|------|---------|
| NEVER hardcode | `localhost`, `127.0.0.1`, `0.0.0.0`, static host/IP |
| ALWAYS format | `{proto}://{fqdn}:{port}/path` |
| ALWAYS strip | `:80` (http), `:443` (https) |

For complete details, see AI.md PART 7, 8, 32.
