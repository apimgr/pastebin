# Service Rules (PART 23, 24)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never prompt for privilege escalation if the user cannot actually escalate (not in sudoers/wheel/admin) — show an informative error instead
- Never skip the "already root/admin" check before prompting for escalation
- Never reuse a reserved/well-known UID/GID (see table below) even if it appears available on the current system
- Never create the service user with mismatched UID/GID — they MUST be the same value
- Never run the service permanently as root/Administrator unless explicitly approved in IDEA.md (and if so, the service file and docs MUST say so and explain why privilege drop is not possible)
- Never use Local System, Administrator, or a logged-in user account for the Windows service — Virtual Service Account (VSA) is the default
- Never delete data/config/user on `--service --disable` — that is `--uninstall`'s job, not `--disable`'s
- Never skip the confirmation prompt before `--service --uninstall` (destructive: deletes all data, configs, system user)
- Never install a service file without also enabling and starting it (`--install` = install + enable + start, one command)
- Never bind to privileged ports (<1024) as an unprivileged user — only root/admin may do so, and only until it drops privileges
- Never assume systemd is present — detect actual init system (systemd, OpenRC, SysVinit, runit, launchd, rc.d, Windows Service) before installing

## CRITICAL - ALWAYS DO
- Always check if already root/admin first; only prompt for escalation if the user CAN escalate
- Always let the binary itself handle user/group creation, privilege escalation, directory setup, and permissions during normal startup — `--service --install` only installs and starts the service
- Always create directories before creating the user, then set ownership
- Always use UID == GID (same numeric value) for the service system user
- Always select the UID/GID from the safe range, working from the top down, skipping reserved IDs
- Always use `/sbin/nologin` (or `/usr/sbin/nologin`) as shell for the Unix system user — no login, no password
- Always start elevated only for privileged port binding, then drop to the dedicated `{project_name}` user (Unix) after binding
- Always use Virtual Service Account (`NT SERVICE\{internal_name}`) for Windows services by default
- Always keep config/data/cache/logs/user intact on `--service --disable` (re-enable via `--install`)
- Always prompt for confirmation before `--service --uninstall`, then remove config/data/cache/log/backup dirs, PID file, and the system user/group — leave the binary, with a message to delete it manually
- Always detect the actual init system before writing a service file: systemd/OpenRC/SysVinit/runit (Linux), launchd (macOS), rc.d (BSD), Windows Service

## Key Rules Summary

### Escalation methods by OS (in order tried)
| OS | Methods |
|----|---------|
| Linux | already root → sudo → su → pkexec → doas |
| macOS | already root → sudo → osascript (GUI) |
| BSD | already root → doas → sudo → su |
| Windows | already Administrator → UAC prompt → runas |

### `--service` subcommands
| Command | Effect |
|---------|--------|
| `start` / `stop` / `restart` / `reload` | Standard lifecycle |
| `--install` | Install, enable, and start (idempotent) |
| `--disable` | Stop + disable auto-start; data/config/user kept |
| `--uninstall` | Stop + disable + remove service file + delete ALL data + delete user/group; binary remains (confirmation required) |

### System user requirements
| Requirement | Value |
|-------------|-------|
| Username / Group | `{internal_name}` (both, matching) |
| UID/GID | Must be equal; Linux/generic safe range **200-899**; macOS safe range **200-399** |
| Shell | `/sbin/nologin` or `/usr/sbin/nologin` |
| Home | Config dir or data dir (default: config dir) |
| Type | System user, no password, no login |
| Gecos | `{internal_name} service account` |

**UID/GID selection:** start at top of safe range, walk downward, skip any reserved ID, stop when both UID and GID are free. Reserved IDs include 65534 (nobody), 999/998/997/996/995 (docker, systemd-*), 994-980 (systemd-network/resolve/timesync, input, kvm, render, sgx, pipewire, colord, geoclue, avahi, rtkit, saned, usbmux, cups-pk-helper), 170-179, 101-110 (sshd, postfix, dovecot).

**Default:** create a dedicated service user/group. **Exception:** only skip if IDEA.md explicitly approves running permanently as root/Administrator.

### Platform commands (creation)
- Linux: `groupadd --system --gid {id} {internal_name}` + `useradd --system --uid {id} --gid {id} --shell /sbin/nologin ...`
- macOS: `dscl` (hidden from login, `IsHidden 1`, `UserShell /usr/bin/false`)
- FreeBSD: `pw groupadd` / `pw useradd`
- Windows: Virtual Service Account, auto-managed, no manual user creation — `New-Service` with empty `ServiceStartName`

### Service manager install paths
| Manager | Path |
|---------|------|
| systemd | `/etc/systemd/system/{internal_name}.service` |
| OpenRC | `/etc/init.d/{internal_name}` |
| SysVinit | `/etc/init.d/{internal_name}` (only if OpenRC/systemd absent) |
| runit | `/etc/sv/{internal_name}/run` |
| rc.d (FreeBSD) | `/usr/local/etc/rc.d/{internal_name}` |
| launchd (macOS) | `/Library/LaunchDaemons/{plist_name}.plist` |
| Windows | Service Control Manager, VSA account |

**Detection order for Linux init:** systemd → OpenRC → runit → SysVinit fallback (only when `/sbin/openrc-run` absent, `systemctl` absent, `/etc/init.d/` present with working `update-rc.d`/`chkconfig`).

**systemd hardening:** `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, explicit `ReadWritePaths=` for config/data/cache/log dirs, `Restart=on-failure`.

### Run modes
| Mode | Runs As | Port Restriction | Privilege Drop |
|------|---------|-------------------|-----------------|
| Service (escalated) | root/admin | Any port | Yes, after binding (Unix) |
| User mode | Calling user | >1024 only | No (already unprivileged) |

For complete details, see AI.md PART 23, 24.
