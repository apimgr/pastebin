# Testing Rules (PART 28, 29, 30)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER run binaries directly on the local machine — Go is not installed locally; all builds/tests/execution happen in Docker or Incus
- NEVER run `go build` / `go test` / `binaries/pastebin` / `$BUILD_DIR/pastebin` on the host
- NEVER use project directory for test/runtime data — ALL runtime/test data goes to `/tmp/{project_org}/{internal_name}-XXXXXX/`
- NEVER use bare `/tmp` root, `/tmp/{project_name}` (missing org), `mktemp -d` without org/project structure, or generic paths like `/tmp/test-data`
- NEVER (as AI) run `docker compose up` with `docker/docker-compose.yml` or `docker/docker-compose.dev.yml` — human-only files
- NEVER (as AI) mount `./volumes/`, `./docker/rootfs/`, or any project-directory path as a runtime volume
- NEVER create or modify files in the project directory during testing
- NEVER commit config files (`server.yml`, `cli.yml`) to the repo — they are runtime-generated only
- NEVER skip either test phase — Phase 1 (`make test`, ≥60% coverage) and Phase 2 (`./tests/*.sh`, 100% endpoint coverage) are both REQUIRED
- NEVER defer unit tests to the end — create/update `*_test.go` in the same work pass as the logic change
- NEVER test full HTTP requests, DB operations, external services, or auth flows as Go unit tests — those belong in `./tests/*.sh` integration tests
- NEVER use `pkill -f`, `pkill {name}` (no `-x`), `killall`, `kill -9` first-try, `docker kill`, `docker rm $(docker ps -aq)`, `docker system prune`, or any broad-sweep command
- NEVER kill/remove anything by pattern — always exact PID/name/tag after verification
- NEVER delete user data, config files, database files, SSL certs, or git repos without explicit confirmation
- NEVER hardcode any human-readable string — every UI/API/CLI/email string MUST use a translation key (`t()`/`{{t .Lang key}}`)
- NEVER let missing/unsupported language error or crash — always silently fall back to `en`
- NEVER convey information by color alone (accessibility)
- NEVER put non-ReadTheDocs files in `docs/` — it is ONLY for MkDocs documentation files

## CRITICAL - ALWAYS DO

- ALWAYS run host-affecting test steps (`systemctl`, `mount`, `iptables`, reboot, package install) inside a container/VM/netns — never on the host
- ALWAYS use `/tmp/{project_org}/{internal_name}-XXXXXX/` structure for all temp/test dirs; detect org/project from git remote or path
- ALWAYS prefer `docker/docker-compose.test.yml` via `tests/run_tests.sh` / `tests/docker.sh` for AI-driven testing; fallback is copy-to-tempdir only when no `tests/` script exists yet
- ALWAYS build via `make build` when a Makefile exists; output lands in `binaries/`
- ALWAYS run both test phases before release: Phase 1 `make test` (pre-commit gate, required) and Phase 2 `./tests/incus.sh` / `./tests/run_tests.sh` (manual, developer-initiated)
- ALWAYS create/update the matching `*_test.go` immediately when adding/changing package logic
- ALWAYS test every route with all applicable Accept headers (`text/html`+`text/plain` for frontend, `application/json`+`text/plain` for API) and every `.txt` endpoint
- ALWAYS test full CRUD if the project has CRUD operations
- ALWAYS identify the exact PID/container/image before any kill/remove operation, and act on that exact target only
- ALWAYS use `trap` for cleanup in test scripts; scripts exit 0 on success, non-zero on failure
- ALWAYS translate every human-facing string across web frontend, API responses, Swagger/GraphQL descriptions, email templates, CLI/agent output, and legal pages
- ALWAYS fall back missing translation keys to English; fall back unsupported `--lang`/`Accept-Language` to English silently
- ALWAYS meet WCAG 2.1 AA: 4.5:1 text contrast, visible focus indicators, 44×44px touch targets, full keyboard nav, skip links as first focusable elements
- ALWAYS host documentation on ReadTheDocs via MkDocs Material with dark/light/auto theme toggle (dark default)
- ALWAYS keep `docs/` covering the actual shipped product surface: browser, CLI, API, config, and any enabled well-known/discovery endpoints

