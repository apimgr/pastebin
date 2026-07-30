# Frontend Rules (PART 16)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- NEVER use client-side frameworks (React/Vue/Svelte/etc.) — server-side Go `html/template` only
- NEVER require JavaScript for core functionality — JS enhances, it does not enable
- NEVER write desktop-first CSS (base styles + `max-width` media queries) — mobile-first only
- NEVER let long strings (IPv6, Tor .onion, tokens, hashes, UUIDs) overflow their container or break mobile layout
- NEVER use inline CSS or inline `<script>`/`onclick=` handlers — external files only, bind via `data-action`
- NEVER use `alert()`/`confirm()`/`prompt()` — use custom modals/toasts
- NEVER hardcode colors — use `--color-*` CSS custom properties everywhere (dark/light/auto theme)
- NEVER parse HTML in test scripts — test frontend routes via `Accept: text/plain` (much simpler, more reliable)
- NEVER skip trailing-slash URL normalization (301 redirect to canonical, no trailing slash except root `/`)
- NEVER let a submit button be clickable twice — disable immediately on click, re-enable on success or error
- NEVER close a modal automatically on error — only auto-close on success; error keeps it open with the message shown
- NEVER omit the visible "Copied!" confirmation on a copy-to-clipboard button
- NEVER ship a form input without an associated `<label>`, or an image without descriptive `alt` text
- NEVER ignore `prefers-reduced-motion` — animations/transitions must reduce to ~0 when set
- NEVER route `/api/{api_version}/*` behind frontend catch-all routes — API routes are always highest priority

## CRITICAL - ALWAYS DO
- ALWAYS render all HTML via Go `html/template` (server-side); pure vanilla JS only, no frameworks
- ALWAYS make every feature work fully without JavaScript (forms submit via standard POST); JS only enhances (AJAX, validation feedback)
- ALWAYS write mobile-first CSS: unprefixed rule = mobile base, `@media (min-width: 768px)` = tablet+, `@media (min-width: 1024px)` = desktop+
- ALWAYS apply `word-break: break-all; overflow-wrap: break-word;` (or horizontal scroll) to any element that may contain long unbreakable strings
- ALWAYS detect client type (browser vs curl/CLI vs `Accept` header) on frontend routes and respond HTML vs text vs JSON accordingly
- ALWAYS support full CRUD through HTML forms (with `_method` override for PUT/PATCH/DELETE), the JSON API, and form-encoded/text for CLI
- ALWAYS keep footer at the bottom via flex column layout (`body{display:flex;flex-direction:column;min-height:100vh} main{flex:1}`)
- ALWAYS use semantic HTML elements for their intended purpose: `<code>` inline values, `<pre><code>` blocks, `<kbd>` keyboard input, `<samp>` output, `<time>` dates, `<dl>` key-value data
- ALWAYS provide a copy button (with visible "Copied!" feedback + `aria-live="polite"`) for long copy-worthy values: Tor addresses, API tokens, clone URLs
- ALWAYS wrap tables in `.table-wrapper { overflow-x: auto }` for mobile horizontal scroll
- ALWAYS meet WCAG 2.1 AA: keyboard navigation, visible focus indicators, ARIA labels, 4.5:1 text contrast (3:1 large text), skip-to-content link, proper heading hierarchy, `aria-live` error announcements
- ALWAYS support PWA basics: manifest, service worker, offline behavior, installable, maskable icons
- ALWAYS theme via `:root` CSS custom properties with a `html.theme-light` override block — dark default, light override, auto via `prefers-color-scheme`
- ALWAYS set `Access-Control-Allow-Origin: *` on API endpoints (CORS)
- ALWAYS run URL-normalize middleware first in the middleware chain (see PART 5 execution order)
- ALWAYS block reserved route names (`api`, `server`, `static`, `healthz`, `docs`, etc.) from being registered as project-specific slugs

## Key Rules Summary

### Route priority (highest → lowest)
1. `/api/{api_version}/*` — API routes
2. `/server/healthz` — health check
3. `/static/*` — static assets
4. `/server/*` — server pages (docs, about, status)
5. `/{reserved}` — reserved names
6. `/*` — project-specific routes (e.g. `/{slug}` paste lookup), registered last

### Client-type detection order
1. `Accept` header (`text/html` → html, `text/plain` → text, `application/json` → json)
2. `User-Agent` browser match (Mozilla/Chrome/Safari/Edge/Firefox/...) → html
3. `User-Agent` CLI tool match (curl/Wget/HTTPie/python-requests/...) → text
4. Empty `User-Agent` → text (programmatic default)
5. Fallback → html

### Mobile-first breakpoints
| Breakpoint | Target |
|---|---|
| (base, no query) | Phones <768px |
| `min-width: 768px` | Tablets+ |
| `min-width: 1024px` | Desktops+ |
| `min-width: 1280px` | Large desktops (optional) |

### Long-string handling
Apply to IPv6, .onion addresses, API tokens, SHA-256 hashes, UUIDs, base64 blobs:
```css
.long-string, .ip-address, .onion-address, .api-token, .hash, .uuid, .monospace-data {
  word-break: break-all;
  overflow-wrap: break-word;
  font-family: monospace;
}
.code-block { overflow-x: auto; white-space: nowrap; -webkit-overflow-scrolling: touch; }
```

### CSS variable set (dark default, `html.theme-light` overrides)
`--color-bg`, `--color-bg-secondary`, `--color-bg-card`, `--color-bg-hover`, `--color-bg-active`, `--color-code-bg`, `--color-text`, `--color-muted`, `--color-border`, `--color-border-hover`, `--color-success[/-bg]`, `--color-error[/-bg]`, `--color-warning[/-bg]`, `--color-primary[/-bg]`.

### Submit button behavior
Disable on click → show `"{Verb}ing..."` loading text (preserve button width) → re-enable on success or error response. Never allow double-submit.

### Modal behavior
| Event | Behavior |
|---|---|
| Success | Auto-close |
| Error | Stay open, show error |
| Cancel / Escape / backdrop click | Close immediately |
| Unsaved changes | Warn before closing |

### Accessibility checklist
Keyboard nav · visible focus ring · ARIA labels/`aria-describedby`/`role` · 4.5:1 contrast (3:1 large text) · `alt` on all images · `<label>` on all inputs · `aria-live` error announcements · skip-to-content link · h1→h2→h3 hierarchy · `prefers-reduced-motion` respected.

### Testing convention
Prefer `curl -H "Accept: text/plain" /path` (or rely on CLI auto-detect) over parsing HTML in test scripts — HTML parsing is fragile.

### Technology stack
Go `html/template` (all HTML) · pure vanilla JS, no frameworks · CSS-first, JS only where CSS can't do it · no inline CSS/JS · no `alert()`/`confirm()` (custom toasts/modals only).

For complete details, see AI.md PART 16.
