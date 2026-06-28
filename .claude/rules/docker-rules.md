# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Docker Directory Structure
```
docker/
├── Dockerfile              # Production Dockerfile (only one — NO Dockerfile.aio)
├── Dockerfile.dev          # devel image — debug mode; tagged :devel
├── docker-compose.yml      # Production compose — HUMAN USE ONLY
├── docker-compose.dev.yml  # Development compose — HUMAN USE ONLY
├── docker-compose.test.yml # Test compose — AI/AUTOMATED TESTING ONLY
└── rootfs/
    └── usr/local/bin/
        └── entrypoint.sh  # Container entrypoint (REQUIRED)
```

## Dockerfile Rules
- Location: `docker/Dockerfile` — NEVER in project root
- Multi-stage: `casjaysdev/go:latest` builder → `alpine:latest` runtime
- NO `Dockerfile.aio`, NO AIO image, NO `rootfs.aio/`
- Default timezone: `America/New_York` (override with `TZ` env var)
- Internal port: `80` (override with `PORT` env var)
- `STOPSIGNAL SIGRTMIN+3`
- `ENTRYPOINT ["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]`
- NEVER modify ENTRYPOINT/CMD — all customization via `entrypoint.sh`
- Required packages: `git`, `curl`, `bash`, `tini`, `tor`
- Tor binary installed; server binary controls startup
- No LABEL blocks — all OCI metadata via `annotations:` in CI workflow only

## Container Port Behavior
| Context | Port mapping |
|---------|-------------|
| Production | `172.17.0.1:{random}:80` |
| Development | `172.17.0.1:64580:80` |
| Test | `64581:80` (all interfaces) |

## Docker Build (in containers)
- Builder: `casjaysdev/go:latest`
- NEVER build Go on the host machine
- Volume mount: `-v $PWD:/app` (NOT `$(pwd)`)
- `-e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false`

## rootfs Overlay
- `docker/rootfs/` — BUILD-TIME overlay only
- Contents: `entrypoint.sh`, service configs, etc.
- Copied into image during `docker build`; NOT committed to runtime volumes

## Docker Compose
- `name: {project_name}` (top-level)
- `container_name: {project_name}-app`
- `hostname: ${BASE_HOST_NAME:-$HOSTNAME}`
- `pull_policy: always`
- `restart: always`
- All env vars use `${VAR:-default}` fallbacks — stack works with zero .env files
- NEVER run compose from project directory — always use temp dir workflow

---
For complete details, see AI.md PART 26
