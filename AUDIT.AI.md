# Project Audit

Started: 2026-05-24
Last reconciled: 2026-05-24 (against current AI.md after the recent rewrite)
Scope: ALL PARTs of AI.md EXCEPT PART 27 (CI/CD Workflows — explicitly out of scope).

## Pass 1: Security

(no Pass 1 violations found — Argon2id N/A here, no bcrypt/md5/sha1/math-rand used, `crypto/rand` confirmed for paste IDs and delete tokens, SHA-256 for delete-token storage. Constant-time compare on delete-token verification needs a code-walk — tracked in TODO.AI.md "Document threat model".)

## Pass 2: Code Quality

- [ ] `src/main.go`: `--shell completions|init` stubbed (`fmt.Fprintf(..., "--shell is not yet implemented")`) — TODO item open
- [x] `src/main.go`: `--backup DIR` and `--pid FILE` flags wired into `paths.GetBackupDir()` and `paths.GetPIDFile()`; the directory is created and the path is surfaced in `--status` output. (Stale-PID write/verify/remove still missing — separate TODO item.)

## Pass 3: Logic and Correctness

- [x] `src/paths/paths.go`: `isContainer()` uses stdlib `strings.Contains`
- [x] `src/paths/paths.go`: `os.Geteuid() == 0` wrapped in cross-platform `isRoot()` (returns false on Windows)
- [x] `src/paths/paths.go`: `EnsureDir` permissions 0700 (user) / 0755 (root) per PART 8
- [x] `src/paths/paths.go`: macOS user data path no longer appends `/data` (PART 4 spec: `~/Library/Application Support/{project_org}/{internal_name}/`)
- [x] `src/paths/paths.go`: Windows user data path no longer appends `/data` (PART 4 spec: `%LocalAppData%\{project_org}\{internal_name}\`)
- [x] `src/paths/paths.go`: added `GetBackupDir(appName)` and `GetPIDFile(appName)` helpers, container-aware and root/user-aware per PART 4 / PART 26
- [x] `src/main.go`: `--port` help text now matches PART 8 spec (`random 64xxx, 80 in container`) instead of the old `3010` literal

## Pass 4: Documentation Completeness

- [ ] `LICENSE.md` missing third-party entries for ~17 dependencies — open TODO item
- [ ] `docs/` filenames diverge from PART 29 canonical names (`install.md` vs `installation.md`, `config.md` vs `configuration.md`); `integrations.md`, `development.md`, `stylesheets/{dark,light}.css` missing — open TODO item
- [ ] No `man/{project_name}.1` page; PART 24 service docs reference one
- [ ] README.md missing an "Environment Variables" section — open TODO item

## Pass 5: Spec and Rules Compliance

### Project structure (PART 3)

- [ ] `.claude/rules/` directory missing entirely — PART 0 mandates 13 cheatsheet files (only `.claude/plans/` exists) — open TODO item
- [ ] `tests/` missing `docker.sh` and `incus.sh` (both REQUIRED per PART 3) — open TODO item
- [ ] `.gitignore` and `.dockerignore` formatting drift from PART 3 template — informational, not blocking

### Paths (PART 4 / PART 26)

- [x] `src/paths/paths.go`: container paths corrected to PART 26 spec
- [x] `src/paths/paths.go`: Linux user logs corrected to PART 4 (`~/.local/log/`)
- [x] `src/paths/paths.go`: macOS/Windows user data paths corrected to PART 4 (no extra `/data` suffix)
- [x] `src/paths/paths.go`: `GetBackupDir()` + `GetPIDFile()` helpers added
- [ ] PID file write/verify/remove logic still missing (the helper returns the right path but `main.go` does not write/check it) — open TODO item

### Server CLI (PART 8)

- [ ] Default port still `"3010"` in `src/config/config.go`; PART 8 default is "random 64xxx, 80 in container" with persistence to `server.yml` — open TODO item
- [ ] `--shell completions|init` not implemented (canonical command per PART 8) — open TODO item
- [ ] `--clean-expired` is a project-specific extension beyond PART 8 — acceptable as an additive convenience flag; document it under "Maintenance" in `--help` (already present)
- [ ] PID file handling: helper present, write/check/remove missing — open TODO item
- [ ] Live config reload not implemented (PART 5 "Live Reload" + PART 8 "Smart Config Reload") — open TODO item

### Scheduler (PART 18)

- [ ] `src/scheduler/scheduler.go` is interval-only (no cron syntax, no persistent state, no catch-up window) — open TODO item
- [ ] Only `expire-pastes` (project-specific) and `geoip-update` are registered; PART 18 requires `ssl_renewal`, `geoip_update`, `blocklist_update`, `cve_update`, `log_rotation`, `backup_daily`, `backup_hourly` (disabled), `healthcheck_self`, `tor_health` — open TODO item

### Client (PART 32)

- [ ] `src/client/main.go` has hardcoded `defaultServer = "http://localhost:8080"`; PART 32 says compiled default is only valid if IDEA.md specifies one — IDEA.md does not — open TODO item

### Docker (PART 26)

- [ ] `docker/Dockerfile.dev` missing (REQUIRED) — open TODO item
- [ ] `docker/Dockerfile.build` missing (REQUIRED) — open TODO item
- [ ] `docker/docker-compose.dev.yml` missing (REQUIRED) — open TODO item
- [ ] `docker/docker-compose.test.yml` missing (REQUIRED) — open TODO item
- [ ] `docker/Dockerfile` `RUN mkdir -p /config /data/db /data/logs /data/tor /data/geoip /data/backup` — paths do not match PART 26 (`/config/pastebin/`, `/data/pastebin/`, `/data/log/pastebin/`, `/data/db/sqlite/`, `/data/backups/pastebin/`) — open TODO item
- [ ] `docker/Dockerfile` missing `git` from `apk add` (PART 26 required packages: `git, curl, bash, tini, tor`) — open TODO item
- [ ] `docker/Dockerfile` carries two LABEL blocks; PART 26 says all metadata is OCI build-time annotations, never LABEL — open TODO item
- [ ] `docker/docker-compose.yml` mounts `./rootfs/config/...` and `./rootfs/data/...`; PART 26 requires `./volumes/config:/config:z` and `./volumes/data:/data:z` — open TODO item

### Documentation (PART 2, PART 29)

- [ ] LICENSE.md third-party attributions incomplete — open TODO item
- [ ] `docs/` filenames/missing files vs PART 29 canonical layout — open TODO item

## Pass 6: Code Flow Trace

- [ ] `src/server/server.go`: native route shape mismatch with IDEA.md — IDEA.md says `POST/GET/DELETE /api/v1/paste{/...}` (singular) and only `/api/v1/pastes` for the list endpoint. Code uses `/pastes` for both, and lacks the `/api/v1/paste/{id}/raw` JSON-side raw endpoint. README.md already documents the IDEA.md shape — code is the wrong side. — open TODO item
- [ ] `src/server/server.go`: missing `POST /` lenpaste form-POST root handler (IDEA.md "lenpaste routes") — open TODO item
- [ ] `src/server/server.go`: lenpaste response shapes (`/api/v1/new`, `/api/v1/get` with `?openOneUse=true` semantics, `/api/v1/getServerInfo`) need verification against IDEA.md — open TODO item
- [ ] `src/server/server.go`: pastebin.com `/api/api_post.php` `api_option` dispatch — must handle all four (`paste`, `list`, `delete`, `userdetails`) — needs verification — open TODO item
- [ ] Environment-variable audit: `CONFIG_DIR`, `DATA_DIR`, `LOGS_DIR`, `CACHE_DIR`, `BACKUP_DIR`, `PID_FILE`, `PORT`, `ADDRESS`, `BASE_URL`, `DB_PATH`, `SITE_TITLE`, `THEME`, `MAX_SIZE_BYTES`, `NO_COLOR`, `_DAEMON_CHILD`, `PASTEBIN_SERVER`, `SSH_CLIENT`, `SSH_TTY`, `MOSH`, `STY`, `TMUX`, `TERM`, `XDG_*`, `APPDATA`, `LOCALAPPDATA`, `ProgramData` — none of these are documented in README.md — open TODO item
- [ ] Delete-token verification path: code stores the SHA-256 hash and compares; both sides are hashed at compare time, so plain SQL `=` does not leak the plaintext token. Confirmed safe — no TODO needed for the verification itself. The constant-time-compare claim in IDEA.md still needs to be reconciled with the actual implementation — folded into the "Document threat model" TODO.

## Completed (fixed inline this pass)

- `src/paths/paths.go`: container paths corrected to PART 26; Linux user logs to PART 4; macOS/Windows user data paths to PART 4 (no extra `/data` suffix); added `GetBackupDir()` and `GetPIDFile()` helpers.
- `src/main.go`: `--backup` and `--pid` flags now feed `paths.GetBackupDir()` / `paths.GetPIDFile()`, backup dir is created at startup, PID file's parent dir is created, both surfaced in `--status` output.
- `src/main.go`: `--port` help text aligned with PART 8 (`random 64xxx, 80 in container`).
