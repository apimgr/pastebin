# TODO.AI.md

- [ ] `.github/workflows/ci.yml` line 48 (test job) and line 133 (coverage-report job): `go test` invocations are missing `-buildvcs=false`. The mounted `.git` directory in the `casjaysdev/go:latest` container requires this flag on all Go build/test commands (PART 25); currently only `go build` has it. Add `-buildvcs=false` to both `go test` commands.
