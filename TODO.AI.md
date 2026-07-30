# TODO

- go-lint flagged two directory naming violations (found incidentally while
  running the lint gate for an unrelated AUDIT.AI.md commit, 2026-07-30):
  `src/metrics` should be singular `src/metric`, and `src/paths` should be
  singular `src/path`, per the Go package-naming convention (singular source
  packages: handler, model, config). Not fixed yet — renaming a package
  touches every import site across the codebase, so it needs a deliberate
  pass rather than a bundled fix.
