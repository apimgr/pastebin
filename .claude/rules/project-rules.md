# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- **Never** use GPL/AGPL/LGPL licensed dependencies (copyleft forces the project to be GPL) — MIT/Apache 2.0/BSD/ISC/MPL only
- **Never** ship without a `LICENSE.md` in project root — legally ambiguous
- **Never** list only library names without full attribution (copyright + license type) for embedded/third-party licenses
- **Never** hardcode `{project_name}` or `{project_org}` — always infer from `git remote get-url origin` (preferred) or directory path (fallback)
- **Never** assume the current working directory is the project root — always determine it programmatically (`git rev-parse --show-toplevel`, or `os.Executable()` in Go) and use paths relative to it
- **Never** put user-editable config in `{data_dir}` or app-managed data in `{config_dir}` — never mix directory purposes
- **Never** skip a required OS (Linux, BSD, macOS, Windows) or architecture (AMD64, ARM64)
- **Never** hardcode a specific Go version anywhere (docs, Docker, CI) — always latest stable
- **Never** use a CGO-requiring library — breaks `CGO_ENABLED=0` static builds
- **Never** use `github.com/mattn/go-sqlite3` (CGO) — use `modernc.org/sqlite` (pure Go)
- **Never** use forbidden libraries: `lib/pq`, `ooni/go-libtor`, `dgrijalva/jwt-go`, `gorilla/mux`, `go-redis/redis` (old path)
- **Never** run `go` directly — always use Makefile targets (`make dev`, `make test`, etc.)
- **Never** commit `binaries/`, `releases/`, `volumes/`, or `docker/rootfs/` — all gitignored
- **Never** put project-specific Dockerfile in the repo root — it belongs in `docker/`
- **Never** omit the required `.gitignore` header (`# gitignore created on MM/DD/YY at HH:MM` + literal `ignoredirmessage` line)

## CRITICAL - ALWAYS DO

- Always use MIT License; `LICENSE.md` in project root with copyright `(c) {year} {project_org}`
- Always embed third-party dependency licenses in `LICENSE.md` (compact table format recommended for 10+ deps)
- Always include a license badge in README.md: `[![License](https://img.shields.io/github/license/{project_org}/{project_name})](LICENSE.md)`
- Always apply license metadata as an OCI annotation at Docker build time (`org.opencontainers.image.licenses=MIT`), never a Dockerfile `LABEL`
- Always verify no copyleft licenses via CI (`go-licenses csv ./... | grep -iE 'GPL|AGPL|LGPL'`)
- Always infer `{project_name}`/`{project_org}` from git remote first, directory path as fallback
- Always keep `{internal_name}` frozen from first setup forever, even after a project rename (`{project_name}` may change, `{internal_name}` never does)
- Always place the `.claude/rules/` directory with all 13 required rule files (this file is one of them)
- Always use `modernc.org/sqlite` for SQLite (pure Go, `CGO_ENABLED=0`); accept config aliases `sqlite`/`sqlite2`/`sqlite3`, normalize internally to `sqlite`
- Always use `github.com/tursodatabase/libsql-client-go` for libSQL/Turso (remote-only; accepts `libsql`/`turso` config aliases)
- Always build with `casjaysdev/go:latest` in Docker/CI — never `setup-go`, never a pinned tag
- Always support all 4 OSes (Linux, BSD, macOS, Windows) and both architectures (AMD64, ARM64)
- Always determine project root programmatically; never assume CWD

## Key Rules Summary

### Variable placeholders
| Placeholder | Case | Frozen? | Use |
|---|---|---|---|
| `{project_name}` | lower | No (can rename) | binaries, user-facing, docs |
| `{PROJECT_NAME}` | UPPER | No | env vars, Makefile |
| `{project_org}` | lower | No | filenames, paths, owners |
| `{internal_name}` | lower | **Yes** | config/data/log/cache/pid dirs, systemd unit, plist |
| `{plist_name}` | derived | Yes | `io.github.{project_org}.{internal_name}` |

For pastebin: `project_name=pastebin`, `project_org=apimgr`, `internal_name=pastebin`, `internal_org=apimgr`.

### Required `.claude/rules/` files (this project)
`ai-rules.md`, `project-rules.md`, `config-rules.md`, `binary-rules.md`, `backend-rules.md`, `api-rules.md`, `frontend-rules.md`, `features-rules.md`, `service-rules.md`, `makefile-rules.md`, `docker-rules.md`, `cicd-rules.md`, `testing-rules.md`.

### Directory structure highlights
- `src/` — all source; `docker/` — Dockerfile + compose + `rootfs/`; `tests/` — integration scripts (`run_tests.sh`, `docker.sh`, `incus.sh`)
- `docs/` — MkDocs/ReadTheDocs only; `binaries/`, `releases/`, `volumes/` — gitignored runtime/build output
- Root files: `README.md`, `LICENSE.md`, `AI.md`, `TODO.AI.md`, `Jenkinsfile`, `release.txt`

### OS-specific paths (Linux example — see AI.md PART 4 for macOS/BSD/Windows/Docker)
| Type | Privileged | User |
|---|---|---|
| Config | `/etc/apimgr/pastebin/server.yml` | `~/.config/apimgr/pastebin/server.yml` |
| Data | `/var/lib/apimgr/pastebin/` | `~/.local/share/apimgr/pastebin/` |
| Logs | `/var/log/apimgr/pastebin/server.log` | `~/.local/log/apimgr/pastebin/server.log` |
| PID | `/var/run/apimgr/pastebin.pid` | `~/.local/share/apimgr/pastebin/pastebin.pid` |
| Service | `/etc/systemd/system/pastebin.service` | — |

Docker-only paths: `/config/{project_name}/`, `/data/{project_name}/` (never used on native OS installs).

### Required Go libraries
SQLite: `modernc.org/sqlite` · libSQL: `tursodatabase/libsql-client-go` · Cache: `redis/go-redis/v9`, `bradfitz/gomemcache` · Router: `go-chi/chi/v5` · Tor: `cretz/bine` · Scheduler: `go-co-op/gocron/v2` · Validation: `go-playground/validator/v10`

For complete details, see AI.md PART 2, 3, 4.
