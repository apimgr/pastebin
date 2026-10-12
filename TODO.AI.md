# TODO.AI.md

## Remaining implementation gaps

### PART 28 — Browser E2E feature coverage

The current suite has three tiers, a fresh temporary Compose volume, an
internal-only test network, an internal-link crawl, a no-JavaScript paste
create/view flow, browser page and resource checks, and a mobile-width overflow
assertion. It still needs the complete IDEA-derived test matrix required by
AI.md PART 28:

- Add named Tier 1, Tier 2, and Tier 3 tests for every remaining project
  feature, including expiry, burn-after-read, visibility, syntax highlighting,
  download, deletion, compat protocols, CLI flows, theme changes, locale
  switching, form validation, and owner-token failures.
- Assert that every page has correct title, language, charset, and viewport
  metadata; exercise theme styles, CSRF enforcement, and static asset status.
- Use local test doubles for any outbound integration needed by these flows;
  keep the test network isolated from external services.
- Capture the required failure artifacts (screenshot, page HTML, and server
  log) under the mandated project temp directory.

### PART 32 — CLI configuration behavior

`cli.yml` currently persists settings that are not applied by command
execution. Trace each setting against AI.md PART 32 and implement its behavior
or remove it from generated defaults and documentation when the spec does not
require it. Known candidates include output pager/quiet/verbose, logging,
cache, TUI mouse/unicode, `update.auto`, and command defaults such as expiry,
syntax, visibility, output, and list limit.
