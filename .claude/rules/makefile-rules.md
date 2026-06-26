# Makefile Rules (PART 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Makefile is LOCAL DEV ONLY — NEVER used in CI/CD
| Command | Purpose | Output |
|---------|---------|--------|
| `make dev` | Development build | `${TMPDIR}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/` |
| `make local` | Production test build | `binaries/` (with version) |
| `make build` | Full release build | `binaries/` (all 8 platforms) |
| `make test` | Unit tests + coverage | Coverage report |

- NEVER: `go build ...` locally (use `make dev`)
- NEVER: `go test ...` locally (use `make test`)
- NEVER: use Makefile commands in CI/CD — use explicit commands with all env vars

## Docker Build Pattern (Makefile internal)
- Image: `casjaysdev/go:latest`
- Mount: `-v $PWD:/app` (NOT `$(pwd)`)
- Required: `-e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false`
- Coverage dir: `/tmp/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/` (NEVER in project tree)
- `$PWD` in shell commands, `$(PWD)` in Makefile variables

## Env Vars Required in Build
```
CGO_ENABLED=0
GOFLAGS=-buildvcs=false
GOOS={os}
GOARCH={arch}
```

## LDFLAGS Template
```
-s -w \
  -X 'main.Version=${VERSION}' \
  -X 'main.CommitID=${COMMIT_ID}' \
  -X 'main.BuildDate=${BUILD_DATE}' \
  -X 'main.OfficialSite=${OFFICIALSITE}'
```

---
For complete details, see AI.md PART 25
