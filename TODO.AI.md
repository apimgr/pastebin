## [ ] Add unit tests for core packages

All packages have 0% coverage. PART 28 requires unit tests for backend logic. At minimum add tests for:

- `src/database/` — paste CRUD, expired/burned cleanup, delete-token hash verification
- `src/handler/` — create paste (JSON + form + multipart), get, delete with token
- `src/config/` — config load, defaults, env overrides, `ParseBool`-equivalent
- `src/paths/` — container detection, root vs user, BackupDir, PIDFile resolution

Read: AI.md PART 28

## [ ] Complete LICENSE.md third-party attributions

PART 2 requires every dependency to be attributed. LICENSE.md currently lists 8 entries; go.mod has ~25 direct + indirect modules (cretz/bine, oschwald/maxminddb-golang, prometheus/client_golang and submodules client_model/common/procfs, golang.org/x/sys, x/term, x/net, x/text, beorn7/perks, cespare/xxhash, dlclark/regexp2, hashicorp/golang-lru, munnerz/goautoneg, ncruces/go-strftime, remyoudompheng/bigfmt, go.yaml.in/yaml, google.golang.org/protobuf, modernc.org/gc, libc, mathutil, memory, strutil, token). Add them all using the compact-table format from PART 2.

Read: AI.md PART 2

## [ ] Document threat model in IDEA.md mapping to actual code

IDEA.md "Threat model & abuse cases" lists defenses (rate-limit on create/delete, max paste size at HTTP layer, HTML escaping, `crypto/rand`, SHA-256 delete-token hash, constant-time compare, automatic cleanup). PART 11 "Public Endpoint Safety Principle" and the audit's Step 1 "Threat model / abuse model" check require each defense to be verifiably implemented. Walk the code and confirm — or open follow-up TODOs for any gap — for:

- Max paste size enforced at HTTP body layer BEFORE reading (not after)
- Rate limiter present on `POST /api/v1/paste`, `POST /create`, `POST /upload`, `POST /`, `POST /api/api_post.php`, `POST /api/v1/new`, and all delete endpoints
- Constant-time compare on delete-token verification path
- HTML escape on ALL user-controlled rendering (title, content rendered text, language label)

Read: IDEA.md "Threat model & abuse cases", AI.md PART 11

## [ ] Verify pastebin.com api_post.php dispatches all four api_option values

IDEA.md "pastebin.com API" specifies `POST /api/api_post.php` must handle `api_option` values: `paste`, `list`, `delete`, `userdetails`. Verify `CompatHandler.PastebinPost` dispatches all four and returns the expected XML for `list` (XML `<paste>` elements honoring `api_results_limit` 1–1000) and `userdetails` (stub XML user record, username `"anonymous"`).

Read: IDEA.md "pastebin.com API"

## [ ] Verify lenpaste response shapes match IDEA.md

- `POST /api/v1/new` MUST return `{"id":"..."}`
- `GET /api/v1/get` MUST return `{"id","title","body","syntax","createTime","deleteTime","oneUse"}` and, when `burn_after == 1` and `?openOneUse=true` is NOT set, return only `{"id":"...","oneUse":true}` with body withheld (matches lenpaste behaviour for one-time pastes)
- `GET /api/v1/getServerInfo` MUST return the exact shape: `version`, `titleMaxlength`, `bodyMaxlength`, `maxLifeTime` (-1), `serverAbout`, `serverRules`, `adminName`, `adminMail`, `syntaxes`

Read: IDEA.md "lenpaste routes"

## [ ] Implement stale-PID detection binary name check on non-Linux

`src/pid/pid_unix.go`: `isOurProcess()` falls back to `isOurProcessDarwin()` on macOS/BSD via `ps -p {pid} -o comm=`. The `comm=` format truncates to 15 characters on some systems, which may not match "pastebin" for this binary. Test on macOS and FreeBSD and adjust if needed.

Read: AI.md PART 8 ("PID File Handling")

## [ ] Implement full scheduler task bodies

The following PART 18 required tasks were registered with stub implementations:

- `ssl_renewal`: check cert expiry at `{config_dir}/ssl/letsencrypt/{fqdn}/`, renew 7 days before expiry
- `blocklist_update`: download/update IP/domain blocklists to `{data_dir}/security/blocklists/`
- `cve_update`: download/update CVE/security databases to `{data_dir}/security/cve/`
- `log_rotation`: rotate and compress logs older than `max_age` (default 30d), honour `max_size` (default 100MB)
- `backup_daily`: full backup to `{backup_dir}`, verify after creation, honour retention policy
- `backup_hourly`: hourly incremental backup (disabled by default)
- `tor_health`: check Tor connectivity, restart `cretz/bine` controller if unhealthy

Read: AI.md PART 18, PART 19, PART 20, PART 22, PART 31
