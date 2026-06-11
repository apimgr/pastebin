# TODO

Spec compliance fixes from audit — ordered by impact.

## PART 14 API routes (server.go)
- [ ] B5.5 Remove `/healthz` and `/health` unconditional mounts; gate on `server.healthz.root.enabled`
- [ ] B5.7 Remove root `/graphql` mount (spec explicitly removed it)
- [ ] B5.2 Add `/api/healthz` unversioned alias (same handler as versioned)
- [ ] B5.3 Add `/api/v1/server/graphql` versioned route

## PART 8 Binary CLI (main.go)
- [ ] B7.2 Fix `--status` to probe live server via HTTP; exit 0=healthy, 1=unhealthy
- [ ] B8.4 Fix `--version` output: add `Go:` and `OS/Arch:` lines

## PART 26 Docker (docker-compose.yml + entrypoint.sh)
- [ ] B2.9  docker-compose.yml: `restart: unless-stopped` → `restart: always`
- [ ] B2.11 docker-compose.yml: `container_name: pastebin` → `container_name: pastebin-app`
- [ ] B2.13 docker-compose.yml: fix healthcheck timing to match spec (interval: 10s timeout: 5s start_period: 90s)
- [ ] B2.14 Remove `build:` directives from docker-compose.dev.yml and docker-compose.test.yml
- [ ] B2.17 entrypoint.sh: remove Tor management (binary owns Tor per PART 31/26)
- [ ] B2.18 entrypoint.sh: use `exec` to swap to binary as PID 1

## PART 32 Client (src/client/main.go)
- [ ] B9.6 Fix `versionLessThan` — string compare is broken for multi-digit semver

## PART 25 Makefile
- [ ] B1.2 Fix Go cache mounts: named volume `go-state` → bind-mount host dirs per spec template
- [ ] B1.4 Add `help` target; add to .PHONY

## Housekeeping
- [ ] Delete AUDIT.AI.md (spec says delete when all resolved; misleadingly large)
