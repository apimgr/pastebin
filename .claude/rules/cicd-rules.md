# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never use Makefile targets inside any CI/CD workflow — commands must be explicit (`go build ...`) so every flag is visible in the workflow file
- Never reference local user paths (e.g. `~/.local/share/go`) in CI — use `/tmp/` or CI-native caching instead
- Never depend on local Docker containers for the build step on GitHub/Gitea/Forgejo — those platforms use `container: image: casjaysdev/go:latest` directly, not a bind-mounted local container
- Never cross-cancel different release refs — concurrency groups must only cancel an older run for the *exact same* branch or tag ref (e.g. `v1.2.4` must never cancel `v1.2.3`)
- Never pin a third-party Action to a floating tag (`@v4`, `@main`) — pin to a full commit SHA, with the version as a trailing comment (e.g. `actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0  # v7.0.0`)
- Never create `build-toolchain.yml` or an `ensure-build-image` gate for Go projects — `casjaysdev/go:latest` is externally maintained and used directly
- Never `apk add` or `go install` extra tooling inline inside a CI job — all tooling (git, bash, govulncheck, cyclonedx-gomod, staticcheck, etc.) is pre-installed in `casjaysdev/go:latest`
- Never use `default_branch` for the secret-scan commit range — after a push it resolves to the same commit as HEAD and silently skips the scan; use `github.event.before` / `github.event.after`
- Never skip any of the 8 release platform targets (linux, darwin, windows, freebsd × amd64, arm64)
- Never guess `OFFICIAL_SITE` — it comes from `site.txt` if present, otherwise a repository/CI secret, otherwise left empty; never hardcode a guessed value
- Never omit `permissions:` blocks — each job/workflow declares the minimum permissions it needs (e.g. `contents: read` for CI, `contents: write` for release)

## CRITICAL - ALWAYS DO
- Always use explicit `go build` commands with all flags visible (`-buildvcs=false -trimpath -ldflags "..."`) in CI — never a Makefile wrapper
- Always set `VERSION`, `COMMIT_ID`, `BUILD_DATE` explicitly via a "Set build info" step, written to `$GITHUB_ENV`/`$GITEA_ENV`/`$FORGEJO_ENV` or exported for GitLab
- Always build the full 8-platform matrix for release/beta/daily workflows: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64, freebsd/amd64, freebsd/arm64
- Always add workflow `concurrency` groups keyed on the ref, with `cancel-in-progress` true for branch-push workflows targeting `main`/`master`/`devel`/`dev`/`beta`, and true-per-exact-tag for tag-only release workflows
- Always run every job inside `container: image: casjaysdev/go:latest` for GitHub/Gitea/Forgejo Go jobs (or `image: $BUILD_IMAGE` set to the same for GitLab) — never install Go on the runner directly
- Always set `CGO_ENABLED: "0"` for every Go build/test job
- Always build the CLI binary too, but only when `src/client/` exists (`if: hashFiles('src/client/') != ''` / `if [ -d "src/client" ]`)
- Always enforce a minimum coverage threshold (60%) in `ci.yml` and fail the job if unmet
- Always run secret scanning (truffleHog, Apache-2.0) on every public repo, on push/PR and a weekly schedule
- Always use CI-native caching (GitHub Actions cache, GitLab cache, etc.) — never local host cache paths
- Always create `ci.yml` and `release.yml` for every project (required); `beta.yml`, `daily.yml`, `docker.yml` only when the project needs them
- Always build Docker images for `linux/amd64` and `linux/arm64` via `docker buildx`, tagging `devel`+`{commit_id}` on any push, `beta` also on the beta branch, and `{version}`+`latest`+`YYMM` on version tags
- Always use provider-correct variable/context names: `github.*`/`GITHUB_ENV` on GitHub, `gitea.*`/`GITEA_ENV` on Gitea, `forgejo.*`/`FORGEJO_ENV` on Forgejo (Forgejo also accepts `GITEA_*`), `$CI_*` on GitLab

## Key Rules Summary

