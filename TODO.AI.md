# TODO.AI.md

- CI `vuln-scan` job (govulncheck) failing on `main` as of commit `f197de572a72` — 7 new Go
  stdlib CVEs (GO-2026-6218, 6091, 6090, 6089, 6088, 5972, 5026), all fixed in go1.26.6;
  `casjaysdev/go:latest` currently ships go1.26.5. Not caused by any code in this repo —
  previous push (`fb9de177`) passed CI cleanly on the same image. Re-run CI once
  `casjaysdev/go:latest` picks up go1.26.6, or track upstream image update; remove this
  item once `vuln-scan` is green again.

- General `?lang=` navigation does not persist a `lang` cookie. `i18n.LangFromRequest()`
  (`src/common/i18n/i18n.go:197`) reads `?lang=` for that single request only — it never
  calls `http.SetCookie`. AI.md's Client-Side Preferences table documents `?lang=` as
  setting a persistent cookie on ordinary page navigation, but today only the new
  `/server/preferences/import` endpoint (`src/server/preferences.go`) actually persists
  `lang`/`theme` cookies. Needs a small middleware or handler-level change so any request
  carrying a valid `?lang=` sets the `lang` cookie (mirroring `setPreferenceCookie`),
  not just the preferences-import flow.
