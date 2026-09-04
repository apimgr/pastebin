# TODO.AI.md

## PART 16 audit follow-ups (AI.md WEB FRONTEND)

- Multi-line code block naming mismatch: `src/server/static/css/public.css.tmpl`
  (~675-728), `src/server/template/page/{home,healthz,help}.tmpl`, and
  `copyCode()` in `src/server/static/js/app.js` use `.code-block`/`.code-copy`/
  `data-copy-code`. AI.md 21422-21484 specifies `.code-block-multi` wrapper and
  reuse of `.copy-btn`/`data-copy-target="<id>"` (same pattern as single-line
  copy). Needs a coordinated rename across 1 CSS file + 3 templates + JS.
- `src/server/template/partial/public/nav.tmpl` theme-toggle form posts to
  `{{.AssetPrefix}}/theme` (working `POST /theme` route exists), but a spec
  code sample elsewhere shows `/server/preferences`. Confirm canonical route
  against AI.md's Theme Toggle section (~line 22349) before changing.
- Site Banner / Announcements (AI.md 22260-22347, 25648-25692) entirely
  unimplemented: no `web.announcements` config schema, no
  `site-banner`/`site-banner-*` classes/template/JS, no `/announcements/dismiss`
  route. Needs a new config schema in `src/config/**` before frontend work.
- Form Validation markup gap: CSS for `.field-error`/`aria-invalid` states was
  added, but no template (`contact.tmpl`, `create.tmpl`, `preferences.tmpl`,
  `remove.tmpl`) wires `aria-describedby`/`aria-invalid`/`<span
  class="field-error">` per field, and `app.js` has no blur-validation JS. Needs
  new i18n error-message keys per field across all 7 locales.
- Toast CSS class naming: `components.css.tmpl` uses BEM-style
  `.toast--info`/`.toast--success`/etc.; AI.md's Toast Structure example uses
  two space-separated classes (`class="toast toast-success"`). Not reconciled
  against the full `showToast`/`dismissToast` JS API — needs a dedicated pass.
- Buttons loading-state pattern: spec uses `data-action="submit-loading"` +
  `data-loading-text` attributes; `app.js` instead binds a generic handler to
  every `<form>` with a hardcoded English-word→i18n-key map. Functionally
  broader than spec but structurally different — needs a human call on whether
  this is an acceptable superset.
- `src/config/footer.go` / `src/config/config.go`: AI.md's Footer Customization
  "Custom HTML Validation" (reject fully-stripped content) and "Sanitization
  Preview (Startup Log)" requirements are unimplemented.
- Cookie Consent Banner exact CSS values (`#7c5295` background, `z-index:
  9999`, `@media (min-width: 601px)` breakpoint) not verified against
  `components.css.tmpl`/`public.css.tmpl`.
- `app.js` JS-rendered consent banner path (`#pb-consent-data`,
  `consentInfo`/`consentConfig` template funcs) not verified against spec's
  `CheckTrackingAllowed` behavior table; `src/server/consent.go`/`tracking.go`/
  `preferences.go` not diffed against the consent/CCPA/tracking behavior
  tables.
- `/server/about` GeoIP-attribution and IDEA.md-sourced-content requirements
  not conclusively verified against `about.tmpl` — needs a follow-up pass
  reading that spec slice directly.
- Config-key existence not verified (would touch `src/config/**`, out of PART
  16 audit scope): `server.contact.general.email`, `pages.{about,privacy,
  contact,help,terms}.*`.
- Image Sources / Image Scaling / Remote URL Fetching (AI.md 25417-25634)
  entirely unimplemented: no SSRF-safe fetch util, no multi-size image
  generation/caching, no scheduler re-fetch task. `branding.favicon`/`.logo`/
  `.og_image` config fields exist but are never consumed —
  `/favicon.ico` unconditionally redirects to the embedded static default.
  Full new subsystem, needs a dedicated implementation task.
- `public.tmpl`'s default `<title>{{.SiteTitle}}</title>` fallback has no
  tagline suffix (`{title} - {tagline}`) — ambiguous whether the spec intends
  this for the no-override default page title; needs a decision.
- Apple touch icon (`icon-180.png`) is served as SVG
  (`image/svg+xml`) via `handlePWAIcon192/512`; iOS Safari does not reliably
  render SVG for `apple-touch-icon`. Spec's PWA File Structure lists real PNG
  rasters at multiple sizes (72-512px + maskable). Needs an SVG→PNG rasterizer
  or precomputed PNG assets — no such dependency exists yet.

## Full-AI.md compliance pass follow-ups

- `.dockerignore` line for git excludes `.git` rather than `.git/`; the
  `no-forbidden-files.sh` hook rejects any Edit/Write to that path (false
  positive — it classifies the file as a Dockerfile that must live under
  `docker/`). Needs a human to make the one-character change or adjust the
  hook. Never auto-bypassed.
- `src/tor/tor.go` `TorConfig` and `src/i2p/i2p.go` `I2PConfig` struct fields
  were not exhaustively diffed against PART 31's config tables
  (`max_circuits`, `circuit_timeout`, `num_intro_points`, `bandwidth_*`,
  `inbound_length`/`quantity`, `signature_type`, etc.). Needs a field-by-field
  pass.
- `src/server/tor_control.go` was not exhaustively diffed against PART 31.1.
- CLI-side i18n is entirely absent: `src/client/**` never calls
  `i18n.GetLanguage`/`i18n.Translate`, so `--lang`/`cli.yml lang:` has no
  effect on CLI output and every CLI string is hardcoded English. PART 30
  requires the locale files be embedded in ALL binaries. Large, needs a
  dedicated task (extract every `fmt.Printf` string in `src/client/` to keys
  across all 7 locales).
- `src/client/` is a flat package; PART 32's illustrative tree splits it into
  subpackages. Cosmetic/structural — needs a decision before churn.
- PART 5's escalation matrix requires privilege escalation for
  `--service start/stop/restart/reload/disable`; `src/service/service.go`
  currently gates only `Install()`/`Uninstall()` on `isPrivileged()`.
- `writeJSON()` is duplicated verbatim in `src/server/server.go` and
  `src/handler/paste.go`. Both are individually spec-compliant; deduplicating
  into a shared helper package is an architectural change.
- AI.md self-contradiction (recorded, no code change): PART 5's "Six
  Operational States" table and its "Mode Shortcuts" table disagree on whether
  `MODE=debug` implies development. Implementation follows the Mode Shortcuts
  table plus the explicit-`DEBUG`-env-wins rule.
- PART 23 lists s6 among supported Linux init systems but PART 24 supplies no
  s6 service template — nothing concrete to implement against.
