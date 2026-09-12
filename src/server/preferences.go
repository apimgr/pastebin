package server

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/apimgr/pastebin/src/common/httputil"
	"github.com/apimgr/pastebin/src/common/i18n"
)

// prefCookiePrefix names every app-specific guest preference cookie
// (AI.md 23480-23496: "{project_name}_pref_{key}"). Pastebin has no concrete
// app-specific preference today, but the export/import mechanism below is
// generic so any future `pastebin_pref_*` cookie round-trips automatically
// without a second storage mechanism ever being invented (AI.md 23496).
const prefCookiePrefix = "pastebin_pref_"

// prefKeyPattern constrains the `{key}` portion of a `pastebin_pref_{key}`
// cookie/param name — lowercase alnum and underscores only, matching the
// project's own naming conventions for config/cookie keys.
var prefKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// prefValuePattern is a conservative allowlist for app-specific preference
// values in the absence of any concrete preference (and therefore no
// per-key enum) to validate against yet — printable ASCII, no control
// characters, no cookie/URL metacharacters, bounded length.
var prefValuePattern = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,64}$`)

// appPreferencesFromRequest returns every `pastebin_pref_*` cookie present
// on the request, keyed by the bare `{key}` suffix (AI.md 23496: "cookie-only,
// read per request, never persisted server-side").
func appPreferencesFromRequest(r *http.Request) map[string]string {
	prefs := make(map[string]string)
	for _, c := range r.Cookies() {
		if key, value, ok := parseAppPref(c.Name, c.Value); ok {
			prefs[key] = value
		}
	}
	return prefs
}

// extractAppPrefs scans a set of query values (either the live request query
// or a decoded import `code`) for `pastebin_pref_*` params, applying the same
// key/value allowlist as appPreferencesFromRequest — an imported value is
// still untrusted input (AI.md 22908), so it is revalidated the same way.
func extractAppPrefs(values url.Values) map[string]string {
	prefs := make(map[string]string)
	for name, vals := range values {
		if len(vals) == 0 {
			continue
		}
		if key, value, ok := parseAppPref(name, strings.TrimSpace(vals[0])); ok {
			prefs[key] = value
		}
	}
	return prefs
}

// parseAppPref validates a single `pastebin_pref_{key}` name/value pair
// against prefKeyPattern/prefValuePattern, returning the bare key on success.
func parseAppPref(name, value string) (key string, val string, ok bool) {
	if !strings.HasPrefix(name, prefCookiePrefix) {
		return "", "", false
	}
	key = strings.TrimPrefix(name, prefCookiePrefix)
	if !prefKeyPattern.MatchString(key) || !prefValuePattern.MatchString(value) {
		return "", "", false
	}
	return key, value, true
}

// setPreferenceCookie writes a client-side preference cookie using the same
// Secure/SameSite/MaxAge shape as handleThemeSet and handleConsentSet
// (AI.md 22886-22890: "the server sets the same cookies on its POST/GET
// preference endpoints").
func (s *Server) setPreferenceCookie(w http.ResponseWriter, r *http.Request, name, value string) {
	secure := r.TLS != nil
	if s.liveCfg().Web.CSRF.Secure == "true" {
		secure = true
	} else if s.liveCfg().Web.CSRF.Secure == "false" {
		secure = false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// handlePreferences serves the guest preferences hub — GET /server/preferences,
// API-mirrored at GET /api/{api_version}/server/preferences (AI.md 22904). It
// reports the two exportable preferences (theme, lang) resolved from the
// request's cookies; nothing is read from or written to the database — there
// is no preferences table (AI.md 22909).
func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	theme := s.themeFromRequest(r)
	lang := i18n.LangFromRequest(r)

	switch detectClientType(r) {
	case "json":
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"theme": theme,
				"lang":  lang,
			},
		})
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "preferences.html", s.preferencesPageData(r, theme, lang))
		if err != nil {
			fmt.Fprintf(w, "Preferences — theme=%s lang=%s\n", theme, lang)
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
	default:
		s.renderTemplate(w, r, "preferences.html", s.preferencesPageData(r, theme, lang))
	}
}

// preferencesPageData assembles the template data shared by the preferences
// hub, export, and import-error views.
func (s *Server) preferencesPageData(r *http.Request, theme, lang string) map[string]interface{} {
	data := s.pageData()
	data["PrefTheme"] = theme
	data["PrefLang"] = lang
	// Seed the cookie-consent toggles from the visitor's existing choice (if
	// any), falling back to the configured defaults — the preferences page is
	// the one place a visitor can revisit and change consent after the
	// first-visit banner is gone (no-JS parity: a working control must remain
	// reachable without JavaScript, AI.md PART 16 "Frontend Consumes Backend").
	cfg := s.liveCfg()
	def := buildConsentClientConfig(cfg)
	consentPreferences := def.DefaultPreferences
	consentAnalytics := def.DefaultAnalytics
	if prior, hadPrior := priorConsentState(r); hadPrior {
		consentPreferences = prior.Preferences
		consentAnalytics = prior.Analytics
	}
	data["ConsentPreferences"] = consentPreferences
	data["ConsentAnalytics"] = consentAnalytics
	// CCPA opt-out toggle (AI.md 23480: every cookie in the table needs a
	// control here, including CCPA) — same Privacy/CCPAOptedOut shape
	// privacyPageData exposes to privacy.tmpl, so the same template partial
	// pattern works unchanged on this page.
	privacy := cfg.Server.Privacy
	data["Privacy"] = &privacy
	ccpaOptedOut := false
	if cookie, err := r.Cookie("ccpa_opt_out"); err == nil && cookie.Value == "true" {
		ccpaOptedOut = true
	}
	data["CCPAOptedOut"] = ccpaOptedOut
	return data
}

// preferencesExportQuery builds the canonical `theme=...&lang=...&pastebin_pref_{key}=...`
// query string for the current preferences — the query string IS the portable
// preference state (AI.md 22901: "the code/URL is the preference values, not
// a lookup key"). theme, lang, and every app-specific `pastebin_pref_*`
// cookie round-trip (AI.md 23502); `cookie_consent`/`ccpa_opt_out`/
// `pastebin_build` are never included since they aren't in extra (they don't
// carry the `pastebin_pref_` prefix appPreferencesFromRequest filters on).
func preferencesExportQuery(theme, lang string, extra map[string]string) string {
	q := fmt.Sprintf("theme=%s&lang=%s", url.QueryEscape(theme), url.QueryEscape(lang))
	if len(extra) == 0 {
		return q
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(q)
	for _, k := range keys {
		b.WriteString("&")
		b.WriteString(url.QueryEscape(prefCookiePrefix + k))
		b.WriteString("=")
		b.WriteString(url.QueryEscape(extra[k]))
	}
	return b.String()
}

// handlePreferencesExport serves GET /server/preferences/export, API-mirrored
// at GET /api/{api_version}/server/preferences/export (AI.md 22905). It
// returns the current theme/lang preferences as a full importable URL and as
// a base64url short code for manual retyping on a device without copy/paste.
func (s *Server) handlePreferencesExport(w http.ResponseWriter, r *http.Request) {
	theme := s.themeFromRequest(r)
	lang := i18n.LangFromRequest(r)
	appPrefs := appPreferencesFromRequest(r)
	query := preferencesExportQuery(theme, lang, appPrefs)
	exportURL := s.baseURL(r) + "/server/preferences/import?" + query
	code := base64.RawURLEncoding.EncodeToString([]byte(query))

	switch detectClientType(r) {
	case "json":
		data := map[string]interface{}{
			"theme": theme,
			"lang":  lang,
			"url":   exportURL,
			"code":  code,
		}
		if len(appPrefs) > 0 {
			data["prefs"] = appPrefs
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":   true,
			"data": data,
		})
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Preferences export\nURL:  %s\nCode: %s\n", exportURL, code)
		if len(appPrefs) > 0 {
			keys := make([]string, 0, len(appPrefs))
			for k := range appPrefs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(w, "%s%s: %s\n", prefCookiePrefix, k, appPrefs[k])
			}
		}
	default:
		data := s.preferencesPageData(r, theme, lang)
		data["ExportURL"] = exportURL
		data["ExportCode"] = code
		s.renderTemplate(w, r, "preferences.html", data)
	}
}

// handlePreferencesImport serves GET /server/preferences/import, API-mirrored
// at GET /api/{api_version}/server/preferences/import (AI.md 22908). It
// accepts either explicit `theme`/`lang` query params (from a shared full
// URL) or a `code` param (a pasted base64url short code, with an optional
// leading full-URL prefix already stripped client-side per AI.md 22907;
// stripped again here defensively for the no-JS path). Every value is
// revalidated against its normal allowlist — an imported value is still
// untrusted input (AI.md 22908) — anything unknown or malformed is silently
// dropped rather than applied. Nothing is persisted server-side: decode →
// validate → set cookie → redirect happens in this one request (AI.md 22909).
func (s *Server) handlePreferencesImport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	theme := strings.TrimSpace(q.Get("theme"))
	lang := strings.TrimSpace(q.Get("lang"))
	// Values sourced from the query string take precedence; the decoded code
	// only fills in whatever wasn't given directly (mirrors theme/lang below).
	appPrefs := extractAppPrefs(q)

	if code := strings.TrimSpace(q.Get("code")); code != "" && (theme == "" || lang == "" || len(appPrefs) == 0) {
		if idx := strings.LastIndex(code, "/server/preferences/import?"); idx != -1 {
			code = code[idx+len("/server/preferences/import?"):]
		}
		if decoded, err := base64.RawURLEncoding.DecodeString(code); err == nil {
			if values, err := url.ParseQuery(string(decoded)); err == nil {
				if theme == "" {
					theme = strings.TrimSpace(values.Get("theme"))
				}
				if lang == "" {
					lang = strings.TrimSpace(values.Get("lang"))
				}
				for k, v := range extractAppPrefs(values) {
					if _, exists := appPrefs[k]; !exists {
						appPrefs[k] = v
					}
				}
			}
		}
	}

	if validThemes[theme] {
		s.setPreferenceCookie(w, r, "theme", theme)
	}
	if lang != "" && i18n.IsSupported(lang) {
		s.setPreferenceCookie(w, r, "lang", strings.ToLower(lang))
	}
	for key, value := range appPrefs {
		s.setPreferenceCookie(w, r, prefCookiePrefix+key, value)
	}

	// Never linger on the visible URL/browser history (AI.md 22908).
	dest := "/"
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host == r.Host {
			if p, ok := safeRedirectPath(u); ok {
				dest = p
			}
		}
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
