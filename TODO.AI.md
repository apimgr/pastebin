# TODO.AI.md

## [ ] Wire PGP keypair rotation into a caller
`src/server/pgp_keypair.go` `Server.RotateKeypair` (30-day grace window on
rotation) is fully implemented but wired to nothing — no scheduler task,
maintenance command, or handler calls it. Wire it (e.g. a `secret rotate`
maintenance path or scheduled task); do not delete, it is a spec-mandated
capability.
Read: AI.md PART 11

## [ ] Route API handlers through the canonical error envelope helper
`src/handler/paste.go` `sendAPIError`/`mapAPIErrorCodeToHTTPStatus`
(canonical `{ok:false,...}` error envelope) are implemented and unit-tested
but no handler calls them — the API handlers write the envelope inline
instead. Wire the handlers through `sendAPIError` for a single canonical
error path; do not delete the helper.
Read: AI.md PART 9

## [ ] Fix client exit codes to the sysexits range
`src/client/main.go`: `exitConnection`/`exitAuth`/`exitNotFound` use exit
codes 3/4/5, outside the valid range (0-2, 64-78, 128-143). Use e.g. 75
(EX_TEMPFAIL), 77 (EX_NOPERM), 66 (EX_NOINPUT).
Read: AI.md PART 32

## [ ] Remove hardcoded 127.0.0.1 fallback
`src/main.go` line 722: hardcoded `127.0.0.1` fallback — never hardcode a
static IP; use `localhost` for display or detect from request context.
Read: AI.md PART 11

## [ ] Raise Makefile coverage gate to match IDEA.md
`IDEA.md` declares `coverage_minimum: 80`, but `Makefile`'s `test` target
hardcodes a 60% enforcement threshold. Current coverage is 75.6%, which
passes the Makefile's 60% gate but not the declared 80% minimum. Either
raise the Makefile gate to 80% and add tests to close the gap, or update
IDEA.md if 80% was aspirational — do not silently pick one without
checking which the user intends.
Read: Makefile:172-192, IDEA.md:20