## Key Rules Summary

### Testing phases (PART 28)

| Phase | Files | Run With | Enforces |
|-------|-------|----------|----------|
| 1 — Toolchain Gate | `*_test.go` | `make test` (Docker) | ≥60% Go code coverage; pre-commit gate |
| 2 — Binary Validation | `./tests/*.sh` | `./tests/run_tests.sh` | 100% endpoint/route coverage; manual, developer-initiated |

Required scripts in `tests/`: `run_tests.sh` (auto-detect runtime), `docker.sh` (alpine, fallback), `incus.sh` (debian+systemd, preferred). All must be executable, WTFPL-licensed, use `set -eo pipefail`, install `curl bash file jq` in test containers, build via `make build`/casjaysdev/go:latest with host `GO_CACHE`/`GO_BUILD` mounts, test binary rename (`--help` shows actual invoked name), test operator token auth, and clean up via `trap`.

### Temp directory structure

Only acceptable pattern: `/tmp/{project_org}/{internal_name}-XXXXXX/` (e.g. `/tmp/apimgr/pastebin-k9mN2p/`). Never bare `/tmp`, never missing org, never generic names.

### Process/container safety

| Do | Don't |
|----|-------|
| `kill {pid}` after verifying via `pgrep -la` | `pkill -f`/`killall` |
| `docker stop/rm {project_name}` (exact name) | `docker rm $(docker ps -aq)` |
| `docker rmi {org}/{name}:tag` (exact tag) | `docker system/image/volume prune` |
| `rm -rf $BUILD_DIR`/`$TEST_DIR` (from mktemp) | `rm -rf /`, `~`, `.`, `*` |

### Docs (PART 29)

Required root files: `mkdocs.yml`, `.readthedocs.yaml`. Required `docs/` pages: `index.md`, `installation.md`, `configuration.md`, `api.md`, `cli.md` (if applicable), `security.md`, `integrations.md`, `development.md`, `requirements.txt`. Theme: MkDocs Material, `scheme: slate` (dark, default) / `scheme: default` (light) / auto — matches PART 16 theme rules. RTD URL is `https://{project_org}-{project_name}.readthedocs.io` or `https://{project_name}.readthedocs.io` depending on RTD project setup.

### I18N (PART 30)

- Encoding: UTF-8 everywhere. Default language: `en`.
- Fallback chain: `?lang=` query param (sets 1-year cookie) → `lang` cookie → `Accept-Language` header → `en`.
- CLI/server binaries: `--lang` flag → config file `lang:` → `LC_ALL`/`LANG` env → `en`.
- Supported languages (all binaries, no partial support): `en`, `es`, `zh`, `fr`, `ar` (rtl), `de`, `ja`.
- Translation files: `src/common/i18n/locales/{lang}.json`, embedded via `go:embed`, shared by server and CLI. Build-time validation ensures every language has the same keys as `en.json`.
- Key naming: dot-separated lowercase (`health.status.title`); interpolation via `{variable}`; plurals nested under `zero/one/two/few/many/other`.

### Accessibility (PART 30)

| Requirement | Standard |
|--------------|----------|
| WCAG compliance | 2.1 AA, mandatory |
| Color contrast | 4.5:1 normal text, 3:1 large text/UI components |
| Touch targets | 44×44px minimum |
| Keyboard | All functionality operable via keyboard |
| Screen readers | Full NVDA/JAWS/VoiceOver support |

Required patterns: skip links as first focusable elements, ARIA live regions for dynamic content, `role="dialog"`+focus trap for modals, landmark roles (`banner`/`navigation`/`main`/`complementary`/`contentinfo`), associated form labels with `aria-describedby`/`aria-required`, `.sr-only` class for screen-reader-only text. Test with axe DevTools, WAVE, Lighthouse, and manual keyboard-only navigation.

For complete details, see AI.md PART 28, 29, 30.
