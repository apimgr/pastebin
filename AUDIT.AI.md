# Project Audit

Started: 2026-07-24
Scope: AI.md PART 7–33 line-by-line compliance. AI.md is authoritative; code is fixed to match.

## Pass 5: Spec Compliance

### Build / Docker / Makefile (bounded — fix now)
- [ ] docker/Dockerfile:73 `ENV MODE=development` in production runtime stage → remove; keep TZ + DATABASE_DIR (AI.md PART 26 §32031-32033: "MODE is never baked into the image")
- [ ] Makefile:8 `VERSION ?=` inverts release.txt precedence → `VERSION := $(shell cat release.txt 2>/dev/null || echo "$${VERSION:-devel}")` (AI.md PART 25 §31150)
- [ ] Makefile:12 `BUILD_DATE` human date → ISO 8601 UTC `date -u +"%Y-%m-%dT%H:%M:%SZ"` (AI.md PART 25 §31153-31154)

### Frontend PART 16 (bounded — fix now)
- [ ] JS: 4 files (main.js, create.js, consent.js, remove.js) → single static/js/app.js; update all `<script src>`; delete extras (AI.md §23606-23607, §23658)
- [ ] templates/offline.html:41 inline `onclick="window.location.reload()"` → bind via addEventListener in app.js (AI.md §23622 "NO inline styles/handlers", PART 16 progressive-enhancement)
- [ ] static/css/main.css.tmpl non-print `!important` uses (lines ~1279, ~2035, ~2603-2606) → remove or confine to `@media print` (AI.md §23623)

### Frontend PART 16 (LARGE — full rewrite of rendering layer; browser verification required — deferred, flagged to user)
- [ ] CSS: single main.css.tmpl → split into common.css / components.css / public.css, load order common→components→public (AI.md §23602-23605, §23619-23620)
- [ ] templates: flat `src/server/templates/*.html` → `src/server/template/` with `.tmpl` + layout/partial/page/component subdirs; update //go:embed (server.go:57) + ParseFS (server.go:647) + all {{template}} includes (AI.md §9286, §23588-23596)

## Completed
(none yet)
