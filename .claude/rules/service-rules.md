# Service Rules (PART 23, 24)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Privilege Escalation (PART 23)
- `isElevated()` — check if running as root/admin
- `canEscalate()` — check if sudo/runas available
- `execElevated()` — re-exec with elevated privileges
- Platform-split: `_unix.go` / `_windows.go` build tags; CGO_ENABLED=0 safe
- System user `pastebin` created on first root startup
- Privilege dropped after port binding (Unix)
- Windows: NT SERVICE VSA

## Service Lifecycle (PART 23, 24)
- `--service --install` — install systemd unit (Linux), launchd plist (macOS), Windows Service
- `--service --uninstall` — `[y/N]` destructive confirmation + data/user purge
- `--service start|stop|restart|reload|--disable`
- Post-install: start service automatically

## Systemd Unit Requirements (PART 24)
- `Type=simple`
- `RestartSec=5`
- `Restart=on-failure` (NOT `Restart=always`)
- Hardening directives: `NoNewPrivileges=yes`, `ProtectSystem=strict`, `PrivateTmp=yes`, etc.

## Supported Service Managers (PART 24)
| Platform | Service Manager |
|----------|----------------|
| Linux (systemd) | systemd unit |
| Linux (SysV) | init.d script |
| Linux (OpenRC) | OpenRC service |
| macOS | launchd plist |
| Windows | Windows Service |
| FreeBSD | rc.d script |

- Auto-detect service manager at runtime
- NEVER assume systemd — detect actual init system

---
For complete details, see AI.md PART 23, 24
