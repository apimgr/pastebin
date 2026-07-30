# TODO.AI.md

- Hand-rolled CLI argument parsing in `src/main.go` lines 64-188 should use
  stdlib `flag` per `binary-rules.md` (server binary is single-command,
  never manual `switch`/`os.Args` loops).
- `src/server/pgp_keypair.go` `Server.RotateKeypair` (AI.md 14182: PGP
  keypair rotation with 30-day grace window) is fully implemented but wired
  to nothing — no scheduler task, maintenance command, or handler calls it.
  Wire it (e.g. a `secret rotate` maintenance path or scheduled task), do not
  delete: it is a spec-mandated capability.
- `src/handler/paste.go` `sendAPIError`/`mapAPIErrorCodeToHTTPStatus` (PART 9
  canonical error envelope) are implemented and unit-tested but no handler
  uses them — the API handlers write the `{ok:false,...}` envelope inline
  instead (e.g. paste.go:847). Wire the handlers through `sendAPIError` for a
  single canonical error path, do not delete the helper.
- `src/client/main.go` lines 60-62: `exitConnection`/`exitAuth`/
  `exitNotFound` use exit codes 3/4/5, outside the valid sysexits range
  (0-2, 64-78, 128-143) — should use e.g. 75 (EX_TEMPFAIL), 77
  (EX_NOPERM), 66 (EX_NOINPUT).
- `src/main.go` line 746: hardcoded `127.0.0.1` fallback — never hardcode
  a static IP; use `localhost` for display or detect from request context.
