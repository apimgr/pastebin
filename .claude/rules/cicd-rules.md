# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CI/CD Basics
- NEVER use Makefile in CI — use explicit commands with all env vars
- All third-party Actions MUST be pinned to a full commit SHA (never `@v4`, `@main`)
- `pull_request_target` is FORBIDDEN for untrusted code builds/tests
- Fork PRs NEVER receive secrets, write tokens, or deployment credentials
- Secret scanning (truffleHog) runs on every push — NEVER gitleaks (requires commercial license)

## Workflow Files Required
| Provider | Location |
|----------|---------|
| GitHub | `.github/workflows/ci.yml`, `release.yml`, `daily.yml`, `beta.yml`, `docker.yml`, `security.yml` |
| GitLab | `.gitlab-ci.yml` |
| Gitea | `.gitea/workflows/ci.yml`, `release.yml`, `daily.yml`, `beta.yml`, `docker.yml` |
| Forgejo | `.forgejo/workflows/ci.yml`, `release.yml` |
| Jenkins | `Jenkinsfile` |

## GitHub Actions Job Order (ci.yml)
- Parallel: `lint`, `test`, `secret-scan`, `workflow-policy`, `vuln-check`
- Then: `build` (needs: lint, test)
- Then: `coverage`, `image-scan`, `upload-artifacts` (need: build)

## Go build in CI (NO `container:` directive)
- Use `actions/setup-go` with `go-version-file: 'go.mod'` and `cache: true`
- DO NOT use `container: image: casjaysdev/go:latest` — Docker Hub unreliable on GitHub runners
- All build steps: `CGO_ENABLED: "0"` env var explicitly set

## Permissions
- Workflow default: `contents: read` (read-only baseline)
- Release job only: `contents: write`, `packages: write`, `id-token: write`, `attestations: write`

## Pinned Action SHAs (current)
| Action | SHA | Version |
|--------|-----|---------|
| `actions/checkout` | `9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0` | v7.0.0 |
| `actions/setup-go` | `924ae3a1cded613372ab5595356fb5720e22ba16` | v6.5.0 |
| `actions/upload-artifact` | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` | v7.0.1 |
| `actions/download-artifact` | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` | v8.0.1 |
| `softprops/action-gh-release` | `718ea10b132b3b2eba29c1007bb80653f286566b` | v3.0.1 |
| `trufflesecurity/trufflehog` | `30d5bb91af1a771378349dbbb0c82129392acf70` | v3.95.6 |

## Docker Image Tags
- Any push → `devel`, `{commit}`
- Beta branch → adds `beta`
- Tag → `{version}`, `latest`, `YYMM`, `{commit}`

## VERSION Precedence
- `release.txt` wins when present
- Fallback: tag name, beta timestamp, or daily timestamp per workflow

---
For complete details, see AI.md PART 27
