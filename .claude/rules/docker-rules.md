# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never place `Dockerfile` or `docker-compose*.yml` in project root — always `docker/`
- Never modify `ENTRYPOINT` or `CMD` in the Dockerfile — all customization goes in `entrypoint.sh`
- Never add `LABEL` blocks to the Dockerfile — all metadata applied by CI as `--label`/`--annotation` at build time
- Never include `build:` or `version:` keys in any docker-compose file
- Never use `${VAR}`/`${VAR:-default}` syntax or `.env`/`.env.example`/`.env.sample` files in compose — hardcode sane defaults directly, YAML map style (`KEY: value`), never list style (`- KEY=value`)
- Never run `docker compose` from the project directory, or with `--project-directory` pointing at project root, or mount volumes to `{project_root}/volumes/` — always use the temp-dir workflow
- Never let entrypoint.sh create directories, set permissions, create users/groups, or manage Tor — the binary does all of that
- Never omit `exec` at the end of entrypoint.sh — without it, signals never reach the app (PID 1 problem)
- Never push `:dev` or `:test` image tags to the production registry
- AI must never use `docker/docker-compose.yml` (production) or `docker/docker-compose.dev.yml` (human-only) directly — use `docker/docker-compose.test.yml` via `tests/` scripts, or as fallback, the direct temp-dir workflow
- Never commit runtime `./volumes/` content from local runs — only `docker/rootfs/` (build-time overlay) is committed

## CRITICAL - ALWAYS DO
- Dockerfile at `docker/Dockerfile`; build context is project root (`.`); build with `-f docker/Dockerfile .`
- Multi-stage build: builder stage `FROM casjaysdev/go:latest`, runtime stage `FROM alpine:latest`
- `CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build` in the builder stage; strip with `-ldflags "-s -w ..."`
- Copy build-time overlay via `COPY docker/rootfs/ /` (contains `docker/rootfs/usr/local/bin/entrypoint.sh`, REQUIRED)
- Runtime stage installs: `git curl bash tini tor`; Tor binary installed but the pastebin binary controls Tor startup (PART 31)
- Runtime stage creates and switches to a non-root user (`addgroup -S app && adduser -S -G app app` then `USER app`) unless the app must bind <1024 (prefer `setcap`) or manage system services — document any exception in IDEA.md
- `ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]`
- `STOPSIGNAL SIGRTMIN+3` (systemd-compatible graceful shutdown)
- `HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3 CMD /usr/local/bin/pastebin --status`
- `EXPOSE 80` — internal port is always 80
- `ENV MODE` never set in the Dockerfile — binary defaults to production; compose files set `MODE` explicitly
- Every docker-compose file includes the `x-logging: &default-logging` anchor (`max-size: '5m'`, `max-file: '1'`, `driver: json-file`) and every service references it via `logging: *default-logging`
- Use the temp-dir workflow for any `docker compose up`: `mkdir -p "${TMPDIR:-/tmp}/apimgr"`, `mktemp -d ".../pastebin-XXXXXX"`, create `volumes/config` + `volumes/data`, copy the compose file in, `cd` there, run, then `rm -rf` when done
- Compose mounts exactly two volumes: `./volumes/config:/config:z` and `./volumes/data:/data:z` (`:z` in production/test, omitted in dev temp dirs is fine either way but production docs use `:z`)
- All release images built for `linux/amd64` AND `linux/arm64`; release images pushed to `{PLATFORM_CONTAINER_REGISTRY}/apimgr/pastebin`

## Key Rules Summary

**Directory layout (`docker/`):**
| File | Purpose |
|------|---------|
| `Dockerfile` | Production multi-stage build |
| `Dockerfile.dev` | Same as release but binary runs in debug mode, tagged `:devel` |
| `docker-compose.yml` | Production — human use only |
| `docker-compose.dev.yml` | Development — human use only |
| `docker-compose.test.yml` | Automated testing — AI/CI use (prefer `tests/run_tests.sh`, `tests/docker.sh`) |
| `rootfs/usr/local/bin/entrypoint.sh` | Container entrypoint (required, minimal) |

**Container paths:**
| Path | Contents |
|------|----------|
| `/config/pastebin/` | server.yml, `ssl/`, `tor/torrc` |
| `/data/pastebin/` | uploads, cache, `security/{geoip,blocklists}/`, `tor/` |
| `/data/db/sqlite/server.db` | Main database (name always `server.db`) |
| `/data/db/valkey/` | Cache persistence (if used) |
| `/data/log/pastebin/` | access.log, error.log, tor.log |
| `/data/backups/pastebin/` | Backup archives |
| `/usr/local/bin/pastebin` | Application binary |

**Entrypoint responsibilities (minimal only):** set env defaults (`TZ`, `CONFIG_DIR`, `DATA_DIR`), build CLI flags from env (`ADDRESS`, `PORT`, `DEBUG`), trap `SIGTERM`/`SIGINT`/`SIGQUIT` for graceful shutdown, `exec` the binary as the final step.

**Entrypoint env vars:** `TZ` (default `America/New_York`), `MODE` (`production`/`development`), `DEBUG` (`false`), `ADDRESS` (`0.0.0.0`), `PORT` (`80`). Boolean env vars accept any truthy/falsy spelling.

**Compose service naming:**
| Type | Service | Container |
|------|---------|-----------|
| Main app | `pastebin` | `pastebin-app` |
| Database | `pastebin-db` | `pastebin-db` |
| Cache | `pastebin-cache` | `pastebin-cache` |
| Proxy | `pastebin-proxy` | `pastebin-proxy` |

**Compose env var differences:**
| File | DEBUG/MODE |
|------|-----------|
| `docker-compose.yml` (prod) | neither set (production defaults) |
| `docker-compose.dev.yml` | `DEBUG: 1`, `MODE: dev` |
| `docker-compose.test.yml` | `DEBUG: 1`, `MODE: dev` |

**Port mapping:** internal always `80`; dev external `{randomport}:80` (e.g. `64580:80`, all interfaces); production `172.17.0.1:{randomport}:80` (bridge-only, reverse proxy handles external).

**Required OCI labels (applied by CI via `--label`/`--annotation`, never hardcoded in Dockerfile):** `maintainer`, `org.opencontainers.image.{vendor,authors,title,base.name,description,licenses,created,version,schema-version,revision,url,source,documentation,vcs-type}`, `com.github.containers.toolbox`. Multi-arch manifests need annotations (via `docker/metadata-action`), not just per-layer `LABEL`/`--label`.

**Container detection:** `/.dockerenv`, `/run/.containerenv`, `/dev/lxc`, `container`/`KUBERNETES_SERVICE_HOST` env vars, tini/dumb-init/s6/runsv/catatonit as PID 1, `/proc/1/cgroup` containing `docker`/`kubepods`/`lxc`.

**Image tags:** release → `:latest`, `:{version}`, `:{YYMM}`, `:{commit}` pushed to the platform registry; local dev/test → `pastebin:dev` / `pastebin:test`, never pushed.

For complete details, see AI.md PART 26.
