# Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Configuration (PART 5)
- Config file: `server.yml` (NOT `.yaml`); auto-migrate if `.yaml` found
- NEVER use `strconv.ParseBool()` — ALWAYS use `config.ParseBool()` / `config.IsTruthy()`
- Accepts ~40 truthy/falsy strings: yes/no, true/false, 1/0, on/off, enable/disable, etc.
- NEVER load `.env` files — read env vars individually as overrides
- Port: random unused 64000–64999 on first run, saved to config; dual format `"8090,8443"`
- Port 80/443 triggers Let's Encrypt; first-run auto-select random 64xxx port
- `Validate()` warns-and-defaults — NEVER fails startup
- `EncryptionKey`: AES-256, auto-generated via `crypto/rand`, stored in config
- Maintenance mode: response has `ok:false`, `error:"MAINTENANCE_MODE"`, `Retry-After` header

## Boolean Handling
- `config.ParseBool()` — the ONLY boolean parser in this codebase
- Truthy: `yes`, `true`, `1`, `on`, `enable`, `enabled`, `active`, `t`, `y`, `ok`, `affirmative`, ...
- Falsy: `no`, `false`, `0`, `off`, `disable`, `disabled`, `inactive`, `f`, `n`, `nope`, ...
- Case-insensitive; leading/trailing whitespace stripped

## Privilege & System User (PART 5)
- `isElevated()` / `canEscalate()` / `execElevated()` — platform-split `_unix.go` / `_windows.go`
- CGO_ENABLED=0 safe — no cgo in privilege code
- System user `pastebin` created during first root startup; privilege dropped after port binding
- Windows: NT SERVICE VSA
- Sensitive ops (`--maintenance setup/restore/mode`) require authorization proof

## Application Modes (PART 6)
| Mode | Debug | Result |
|------|-------|--------|
| production | false | Normal operation |
| production | true | Normal + debug endpoints |
| development | false | Dev behavior |
| development | true | Dev + debug endpoints |

- Mode priority: `--mode` flag → `MODE` env → default `production`
- Debug priority: `--debug` flag → `DEBUG` env truthy → default `false`
- Debug enables: `/debug/pprof/*`, `/debug/vars`, `/debug/config`, `/debug/routes`
- Debug bypasses operator token auth (dev-only — NEVER in production tests)
- `src/mode/mode.go`: `SetAppMode()`, `SetDebugEnabled()`, `IsDebugEnabled()`, `FromEnv()`

## Server Configuration (PART 12)
- Schema covers: limits, proxies, rate-limit, TLS, scheduler, headers, webhooks, contact roles
- All settings configurable via config file AND API
- NEVER hardcode dev machine values — detect at runtime

---
For complete details, see AI.md PART 5, 6, 12
