# Project Audit

Started: 2026-07-27

All auto-fixable findings are resolved (see Completed). Only the items in
Flagged remain — they are design decisions that require operator input and
were intentionally NOT auto-changed.

## Flagged (design decisions — NOT auto-fixed)
- task/task.go: the `update_available` scheduler email is dead — PART 17
  defines exactly 8 email templates and does not include `update_available`,
  and the call passes CamelCase keys (`CurrentVersion`, `NewVersion`,
  `Branch`) that never substitute against the snake_case renderer. Either
  drop the notification or add a spec'd template + snake_case keys. Needs a
  spec decision (adding a 9th template deviates from PART 17).
- cache Get/Set/Delete appear unused (PART 9 design question — keep as public
  API surface or wire in / remove).
- security.yml vs ci.yml consolidation (risky CI restructure, needs `act`
  verification before touching).
- ~9 dead exported symbols (low-value churn; leave unless a cleanup pass is
  explicitly requested).

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
