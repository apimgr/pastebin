# Binary Rules (PART 7, 8, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Binary Requirements (PART 7)
- `CGO_ENABLED=0` ALWAYS — pure Go, no exceptions
- Single static binary — all assets embedded with `//go:embed`
- 8 required platforms: linux/darwin/windows/freebsd × amd64/arm64
- Binary naming: `pastebin-{os}-{arch}` (windows: `pastebin-{os}-{arch}.exe`)
- NEVER use `-musl` suffix
- Build source: ALWAYS `./src` directory
- LDFLAGS: `-s -w -X 'main.Version=...' -X 'main.CommitID=...' -X 'main.BuildDate=...' -X 'main.OfficialSite=...'`
- All four var declarations MUST exist in both `src/main.go` and `src/client/main.go`

## Server Binary CLI (PART 8)
```
--help / -h
--version / -v
--mode {production|development}
--config {config_dir}
--data {data_dir}
--log {log_dir}
--pid {pid_file}
--address {listen}
--port {port}
--baseurl {path}
--debug
--status
--service {start,restart,stop,reload,--install,--uninstall,--disable,--help}
--daemon
--maintenance {backup,restore,update,mode,setup,--help} [optional-file-or-setting]
--update [check|yes|branch {stable|beta|daily}]
```
- Short flags: `-h` (help) and `-v` (version) ONLY — all others are long-form only
- These CLI flags are NON-NEGOTIABLE — do not change, rename, or remove

## Runtime Detection (NEVER hardcode)
| Value | Detection |
|-------|-----------|
| Hostname | `os.Hostname()` at startup |
| IP address | Network interface scan (skip veth/docker/tun/wg/tailscale) |
| CPU cores | `runtime.NumCPU()` |
| Memory | System memory APIs at runtime |
| OS/Arch | `runtime.GOOS`, `runtime.GOARCH` |
| Timezone | System TZ or `TZ` env var |

## Display Detection
- `NO_COLOR` env var respected — disables all ANSI color
- `--color {auto|always|never}` flag overrides `NO_COLOR`
- Terminal width detection for responsive output
- 7 size breakpoints: Micro (<40), Tiny (40-59), Small (60-79), Medium (80-99), Large (100-119), Wide (120-139), Wide+ (140+)

## Client Binary (PART 32)
- Binary: `pastebin-cli`
- Commands: `create`, `get`, `delete`, `list`, `update`, `tui`, `completions`
- All requests send `User-Agent: pastebin/{version}` and `Accept-Language: {lang}`
- Config: `cli.yml` (same OS path logic as server, minus `/etc/`)
- Exit codes: 0=success, 1=general, 2=config, 3=connection, 4=auth, 5=not found, 64=usage
- TUI: bubbletea + bubbles + lipgloss (all required deps)
- Auto-update: download → SHA-256 verify → atomic `os.Rename` → `syscall.Exec` re-exec

---
For complete details, see AI.md PART 7, 8, 32
