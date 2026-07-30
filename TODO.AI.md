# TODO.AI.md

- Hand-rolled CLI argument parsing in `src/main.go` lines 64-188 should use
  stdlib `flag` per `binary-rules.md` (server binary is single-command,
  never manual `switch`/`os.Args` loops).
- ~9 dead exported symbols flagged in a prior audit still need to be
  identified and removed (see prior `AUDIT.AI.md` findings).
- `src/client/main.go` lines 60-62: `exitConnection`/`exitAuth`/
  `exitNotFound` use exit codes 3/4/5, outside the valid sysexits range
  (0-2, 64-78, 128-143) — should use e.g. 75 (EX_TEMPFAIL), 77
  (EX_NOPERM), 66 (EX_NOINPUT).
- `src/main.go` line 746: hardcoded `127.0.0.1` fallback — never hardcode
  a static IP; use `localhost` for display or detect from request context.
