# Testing Rules (PART 28, 29, 30)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## Testing (PART 28)
- Coverage threshold: ≥60% (CI gate), target ≥80%
- Coverage output: `/tmp/{project_org}/{internal_name}-XXXXXX/coverage.out` (NEVER in project tree)
- `make test` (local) / `go test -cover -coverprofile=...` (CI) — same gates
- Integration tests: `tests/run_tests.sh` — auto-detects incus/docker; NEVER run on host
- Run integration tests once implementation is fully verified

## Test File Naming
- Unit tests: `{feature}_test.go` alongside the source file
- Integration tests: `tests/` directory
- Table-driven tests preferred
- Test data: temp directories (`/tmp/...`), never committed

## ReadTheDocs Documentation (PART 29)
- `mkdocs.yml` at repo root; `docs/` directory
- `.readthedocs.yaml` at repo root
- Required docs sections: API, configuration, security, integrations, CLI
- Update docs when routes, config options, auth/integration behavior, or CLI changes
- MkDocs Material theme

## I18N & A11Y (PART 30)
- 7 required locales: `en`, `es`, `fr`, `de`, `zh`, `ar`, `ja`
- All locale files embedded in binary (`//go:embed`)
- Key parity: all 7 locale files MUST have identical keys
- Fallback: missing translation → fall back to `en.json`
- RTL support: `dir="rtl"` for Arabic (`ar`); detected via `i18n.Direction()`
- WCAG 2.1 AA minimum
- Touch targets: minimum 44×44px
- `lang="{{.Lang}}" dir="{{.Dir}}"` on ALL `<html>` elements
- `Accept-Language` header parsing for auto-detection
- `--lang` flag on client binary overrides locale

## NO_COLOR Support
- Respect `NO_COLOR` env var — disables ALL ANSI color output
- `--color {auto|always|never}` flag overrides `NO_COLOR`
- `auto`: color when TTY, no color when piped/redirected

---
For complete details, see AI.md PART 28, 29, 30