### CI/CD vs local development
| Aspect | Local dev | CI/CD |
|---|---|---|
| Go toolchain | Docker `casjaysdev/go:latest` bind-mounted | `container: image: casjaysdev/go:latest` (native) |
| Caching | Host cache dirs bind-mounted | CI-native cache |
| Build command | `make dev` / `make build` | Explicit `go build ...` |
| Testing | Docker/Incus containers | Job container or explicit `docker run` |
| Makefile | Always use Makefile targets | Never use Makefile |

### Provider config locations
| Provider | CI system | Config path | Self-hosted |
|---|---|---|---|
| GitHub | GitHub Actions | `.github/workflows/*.yml` | No |
| Gitea | Gitea Actions | `.gitea/workflows/*.yml` | Yes |
| Forgejo | Forgejo Actions | `.forgejo/workflows/*.yml` (or `.gitea/workflows/`) | Yes, self-hosted only |
| GitLab | GitLab CI | `.gitlab-ci.yml` (single file, stages) | Yes |

### Required/optional workflow files (GitHub/Gitea/Forgejo)
| File | Trigger | Required? |
|---|---|---|
| `ci.yml` | push/PR to default branch; security jobs also weekly cron | Required |
| `release.yml` | tag push `v*` / `*.*.*` | Required |
| `beta.yml` | push to `beta` | Optional |
| `daily.yml` | daily 03:00 UTC + push to main/master | Optional |
| `docker.yml` | any push + version tags | Optional |

### ci.yml jobs
- `lint`: `go vet ./...`, `staticcheck ./...`
- `test`: `go test -cover -coverprofile=... ./...`, enforce ≥60% coverage
- `build` (needs lint, test): `go build -buildvcs=false ./...`
- `vuln-scan`: `govulncheck ./...`
- Security jobs (`secret-scan`, `workflow-policy`, `vuln-scan`, `image-scan`) also run weekly (`cron: '0 6 * * 1'`); skip non-security jobs on schedule with `if: github.event_name != 'schedule'`

### Build info env vars (every workflow)
```
VERSION    = release.txt contents, else tag/ref-derived (e.g. ${GITHUB_REF_NAME#v})
COMMIT_ID  = git rev-parse --short HEAD
BUILD_DATE = date +"%a %b %d, %Y at %H:%M:%S %Z"
OFFICIAL_SITE = site.txt contents, else secrets.OFFICIAL_SITE, else empty
LDFLAGS    = "-s -w -X 'main.Version=...' -X 'main.CommitID=...' -X 'main.BuildDate=...' -X 'main.OfficialSite=...'"
```

### Docker image tags
| Trigger | Tags |
|---|---|
| Any push (all branches) | `devel`, `{commit_id}` |
| Push to beta | `devel`, `beta`, `{commit_id}` |
| Version tag | `{version}`, `latest`, `YYMM` |

Registry: `ghcr.io` (GitHub) or auto-detected from server URL (Gitea/Forgejo self-hosted); GitLab uses `$CI_REGISTRY`.

### GitHub → Gitea → Forgejo variable mapping
| GitHub | Gitea | Forgejo |
|---|---|---|
| `${{ github.* }}` | `${{ gitea.* }}` | `${{ forgejo.* }}` |
| `GITHUB_ENV` | `GITEA_ENV` | `FORGEJO_ENV` |
| `GITHUB_OUTPUT` | `GITEA_OUTPUT` | `FORGEJO_OUTPUT` |
| `github.token` | `secrets.GITEA_TOKEN` | `secrets.FORGEJO_TOKEN` |

### GitLab CI structure
- Single `.gitlab-ci.yml`, stages: `build`, `test`, `package`, `release`, `docker`
- `image: $BUILD_IMAGE` = `casjaysdev/go:latest` for all Go jobs
- Matrix via 8 separate `build:{os}-{arch}` jobs (GitLab has no native OS/arch matrix like GitHub `strategy.matrix`), each gated by `rules: - if: $CI_COMMIT_TAG =~ /^v?\d+\.\d+\.\d+/` for release, or branch rules for beta/daily
- Release via `release:` stage using `registry.gitlab.com/gitlab-org/release-cli`

For complete details, see AI.md PART 27.
