# TODO.AI.md

## Browser E2E Testing Implementation

### Implement browser E2E testing tiers 2 and 3
Read: AI.md PART 28

AI.md requires three E2E tiers behind the `e2e` build tag. Tier 1 is
implemented, but `TestTier2NoJS` and `TestTier3FullBrowser` in
`tests/e2e/e2e_test.go` remain `t.Skip` stubs. Add
`github.com/chromedp/chromedp` and implement the no-JavaScript browser tier
using `emulation.SetScriptExecutionDisabled`, plus the full-browser tier with
zero console errors. The dependency addition previously required explicit
permission; the parent must approve it before implementation.

---

## Audit Findings (from AUDIT.AI.md)

### PART 32 Client: Unreachable exit code
Read: AI.md PART 32

`exitConfig = 2` in src/client/main.go:80 is declared but never referenced;
all config load failures degrade to a warning and exit 0, so PART 32's
documented "2 = configuration error" exit code is unreachable.

### PART 32 Client: Orphaned default config values
Read: AI.md PART 32

`defaultCLIConfig()` writes `output.pager`, `output.quiet`, `output.verbose`,
`logging.*`, `cache.*`, `tui.{theme,mouse,unicode}`, `update.auto`,
`defaults.*` that no code path ever reads.

### PART 22 Updater: Windows orphaned binary deletion
Read: AI.md PART 22

Windows `replaceBinary` (src/updater/updater_windows.go:12-35) never schedules
`MOVEFILE_DELAY_UNTIL_REBOOT` deletion of the `.old` file; every update
orphans a copy of the previous binary.

### PART 22 Updater: Infinite update loop
Read: AI.md PART 22

`--update yes` is an infinite loop. src/main.go:611-633 calls
`updater.RestartSelf()`, which re-execs the same argv (`pastebin --update yes`)
on Unix and spawns `pastebin --update yes` on Windows. The server never starts.

### PART 22 Updater: Version tag mismatch
Read: AI.md PART 22

`Version` can never equal `rel.TagName`. release.yml stamps `VERSION` from
`release.txt` without a `v` prefix, but publishes `tag_name: v1.0.0`. The
comparison at src/updater/updater.go:88/98 is `rel.TagName == currentVersion`.

### PART 22 Updater: Channel match failures
Read: AI.md PART 22

`beta` and `daily` channels match nothing. `matchesBranch` keys on tag shape
(14-char timestamp / `-beta` suffix), but daily.yml publishes `tag_name: daily`
and beta.yml tags with `cat release.txt` (`1.0.0`, no `-beta`).

### PART 21 Backup: Missing restore version check
Read: AI.md PART 21

Restore verification never compares `Manifest.AppVersion` against the running
version, though spec AI.md:30798 requires a version-compatibility check.

### PART 21 Backup: Hourly backup disk pre-check
Read: AI.md PART 21

Hourly backup path skips the disk pre-check — src/task/task.go:497 runs only
`compliancePreCheck`, while the daily path runs a full disk pre-check.
