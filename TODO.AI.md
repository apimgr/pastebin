## [ ] Add GitHub Actions build.yml workflow

`.github/workflows/build.yml` is required (spec: build, test, lint parallel gates). Currently only beta, daily, docker, release, and security workflows exist.

Read: AI.md PART 27

## [ ] Add Gitea workflow files

`.gitea/workflows/build.yml`, `.gitea/workflows/release.yml`, `.gitea/workflows/security.yml` are required for multi-provider CI/CD.

Read: AI.md PART 27

## [ ] Add Forgejo workflow files

`.forgejo/workflows/build.yml`, `.forgejo/workflows/release.yml`, `.forgejo/workflows/security.yml` are required for multi-provider CI/CD.

Read: AI.md PART 27

## [ ] Add GitLab CI file

`.gitlab-ci.yml` with stages: build, test, security, release is required for multi-provider CI/CD.

Read: AI.md PART 27

## [ ] Implement src/swagger/ package

`src/swagger/` is REQUIRED per spec: swagger.go (handler + spec generation), annotations.go, theme.go. OpenAPI/Swagger docs must be served at `/api/v1/server/swagger`.

Read: AI.md PART 14

## [ ] Implement src/graphql/ package

`src/graphql/` is REQUIRED per spec: graphql.go, schema.go, resolvers.go, theme.go. GraphQL endpoint at `/graphql` with `paste(id)` and `pastes(page, limit)` queries.

Read: AI.md PART 14

## [ ] Implement i18n package

`src/common/i18n/` with 7 locale files (en, es, fr, de, zh, ar, ja) embedded in binary. Required per spec.

Read: AI.md PART 30

## [ ] Implement --service CLI command

`--service {start,restart,stop,reload,--install,--uninstall,--disable,--help}` is currently stubbed out. Full implementation required.

Read: AI.md PART 8

## [ ] Implement --maintenance CLI command

`--maintenance {backup,restore,update,mode,setup,--help}` is currently stubbed out. Full implementation required.

Read: AI.md PART 21

## [ ] Implement --update CLI command

`--update [check|yes|branch {stable|beta|daily}]` is currently stubbed out. Full implementation required.

Read: AI.md PART 22

## [ ] Implement --daemon CLI command

`--daemon` (daemonize, detach from terminal) is currently a no-op. Full implementation required.

Read: AI.md PART 8

## [ ] Add src/common/banner/ package

`src/common/banner/banner.go` for responsive startup banner. Required per spec directory structure.

Read: AI.md PART 0

## [ ] Add src/common/display/ package

`src/common/display/detect.go` for terminal/display detection. Required per spec directory structure.

Read: AI.md PART 0

## [ ] Add src/common/theme/ package

`src/common/theme/colors.go` for shared color palette. Required per spec directory structure.

Read: AI.md PART 16

## [ ] Implement --lang CLI flag

`--lang CODE` (language for output) is currently a no-op. Full i18n integration required.

Read: AI.md PART 30

## [ ] Add unit tests

All packages have 0% coverage. Add unit tests for database, handler, and server packages.

Read: AI.md PART 28

## [ ] Add src/data/ embedded data files

`src/data/` directory for static JSON files embedded in binary (e.g., GeoIP data, language list for Chroma).

Read: AI.md PART 0

## [ ] Implement GeoIP support

GeoIP detection required by spec (PART 19). Currently not implemented.

Read: AI.md PART 19

## [ ] Implement metrics endpoint

Prometheus `/metrics` endpoint required by spec (PART 20). Currently not implemented.

Read: AI.md PART 20

## [ ] Implement Tor hidden service support

Tor is installed in the container (PART 31). Auto-enable when Tor binary found. Currently not implemented.

Read: AI.md PART 31
