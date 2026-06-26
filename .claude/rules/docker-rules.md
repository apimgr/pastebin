# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Dockerfile Rules
- Location: `docker/Dockerfile` — NEVER in project root
- Multi-stage: `casjaysdev/go:latest` builder → `alpine:latest` runtime (standard)
- AIO image: `docker/Dockerfile.aio` → `debian:bookworm-slim` runtime (includes redis-server, tor)
- Default timezone: `America/New_York` (override with `TZ` env var)
- Internal port: `80` (override with `PORT` env var)
- `STOPSIGNAL SIGRTMIN+3`
- `ENTRYPOINT ["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]`
- NEVER modify ENTRYPOINT/CMD — all customization via `entrypoint.sh`
- Required packages: `git`, `curl`, `bash`, `tini`, `tor`
- Tor binary installed; server binary controls startup

## Container Port Behavior
| Context | Address | Port |
|---------|---------|------|
| Container default (prod) | `0.0.0.0` | `80` mapped: `-p 172.17.0.1:{random}:80` |
| Container default (dev) | `0.0.0.0` | `80` mapped: `-p {random}:80` (all interfaces) |
| Container custom | `0.0.0.0` | `PORT` env |

## Docker Build (in containers)
- Builder: `casjaysdev/go:latest`
- NEVER build Go on the host machine
- Volume mount: `-v $PWD:/app` (NOT `$(pwd)`)
- `-e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false`

## rootfs Overlay
- `docker/rootfs/` — BUILD-TIME overlay (standard image)
- `docker/rootfs.aio/` — AIO overlay (overrides standard entrypoint)
- Contents: `entrypoint.sh`, service configs, etc.
- Copied into image during `docker build`; NOT committed to runtime volumes

## AIO Image
- Bundles: app binary + redis-server (valkey-compatible) + tor
- `tini -p SIGTERM` as PID 1
- redis-server daemonizes in background before app starts
- `exec` to app binary so `tini` reaps redis orphan on shutdown
- `CACHE_URL=redis://127.0.0.1:6379` auto-set in entrypoint

---
For complete details, see AI.md PART 26
