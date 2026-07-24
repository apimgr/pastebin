# Project Audit

Started: 2026-07-24
Scope: AI.md PART 7–33 line-by-line compliance. AI.md is authoritative; code is fixed to match.

## Pass 5: Spec Compliance

### Build / Docker / Makefile
- [x] docker/Dockerfile:73 `ENV MODE=development` in production stage → removed (AI.md §32031-32033) — commit 78fa9683
- [x] Makefile VERSION `?=` precedence → `:=` release.txt wins (AI.md §31150) — commit dfdcd39a
- [x] Makefile BUILD_DATE → ISO 8601 UTC (AI.md §31153-31154) — commit dfdcd39a

### Frontend PART 16
- [x] JS: 4 files → single static/js/app.js; refs updated; extras deleted (AI.md §23606-23607, §23658) — commit fbd97b18
- [x] offline.html inline `onclick` → data-action hook bound in app.js (AI.md §23622) — commit fbd97b18
- [x] main.css.tmpl `.hidden` non-print `!important` removed; reduced-motion reset kept as WCAG-canonical exception per PART 30 (AI.md §23623) — commit fbd97b18

### Frontend PART 16 — DEFERRED (full rewrite of rendering layer; browser verification required, not safely auto-fixable in an audit pass)
- [x] CSS: single main.css.tmpl → common.css / components.css / public.css, load order common→components→public (AI.md §23602-23605, §23619-23620). 3 CSS-serving routes added (`handleCSS`), all 19 page templates (excl. `_footer.html` partial) updated with 3 `<link>` tags in load order; verified via `docker-compose.test.yml` — `/`, `/server/about`, `/server/help` all 200 with non-empty bodies, and `/static/css/{common,components,public}.css` all 200 `text/css`.
- [ ] templates: flat `src/server/templates/*.html` → `src/server/template/` with `.tmpl` + layout/partial/page/component subdirs and {{block}}/{{define}} composition; update //go:embed (server.go:57) + ParseFS (server.go:647) + all includes (AI.md §9286, §23588-23596). Full rearchitecture of every page; a passing build does not prove pages render. Recommend a dedicated designer-agent pass with a live server + browser verification.

## Completed
- docker/Dockerfile MODE removal (78fa9683)
- Makefile VERSION + BUILD_DATE (dfdcd39a)
- JS consolidation + offline handler + .hidden !important (fbd97b18)
