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

## [ ] Wire Makefile coverage gate to IDEA.md's coverage_minimum override
`Makefile`'s own comment (line 176-177) documents the mechanism: default
60%, "override upward in IDEA.md (coverage_minimum: 80) when appropriate."
`IDEA.md` sets `coverage_minimum: 80`, exercising that documented override
— this is not a spec conflict, AI.md is the how and this is the how it
already prescribes. The gap is that the `test:` recipe never reads the
override; it always hardcodes the `< 60` check. Make the recipe read
`coverage_minimum` from IDEA.md and enforce that value. Current measured
coverage is 75.6%, below 80 — raising the gate before adding tests will
fail `make test`; add tests to close the gap first, then wire the gate.
Read: Makefile:172-192, IDEA.md:20
