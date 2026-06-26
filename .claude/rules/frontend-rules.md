# Frontend Rules (PART 16)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Client-side rendering (React, Vue, Angular, etc.)
- ❌ Require JavaScript for core functionality
- ❌ Client-side routing (SPA)
- ❌ Business logic in JavaScript
- ❌ Let long strings break mobile layout
- ❌ Desktop-first CSS (use mobile-first)
- ❌ Inline CSS or JavaScript
- ❌ JavaScript alerts (use toast notifications)
- ❌ Generic placeholder content in /server/about or /server/help pages
- ❌ Stub templates or "coming soon" pages

## CRITICAL - ALWAYS DO
- ✅ Server-side rendering (Go `html/template`)
- ✅ Progressive enhancement (works without JS)
- ✅ Mobile-first responsive CSS
- ✅ CSS `word-break: break-all` for long strings (IPv6, .onion, tokens)
- ✅ All settings configurable via API and config file
- ✅ WCAG 2.1 AA accessibility
- ✅ Touch targets minimum 44×44px
- ✅ /server/about content from IDEA.md (name, tagline, description, features)
- ✅ /server/help content from IDEA.md (real endpoints, real examples)
- ✅ `lang="{{.Lang}}" dir="{{.Dir}}"` on all `<html>` elements (RTL support)
- ✅ Dark mode default; support dark/light/auto via CSS custom properties
- ✅ Server-side Chroma syntax highlighting

## LONG STRINGS (REQUIRED CSS)
```css
.long-string, .ip-address, .onion-address, .api-token, .hash {
  word-break: break-all;
  overflow-wrap: break-word;
  font-family: monospace;
}
```
Apply to: IPv6, Tor .onion, API tokens, hashes, UUIDs, Base64

## BREAKPOINTS (mobile-first)
| Target | CSS |
|--------|-----|
| Mobile (base) | No media query |
| Tablet+ | `@media (min-width: 768px)` |
| Desktop+ | `@media (min-width: 1024px)` |

## SERVER VS CLIENT
| Task | Where | Why |
|------|-------|-----|
| Data validation | SERVER | Server is authoritative |
| HTML rendering | SERVER | Works without JS |
| Business logic | SERVER | Security, consistency |
| Formatting | SERVER | Consistent output |
| Theme toggle | Client JS | Instant UX feedback |
| Copy to clipboard | Client JS | Browser API required |
| Form feedback | Client JS | UX enhancement only |

## PAGE CONTENT SOURCING
| Page | Content Source |
|------|----------------|
| /server/about | IDEA.md → name, tagline, description, features, links |
| /server/help | IDEA.md → real endpoints, real curl examples, real FAQ |
| /server/privacy | Config → `server.privacy.*` settings |
| /server/terms | Config → customizable, default template |
| /server/contact | Config → `server.contact.general.*` settings |

## THEME SYSTEM
- CSS custom properties only — NEVER hardcode colors
- Dark mode default; user preference via `prefers-color-scheme` + JS toggle
- Dracula-inspired palette: `--bg: #1e1e2e`, `--fg: #cdd6f4`, `--accent: #89b4fa`
- `data-theme="dark|light"` on `<html>`; persisted in `localStorage`

---
For complete details, see AI.md PART 16
