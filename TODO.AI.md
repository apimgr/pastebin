# TODO.AI.md

## Remaining implementation gaps

### PART 28 — Browser E2E feature coverage and runner health

- Complete the IDEA-derived named Tier 1, Tier 2, and Tier 3 matrix for every
  user-facing feature. Current coverage includes the native paste lifecycle,
  browser and no-JavaScript form creation, raw/download/embed/QR views, expiry,
  burn-after-read, owner-token API rejection and management form rendering,
  all listed compat create/read protocols, theme and locale changes, responsive
  layout, CSRF, docs pages, and themed 404/500 responses. Compatibility wire
  protocols and several other IDEA features still lack scenarios in all tiers.
- Crawl every navigable page in all applicable tiers and check every referenced
  asset and browser console result. Tier 1 smoke-requests every registered GET
  route, crawls internal links, and checks referenced assets. Tier 3 now follows
  same-origin links recursively and records console and resource failures;
  routes that are not linked from navigable pages still need browser scenarios.
- Add local test doubles for outbound integrations (including GeoIP, Tor,
  notification delivery, and update/blocklist sources) and exercise them. The
  temporary E2E Compose network is internal-only, with GeoIP and Tor disabled;
  this prevents external access but does not replace local doubles.
