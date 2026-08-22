# TODO.AI.md

- General `?lang=` navigation does not persist a `lang` cookie. `i18n.LangFromRequest()`
  (`src/common/i18n/i18n.go:197`) reads `?lang=` for that single request only — it never
  calls `http.SetCookie`. AI.md's Client-Side Preferences table documents `?lang=` as
  setting a persistent cookie on ordinary page navigation, but today only the new
  `/server/preferences/import` endpoint (`src/server/preferences.go`) actually persists
  `lang`/`theme` cookies. Needs a small middleware or handler-level change so any request
  carrying a valid `?lang=` sets the `lang` cookie (mirroring `setPreferenceCookie`),
  not just the preferences-import flow.
