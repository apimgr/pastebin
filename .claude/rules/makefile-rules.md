# Makefile Rules (PART 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Makefile is for **LOCAL DEVELOPMENT ONLY**. CI/CD workflows NEVER call `make` targets — they use explicit commands (see `cicd-rules.md`, PART 27).

## CRITICAL - NEVER DO
- Never use `make` targets inside CI/CD workflows — CI/CD uses explicit commands only
- Never add more than the six core targets (`dev`, `local`, `build`, `test`, `release`, `docker`) without explicit user approval
- Never hardcode `PROJECT_NAME` or `PROJECT_ORG` — always infer from `git remote get-url origin` or directory path
- Never add `v` prefix to a non-numeric (text/timestamp) version tag: `vdev`, `vbeta`, `vdaily`, `v20251218` are all WRONG
- Never double the `v` prefix (`vv1.2.3`)
- Never guess or assume `site.txt` / `OFFICIAL_SITE` — must be explicitly created/set by the user; empty is valid for self-hosted projects
- Never symlink or copy binaries out of `binaries/` (e.g. to `/usr/local/bin`, `~/bin`, `/tmp`) — run them in place; the only copy exception is the CI/CD release process copying to `releases/`
- Never skip test coverage enforcement (`-short`, `-count=0`, coverage-ignore workarounds) — 60% minimum floor (override upward only, never downward, via `IDEA.md` `coverage_minimum`)
- Never build on the host — all `make` targets that compile/test run inside Docker (`casjaysdev/go:latest`)
- Never use CGO — all builds set `CGO_ENABLED=0`
- Never omit `GOFLAGS=-buildvcs=false` on Docker Go builds (mounted `.git` UID mismatch breaks `go build` otherwise)
- Never let `release.txt` be ignored if present — it always wins over `VERSION` env var or derived/tag values
- Never keep a `-musl` suffix on a musl-built binary — strip it and use the standard name

## CRITICAL - ALWAYS DO
- Six core targets only: `dev` (quick temp-dir build), `local` (production-test build to `binaries/`), `build` (full 8-platform release build), `test` (unit tests + coverage), `release` (manual local release to `releases/`), `docker` (build+push container)
- Version precedence: `release.txt` file (if exists, wins) → `VERSION` env var → `devel` fallback
- Binary naming pattern: `{project_name}[-type]-{os}-{arch}[.exe]` — server: `pastebin`, `pastebin-{os}-{arch}`; client: `pastebin-cli`, `pastebin-cli-{os}-{arch}`
- Build matrix: 8 platforms — linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, freebsd/amd64, freebsd/arm64
- Embed `Version`, `CommitID`, `BuildDate` (ISO 8601 UTC), `OfficialSite` via `-ldflags` in every release/local/build binary
- Use persistent Go module/build caches: `GO_CACHE ?= $(HOME)/go/pkg/mod`, `GO_BUILD ?= $(HOME)/.cache/go-build/{project_name}`
- `make dev` outputs to `${TMPDIR:-/tmp}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/` (isolated, no ldflags, fastest)
- `make test` must enforce ≥60% coverage and fail the build if under threshold
- `make release` uses `gh` CLI for manual local releases only; all automated (stable/beta/daily) releases go through CI/CD
- `make docker` uses `docker buildx` multi-arch build against `docker/Dockerfile`, pushing `linux/amd64` + `linux/arm64`
- Add `v` prefix ONLY to numeric semver tags (`0.2.0` → `v0.2.0`); text (`dev`, `beta`) and timestamp versions never get a `v`
- `clean` target removes `binaries/` and `releases/` only

## Key Rules Summary

### Targets
| Target | Purpose | Output | When |
|--------|---------|--------|------|
| `dev` | Quick dev build | temp dir | active coding |
| `local` | Prod-test build | `binaries/` | test before release |
| `build` | Full release, 8 platforms | `binaries/` | before release |
| `test` | Unit tests + coverage | coverage report | after code changes |
| `release` | Manual release + archive | `releases/` | manual releases |
| `docker` | Build + push container | `$REGISTRY` | container deploy |

### Version files
| File | Purpose |
|------|---------|
| `release.txt` | Canonical single-line stable version (wins over all else) |
| `site.txt` | Optional canonical official site URL (wins over IDEA.md/env/CI secrets) |
| `releases/version.txt` | Version string included in release archive |

### Version tag `v` prefix
| Input | Type | Gets `v`? |
|-------|------|-----------|
| `0.2.0`, `1.2.3-rc1` | numeric semver | yes |
| `dev`, `beta`, `daily` | text | no |
| `20251218060432` | timestamp | no |

### Release types
| Type | Trigger | Version format | Has `v`? | Max releases |
|------|---------|-----------------|----------|---------------|
| Stable | tag push (semver) | `X.Y.Z` | yes | unlimited |
| Beta | push to `beta` branch | `{YYYYMMDDHHMMSS}-beta` | no | unlimited |
| Daily | daily schedule / push to main | `{YYYYMMDDHHMMSS}` | no | 1 (rolling, replaces prior) |

### Directory rules
- `binaries/` — build output only, gitignored, never symlinked/copied except by CI/CD release process
- `releases/` — packaged release artifacts (binaries + `version.txt` + source tarball)
- Docker toolchain image: `casjaysdev/go:latest`, `-e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false`

For complete details, see AI.md PART 25.
