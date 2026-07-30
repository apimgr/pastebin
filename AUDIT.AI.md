# Project Audit

Started: 2026-07-27

All auto-fixable findings are resolved (see Completed). Only the items in
Flagged remain — they are design decisions that require operator input and
were intentionally NOT auto-changed.

## Flagged (design decisions — NOT auto-fixed)
- ~9 dead exported symbols (low-value churn; leave unless a cleanup pass is
  explicitly requested).
- src/main.go lines 64-188: hand-rolled CLI argument parsing (for-loop +
  switch, with a `normalizeArgs()` helper for `=`-form flags) instead of
  `flag`/`pflag`/`cobra` — flagged by go-lint. Functionally correct (all
  required flags/forms work) but violates the "never hand-roll" convention.
  Out of scope for the email-notification/cache work in this commit; needs a
  deliberate refactor pass since it touches every CLI flag in PART 8.
- cache Get/Set/Delete appeared unused. Operator decision: keep `Delete` per
  AI.md/IDEA.md. Not yet wired — implementation has not started; separate
  follow-up commit.

## Resolved (operator decisions)
- security.yml vs ci.yml consolidation: `.github/workflows/security.yml` was
  a forbidden duplicate — cicd-rules.md requires security jobs to live
  inside `ci.yml` with no separate `security.yml`, and `ci.yml` already ran
  `secret-scan`/`workflow-policy`/`vuln-scan`. Deleted `security.yml`
  (commit 356047ddf224); no `act` verification was needed since nothing was
  restructured, only the duplicate file removed. Pushed; CI and Daily Build
  confirmed green on main afterward (Docker Build still running at time of
  this note). A full spec
  sweep of `/server/security**` (handlers, templates, config, i18n, all 37
  related tests) found no other issues — feature is spec-compliant.
- task/task.go: the `update_available` scheduler email was dead — PART 17
  defined only 8 email templates and the call used CamelCase keys that never
  substituted against the snake_case renderer. Operator decision: PART 17 now
  documents 10 templates; added `update_available`/`update_installed` embedded
  templates, fixed the key casing to snake_case, added config-gated
  `notifyAvailable`/`notifyInstalled` toggles wired from
  `server.notifications.email.events.{update_available,update_installed}`,
  and added an `update_installed` send before `RestartSelf()`. Unit tests
  added in task/task_email_test.go via extracted `updateSendAvailable`/
  `updateSendInstalled` helpers.

## Completed
- config/config.go: Save() now writes server.yml atomically (temp + fsync +
  rename); the two ignored Save errors (generated token/key persistence) now
  log a warning instead of silently dropping.
- maintenance/maintenance.go: SetYAMLField and SetMode write server.yml
  atomically via a shared atomicWriteFile helper.
- ssl/ssl.go + ssl/selfsigned.go: cert/key pairs written key-first and each
  write is atomic (DNS-01 and self-signed overlay paths) — no torn cert/key.
- handler/paste.go: ParseExpiry default case guards against int64 nanosecond
  overflow that made large expiries wrap to immediate expiry.
- handler/paste.go: HashToken doc comment corrected (no server-layer caller;
  exported for the black-box package test).
- pid/pid_unix.go: signal(0) EPERM now treated as live-but-other-owner, not
  dead (fixes false "not running" during the root→pastebin privilege drop).
- docs/configuration.md: environment-variable table expanded to the full
  config-override set (server, identity, DB/cache, SMTP_*, dir overrides).
- docker/docker-compose{,.dev,.test}.yml: converted env from list style to
  map style; removed MODE/DEBUG from prod (production defaults); dev/test set
  MODE: dev + DEBUG: 1 per PART 26.
