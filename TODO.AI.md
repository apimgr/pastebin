# TODO.AI.md

## PART 16 audit follow-ups (AI.md WEB FRONTEND)

- Site Banner / Announcements (AI.md 22260-22347, 25648-25692) entirely
  unimplemented: no `web.announcements` config schema, no
  `site-banner`/`site-banner-*` classes/template/JS, no `/announcements/dismiss`
  route. Needs a new config schema in `src/config/**` before frontend work.
- Fixed: Form Validation markup gap per AI.md PART 16 (lines 22745-22864).
  Wired `aria-describedby`/`aria-invalid` plus a sibling `<span
  class="field-error" role="alert" hidden>` onto every form field in
  `contact.tmpl` (both the security-disclosure and regular contact forms),
  `create.tmpl`, `preferences.tmpl`, and `remove.tmpl`. Fixed
  `components.css.tmpl` so `.field-error` only becomes visible via the
  `:user-invalid ~ .field-error` CSS fallback rule (was previously
  unconditionally `display: block`). Added `initFormValidation()` to
  `app.js`, matching AI.md's own reference JS: validates on blur, clears the
  error once the field becomes valid again, mirrors `el.validationMessage`
  on the native `invalid` event, and focuses the first invalid field on
  submit. No new i18n keys needed — error text is the browser-native
  `validationMessage`, matching AI.md's own reference implementation, not a
  custom per-field string.
- Fixed: Toast system did not match AI.md PART 16's Toast Notifications spec
  (structure, JS reference implementation ~line 24687, and stacking CSS ~line
  22431). Renamed all `.toast--{type}`/`.toast--dismissing` BEM classes to
  `.toast-{type}`/`.toast-dismissing` in `components.css.tmpl` and their three
  usages in `app.js` (`showToast`, `showUpdateBanner`, offline indicator).
  Rebuilt `showToast()` to match the spec: `role="alert"`, `.toast-icon`/
  `.toast-message`/`.toast-close`/`.toast-progress` sub-elements, per-type
  auto-dismiss durations (success/info 3s, warning 5s, error never), click
  and close-button dismiss, Escape dismisses the topmost toast, max 5 visible
  with FIFO queueing for the rest, and pause-on-hover (CSS pauses the
  progress bar; JS pauses/resumes the matching removal timer). Container CSS
  switched from `column-reverse`/append to `column`/`prepend()` per the
  spec's stacking CSS, with `max-height`/`overflow: hidden`.
- Fixed: `src/config/footer.go` / `src/config/config.go` were missing AI.md's
  Footer Customization "Custom HTML Validation" and "Sanitization Preview
  (Startup Log)" requirements. Added `ValidateFooterHTML()` (rejects
  non-empty, non-sentinel `web.footer.custom_html` that sanitizes down to
  nothing, warns when partially stripped) and `LogFooterSanitizationPreview()`
  (logs raw input, sanitized output, and a modified-warning at startup),
  wired into `Validate()`/`Load()` following the existing warn-and-default
  pattern (`ValidateTracking`). Added `footer_test.go` covering pass-through,
  sentinel, safe HTML, fully-stripped-rejected, and
  partially-stripped-not-rejected cases.
- Consent-gated tracking path investigated and resolved by reasoning, no code
  change: `trackingScript` (`src/server/tracking.go`) renders analytics embeds
  into `<template id="pb-tracking-snippet">` in `footer.tmpl` — a `<template>`
  element never parses/executes its contained `<script>` tags. `app.js`'s
  `applyConsent()` only calls `activateTracking()` (clones the template into
  `<head>`, triggering execution) when `consent.analytics && cfg
  .analyticsConfigured`, i.e. gated on the visitor's actual stored consent
  choice, not just server config. This achieves the same guarantee as AI.md's
  `CheckTrackingAllowed(r)` reference (tracking never loads without consent)
  via this project's client-side-only consent model (`consent.go`: "server
  never records per-visitor consent") instead of a server-side per-request
  check — a legitimate architectural equivalent, not a gap.
- Fixed: `/server/about` was missing the AI.md PART 16 (lines 26904-26977)
  required GeoIP third-party attribution section (DB-IP link + NRO CC BY 4.0
  notice). Added a conditionally-rendered (`GeoIPEnabled`) attribution
  section to `about.tmpl`, wired `Server.GeoIP.Enabled` into
  `aboutPageData()` in `server.go`, and added `about.attribution`,
  `about.attribution_geoip`, `about.attribution_nro` i18n keys to all 7
  locales (commit 3c7f837fb16c). IDEA.md-sourced content (tagline,
  description, features, links) was already correctly wired via
  `EffectiveTagline()`/`EffectiveDescription()`/`EffectiveFeatures()`/
  `EffectiveLinks()` — no change needed there.
- Config-key existence verified, resolved by reasoning, no code change:
  `server.contact.general.email` exists (`ContactConfig.General ContactRole`,
  `src/config/config.go:145`); `server.pages.{about,privacy,help,terms}.content`
  and `server.pages.contact.{enabled,captcha,success_message}` all exist
  exactly as AI.md 27503-27547 specifies (`PagesConfig`/`PageContentConfig`/
  `ContactPageConfig`, `src/config/config.go:155-180`).
- Image Sources / Image Scaling / Remote URL Fetching (AI.md 25417-25634)
  entirely unimplemented: no SSRF-safe fetch util, no multi-size image
  generation/caching, no scheduler re-fetch task. `branding.favicon`/`.logo`/
  `.og_image` config fields exist but are never consumed —
  `/favicon.ico` unconditionally redirects to the embedded static default.
  Full new subsystem, needs a dedicated implementation task.
