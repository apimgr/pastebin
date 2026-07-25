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
| Development | `{randomport}:80` (all interfaces, e.g. `64580:80`) |
| Test | `172.17.0.1:64581:80` |

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
- `container_name: {project_name}-app` (main), `{project_name}-cache` (Valkey)
- `hostname: {project_name}` (hardcoded, production compose only)
- `pull_policy: always`
- `restart: always`
- NEVER `${VAR}` / `${VAR:-default}` syntax — env vars are hardcoded with sane defaults, YAML map style (`KEY: value`), never list style (`- KEY=value`)
- NEVER `.env`, `.env.example`, `.env.sample` files — stack works with zero .env files
- NEVER `build:` or `version:` keys
- `docker-compose.yml` (prod): no `DEBUG`/`MODE` — production defaults apply
- `docker-compose.dev.yml` / `docker-compose.test.yml`: `DEBUG: 1`, `MODE: dev`
- NEVER run compose from project directory — always use temp dir workflow

---
For complete details, see AI.md PART 26
