# TODO.AI.md

## Remaining implementation gaps

### PART 28 — Browser E2E feature coverage and runner health

- Complete the IDEA-derived Tier 1, Tier 2, and Tier 3 matrix for every
  remaining feature, including all compat protocols and cross-tier coverage
  for form validation and owner-token failures.
- Verify every page's browser console and static asset status, including the
  themed 500 page. Current checks cover page metadata, internal links, mobile
  overflow, no-JavaScript paste flow, core paste lifecycle, stale-build cache
  refresh, CSRF, form validation, owner-token rejection, and themed 404 behavior.
- Provide local test doubles for outbound integrations while keeping the
  Compose network isolated.