- `public.tmpl`'s default `<title>{{.SiteTitle}}</title>` fallback investigated
  and resolved by reasoning, no code change: it is only the `{{block "meta"
  .}}` fallback in the shared layout, and every page template in
  `src/server/template/page/*.tmpl` already defines its own `meta`
  block/`<title>` overriding it — `home.tmpl` uses `{{.SiteTitle}} - Paste
  Sharing Service` (the homepage/tagline-style form AI.md's SEO Meta Tags
  example shows), every subpage uses `{Page} - {{.SiteTitle}}` (e.g.
  `about.tmpl`, `create.tmpl`, `healthz.tmpl`), both matching AI.md's
  `{title} - {tagline}` two-part pattern structurally (specific-part before
  generic-part). The layout-level fallback is unreachable in normal operation
  and exists only as a safety default if a future page omits its own `meta`
  block; hardcoding a `- {tagline}` suffix there would be wrong when
  `Tagline` is empty (default config), so `{{.SiteTitle}}` alone is the
  correct fallback.
- Apple touch icon (`icon-180.png`) is served as SVG
  (`image/svg+xml`) via `handlePWAIcon192/512`; iOS Safari does not reliably
  render SVG for `apple-touch-icon`. Spec's PWA File Structure lists real PNG
  rasters at multiple sizes (72-512px + maskable). Needs an SVG→PNG rasterizer
  or precomputed PNG assets — no such dependency exists yet.

## Full-AI.md compliance pass follow-ups

- `src/tor/tor.go` `TorConfig` / `src/i2p/i2p.go` `I2PConfig` field-by-field
  diff against PART 31's config tables completed, resolved by reasoning, no
  code change: `TorConfig` has `Binary`, `UseNetwork`, `MaxCircuits`,
  `CircuitTimeout`, `BootstrapTimeout`, `SafeLogging`,
  `MaxStreamsPerCircuit`, `CloseCircuitOnStreamLimit`, `BandwidthRate`,
  `BandwidthBurst`, `MaxMonthlyBandwidth`, `NumIntroPoints`, `VirtualPort` —
  all of PART 31.1's default-config keys are present. `I2PConfig` has
  `Enabled`, `Binary`, `SAMAddress`, `VirtualPort`, `InboundLength`,
  `OutboundLength`, `InboundQuantity`, `OutboundQuantity`, `SignatureType`,
  `BootstrapTimeout` — all of PART 31.2's `server.i2p.*` keys are present.
  No missing fields.
- `src/server/tor_control.go` diffed against PART 31.1, resolved by
  reasoning, no code change: `torControlLoopbackMiddleware` restricts the
  `/server/tor/*` channel to loopback peers via `peerAddr()` (the preserved
  original TCP peer, not a proxy-rewritten value) and returns a bare 404 for
  any other caller, matching the "not discoverable" requirement; handlers
  for restart/regenerate/vanity start/stop/apply/import-keys implement
  `RegenerateAddress()`/`ApplyKeys()` semantics via `s.TorRegenerateAddress`/
  `s.TorApplyKeys`/`s.TorImportKeyPath`. No gap found.
- CLI-side i18n is entirely absent: `src/client/**` never calls
  `i18n.GetLanguage`/`i18n.Translate`, so `--lang`/`cli.yml lang:` has no
  effect on CLI output and every CLI string is hardcoded English. PART 30
  requires the locale files be embedded in ALL binaries. Large, needs a
  dedicated task (extract every `fmt.Printf` string in `src/client/` to keys
  across all 7 locales).
- `src/client/` is a flat package; PART 32's illustrative tree splits it into
  subpackages. Cosmetic/structural — needs a decision before churn.
- `src/path/path.go`'s `SafePath()`/`validatePath()` (PART 5) investigated and
  resolved by reasoning, no code change: (1) `pathSecurityMiddleware` in
  `src/server/security_middleware.go` already implements PART 5's HTTP
  traversal-blocking requirement independently (stdlib `path.Clean` +
  `..`/`%2e` rejection), correctly wired at middleware position #3 — `SafePath`
  itself is scoped by its own doc comment to config/CLI-flag/API-parameter
  paths, not general URL paths, so it is not the right fit there. (2) AI.md's
  own PART 5 example (line 7036) applies `SafePath()` to CLI directory flags
  like `--data`, but `validatePathSegment`'s regex (`^[a-z0-9_-]+$`, no
  uppercase, max 64 chars) would reject mandatory real-world OS paths from
  PART 4's own directory table — macOS `/Library/Application Support/...` and
  Windows `%ProgramData%\...` both contain uppercase/spaces. Wiring `SafePath`
  into `--config`/`--data`/`--log`/`--cache`/`--backup`/`--pid` in
  `src/main.go` as literally shown would break mandatory Windows/macOS
  support. `SafePath` remains correctly-scoped defensive utility code for a
  future resource-identifier-shaped path (API file params, slug-like config
  keys) with no current caller — not a bug to fix by removal or forced wiring.
- AI.md self-contradiction (recorded, no code change): PART 5's "Six
  Operational States" table and its "Mode Shortcuts" table disagree on whether
  `MODE=debug` implies development. Implementation follows the Mode Shortcuts
  table plus the explicit-`DEBUG`-env-wins rule.
- PART 23 lists s6 among supported Linux init systems but PART 24 supplies no
  s6 service template — nothing concrete to implement against.
