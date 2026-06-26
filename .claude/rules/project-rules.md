# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## License & Attribution (PART 2)
- MIT license — our code only
- All 3rd-party licenses listed in LICENSE.md
- 100% free, no paid tiers, no enterprise edition
- No feature gating, no premium tiers, no license keys
- NEVER implement "Upgrade to unlock..." or usage limits for monetization
- Rate limits (DDoS protection) are allowed; usage caps (monetization) are FORBIDDEN

## Project Structure (PART 3)
```
src/               # Go source code
src/main.go        # Server entry point
src/config/        # Config package
src/server/        # HTTP server package
src/client/        # CLI client (REQUIRED)
docker/            # Docker files
docker/Dockerfile  # Multi-stage Dockerfile
docker/rootfs/     # BUILD-TIME container filesystem overlay
volumes/           # RUNTIME volume data (gitignored)
binaries/          # Build output (gitignored)
releases/          # Release artifacts (gitignored)
```
- Binary naming: `pastebin-{os}-{arch}` (windows adds `.exe`)
- NEVER use `-musl` suffix
- NEVER put Dockerfile in project root
- All assets embedded with Go `embed` package

## Directory Naming (Go)
- Singular: `handler/`, `model/`, `service/`, `store/` (match package names)
- NOT: `handlers/`, `models/`, `services/`
- Tooling dirs are always plural: `scripts/`, `tests/`, `completions/`

## OS-Specific Paths (PART 4)
| Context | Config | Data | Logs |
|---------|--------|------|------|
| Root/system | `/etc/apimgr/pastebin/server.yml` | `/var/lib/apimgr/pastebin/` | `/var/log/apimgr/pastebin/` |
| User | `~/.config/apimgr/pastebin/server.yml` | `~/.local/share/apimgr/pastebin/` | `~/.local/log/apimgr/pastebin/` |
| Docker | `/config/pastebin/server.yml` | `/data/pastebin/` | `/data/pastebin/logs/` |
| macOS | `~/Library/Application Support/apimgr/pastebin/` | same | `~/Library/Logs/apimgr/pastebin/` |
| Windows | `%APPDATA%\apimgr\pastebin\` | `%LOCALAPPDATA%\apimgr\pastebin\` | `%LOCALAPPDATA%\apimgr\pastebin\logs\` |

- DB file: `{data_dir}/server.db`
- Cache: `{data_dir}/cache/`
- Backup: `{data_dir}/backup/`
- Temp: `/tmp/apimgr/pastebin-XXXXXX/`

---
For complete details, see AI.md PART 2, 3, 4
