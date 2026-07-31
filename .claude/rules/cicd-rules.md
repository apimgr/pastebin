# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- NEVER use Makefile targets inside CI/CD workflows — commands must be explicit (`go build ...` with all flags visible), unlike local dev which always uses Makefile targets
- NEVER reference local user paths like `~/.local/share/go` in CI — use `/tmp/` or CI-native caching
- NEVER depend on local Docker containers for the build step on GitHub/Gitea Actions — they use native `casjaysdev/go:latest` container images, not host Docker
- NEVER cross-cancel different release refs — concurrency auto-cancel may only cancel an older run for the *exact same* branch/tag ref (e.g. a `v1.2.4` run must never cancel `v1.2.3`)
- NEVER install tools inline in `ci.yml` jobs (no `apk add`, no `go install`) — all tooling (git, bash, govulncheck, cyclonedx-gomod, staticcheck, etc.) is pre-installed in `casjaysdev/go:latest`
- NEVER use `github.event.before`/`default_branch` incorrectly for secret-scan range — never use `default_branch` (resolves to HEAD after push, silently skips the scan); use `github.event.before` / `github.event.after`
- NEVER create a `build-toolchain.yml` or `ensure-build-image` gate for Go projects — `casjaysdev/go:latest` is externally maintained
- NEVER include `-musl` suffix in binary names
- NEVER skip building any of the 8 required platform/arch combinations

## CRITICAL - ALWAYS DO
- ALWAYS use Docker `casjaysdev/go:latest` as the toolchain image in both local dev and CI/CD (only the build *commands* differ, not the image)
- ALWAYS use CI-native caching (not local host cache paths) in CI/CD
- ALWAYS set `VERSION`, `COMMIT_ID`, `BUILD_DATE` explicitly in every workflow via a "Set build info" step (never as static `env:`)
- ALWAYS build all platforms in a matrix (8 total: linux/darwin/windows/freebsd × amd64/arm64)
- ALWAYS auto-cancel older in-progress runs for the same ref on push workflows targeting `main`, `master`, `devel`, `dev`, or `beta` via `concurrency:` + `cancel-in-progress`
- ALWAYS apply the same auto-cancel rule per-exact-tag for tag-only release workflows
- ALWAYS run all `ci.yml` jobs inside `container: image: casjaysdev/go:latest`
- ALWAYS pin third-party GitHub Actions to a full commit SHA (never a tag) — see SHA table below
- ALWAYS build the CLI binaries (`-cli` suffix) only when `src/client/` exists (`hashFiles('src/client/') != ''` / `[ -d src/client ]`)
- ALWAYS require `ci.yml` and `release.yml` on every project; `beta.yml`/`daily.yml`/`docker.yml` are optional, project-specific
- ALWAYS enforce the Go test coverage threshold (60%) in `ci.yml`
- ALWAYS include `secret-scan`, `workflow-policy`, `vuln-scan`, `image-scan` security jobs in `ci.yml`, running on push/PR and weekly cron (`0 6 * * 1`); add `if: github.event_name != 'schedule'` to skip non-security jobs on scheduled runs
- ALWAYS use truffleHog (Apache-2.0) for mandatory secret scanning on every public repo
- ALWAYS require Jenkins agent labels `amd64` AND `arm64` to be available, with Docker + buildx on the amd64 runner

## Key Rules Summary

### Workflow files by provider

| File | Trigger | Required? |
|------|---------|-----------|
| `ci.yml` | push/PR to default branch + weekly cron (security jobs) | Required, all providers |
| `release.yml` | tag push `v*` / `*.*.*` | Required, all providers |
| `beta.yml` | push to `beta` branch | Optional |
| `daily.yml` | daily 3am UTC + push to main/master | Optional |
| `docker.yml` | any push (all branches) + version tags | Optional |
| `.gitlab-ci.yml` | single file, stages: build/test/package/release/docker | Required for GitLab |
| `Jenkinsfile` | pipeline stages matching above triggers | Required for Jenkins |

### Config locations by provider

| Provider | Directory | Self-hosted |
|----------|-----------|--------------|
| GitHub | `.github/workflows/*.yml` | No (github.com only) |
| Gitea | `.gitea/workflows/*.yml` | Yes |
| Forgejo | `.forgejo/workflows/*.yml` (or `.gitea/workflows/`) | Yes (self-hosted only) |
| GitLab | `.gitlab-ci.yml` | Yes |
| Jenkins | `Jenkinsfile` | Yes |

### SHA-pinned actions used in this spec (never use tags)

