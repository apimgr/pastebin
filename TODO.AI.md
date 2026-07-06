# TODO.AI.md — Pastebin
# Outstanding implementation tasks not completed during bootstrap.
# Items are removed when fully done. Never cleared in progress.

## [ ] Raise test coverage to ≥80% target
The CI gate enforces ≥60% (`THRESHOLD=60` in ci.yml). The spec target is ≥80%.
Measure current coverage (`make test`), identify under-tested packages, add
table-driven unit tests until overall coverage meets the target. Priority packages:
`src/maintenance`, `src/ssl`, `src/tor`, `src/updater`, `src/service`.
Read: AI.md PART 28

## [ ] Run integration test suite and confirm it passes
`tests/run_tests.sh` exists but has not been executed to verify end-to-end
correctness. Run inside Docker or Incus (auto-detected by the script) once
full binary builds succeed. Fix any failures before marking done.
Read: AI.md PART 28

## [ ] WCAG 2.1 AA accessibility audit
The HTML templates implement mobile-first CSS and minimum 44×44px touch targets,
but conformance against WCAG 2.1 AA has not been verified. Audit all templates
in `src/server/templates/` with an automated tool (axe-core, pa11y, or
Lighthouse). Fix any violations: colour contrast, ARIA roles, skip-nav link,
form label associations, focus indicators.
Read: AI.md PART 30

## [ ] Verify all 8 release platforms build and produce correctly named binaries
`make build` should produce 8 binaries: linux/darwin/windows/freebsd × amd64/arm64.
Run inside `casjaysdev/go:latest` and confirm each output is named
`pastebin-{os}-{arch}` (windows adds `.exe`) with no `-musl` suffix.
Confirm `pastebin-cli-{os}-{arch}` client binaries are built alongside the server.
Read: AI.md PART 7

## [ ] Verify release.txt version matches CI release workflow version source
`release.txt` contains `0.0.9`. The release workflow must read from `release.txt`
first (per spec VERSION precedence). Verify `.github/workflows/release.yml` uses
`release.txt` as the authoritative version source and that the version embedded via
LDFLAGS at build time matches.
Read: AI.md PART 6