| Action | SHA | Version comment |
|--------|-----|------------------|
| `actions/checkout` | `9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0` | v7.0.0 |
| `actions/upload-artifact` | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` | v7.0.1 |
| `actions/download-artifact` | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` | v8.0.1 |
| `softprops/action-gh-release` | `718ea10b132b3b2eba29c1007bb80653f286566b` | v3.0.1 |
| `docker/setup-qemu-action` | `06116385d9baf250c9f4dcb4858b16962ea869c3` | v4.1.0 |
| `docker/setup-buildx-action` | `d7f5e7f509e45cec5c76c4d5afdd7de93d0b3df5` | v4.1.0 |
| `docker/login-action` | `650006c6eb7dba73a995cc03b0b2d7f5ca915bee` | v4.2.0 |
| `docker/build-push-action` | `f9f3042f7e2789586610d6e8b85c8f03e5195baf` | v7.2.0 |

Note: verify/refresh these SHAs against upstream before reuse — pins age.

### Build info variables (every workflow)

Set in a "Set build info" step (not static `env:`):
- `VERSION` — from `release.txt` if present, else derived from ref/date
- `COMMIT_ID` — `git rev-parse --short HEAD`
- `BUILD_DATE` — `date +"%a %b %d, %Y at %H:%M:%S %Z"`
- `OFFICIAL_SITE` — `site.txt` wins, else secret/CI var, else empty (never guess)
- `LDFLAGS="-s -w -X 'main.Version=...' -X 'main.CommitID=...' -X 'main.BuildDate=...' -X 'main.OfficialSite=...'"`
- Build command: `go build -buildvcs=false -trimpath -ldflags "${LDFLAGS}" -o {name} ./src` (CI); local dev never uses `-buildvcs=false` trick without the Docker mount workaround

### Coverage threshold

- `THRESHOLD=60` — `go test -cover -coverprofile=...` then `go tool cover -func=... | awk '/^total:/'`; `exit 1` if `PCT < THRESHOLD`

### Docker image tags

| Trigger | Tags |
|---------|------|
| Any push (all branches) | `devel`, `{commit_id}` |
| Push to `beta` | `devel`, `beta`, `{commit_id}` |
| Version tag | `{version}`, `latest`, `YYMM` |

- Registry: `ghcr.io` (GitHub); auto-detected from server URL (Gitea/Forgejo); `$CI_REGISTRY_IMAGE` (GitLab); provider-specific (Jenkins)
- Platforms: `linux/amd64,linux/arm64` via `docker buildx`
- `{commit_id}` = 7-char short SHA; `YYMM` = e.g. `2512`
- Standard image: `docker/Dockerfile` (alpine, app binary only); Dev image: `docker/Dockerfile.dev` (alpine + debug tooling, `:devel` tag)
- OCI labels/annotations required on every image build: `org.opencontainers.image.{vendor,authors,title,base.name,description,version,created,revision,url,source,documentation,licenses=MIT}` (both `labels:` and `manifest:`-prefixed `annotations:`)

### Platform matrix (8 required builds)

`linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` (.exe), `windows/arm64` (.exe), `freebsd/amd64`, `freebsd/arm64` — CLI builds duplicate this matrix with `-cli` suffix only if `src/client/` exists.

### Variable mapping (GitHub → Gitea → Forgejo → GitLab)

| GitHub | Gitea | Forgejo | GitLab |
|--------|-------|---------|--------|
| `${{ github.* }}` | `${{ gitea.* }}` | `${{ forgejo.* }}` | `$CI_*` |
| `GITHUB_ENV` | `GITEA_ENV` | `FORGEJO_ENV` | `dotenv` artifact |
| `GITHUB_OUTPUT` | `GITEA_OUTPUT` | `FORGEJO_OUTPUT` | n/a |
| `GITHUB_REF_NAME` | `GITEA_REF_NAME` | `FORGEJO_REF_NAME` | `$CI_COMMIT_REF_NAME` |
| `github.token` | `secrets.GITEA_TOKEN` | `secrets.FORGEJO_TOKEN` | n/a |
| `github.sha` | — | — | `$CI_COMMIT_SHA` |
| `github.repository` | `gitea.repository` | `forgejo.repository` | `$CI_PROJECT_PATH` |
| `github.server_url` | `gitea.server_url` | `forgejo.server_url` | `$CI_SERVER_URL` |
| `secrets.NAME` | `secrets.NAME` | `secrets.NAME` | `$NAME` |

### Required tokens/permissions

| Provider | Token permissions |
|----------|--------------------|
| GitHub | `write:packages`, `read:packages`, `delete:packages` |
| Gitea/Forgejo | `package:write` |
| GitLab | `write_registry`, `read_registry` |
| Docker Hub | Read/Write |

### Daily build cleanup

- Delete previous `daily` tag/release before creating new one: `gh release delete daily --yes` + `git push origin :refs/tags/daily` (GitHub); Gitea API `DELETE .../releases/tags/daily` (Gitea/Forgejo)

For complete details, see AI.md PART 27
