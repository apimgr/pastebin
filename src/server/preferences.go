package server

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/pastebin/src/common/httputil"
	"github.com/apimgr/pastebin/src/common/i18n"
)

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
	return data
}

// preferencesExportQuery builds the canonical `theme=...&lang=...` query
// string for the current preferences — the query string IS the portable
// preference state (AI.md 22901: "the code/URL is the preference values, not
// a lookup key"), so only theme and lang are ever included (AI.md 22903).
func preferencesExportQuery(theme, lang string) string {
	return fmt.Sprintf("theme=%s&lang=%s", url.QueryEscape(theme), url.QueryEscape(lang))
}

// handlePreferencesExport serves GET /server/preferences/export, API-mirrored
// at GET /api/{api_version}/server/preferences/export (AI.md 22905). It
// returns the current theme/lang preferences as a full importable URL and as
// a base64url short code for manual retyping on a device without copy/paste.
func (s *Server) handlePreferencesExport(w http.ResponseWriter, r *http.Request) {
	theme := s.themeFromRequest(r)
	lang := i18n.LangFromRequest(r)
	query := preferencesExportQuery(theme, lang)
	exportURL := s.baseURL(r) + "/server/preferences/import?" + query
	code := base64.RawURLEncoding.EncodeToString([]byte(query))

	switch detectClientType(r) {
	case "json":
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok": true,
			"data": map[string]interface{}{
				"theme": theme,
				"lang":  lang,
				"url":   exportURL,
				"code":  code,
			},
		})
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "Preferences export\nURL:  %s\nCode: %s\n", exportURL, code)
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

	if code := strings.TrimSpace(q.Get("code")); code != "" && (theme == "" || lang == "") {
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
			}
		}
	}

	if validThemes[theme] {
		s.setPreferenceCookie(w, r, "theme", theme)
	}
	if lang != "" && i18n.IsSupported(lang) {
		s.setPreferenceCookie(w, r, "lang", strings.ToLower(lang))
	}

	// Never linger on the visible URL/browser history (AI.md 22908).
	dest := "/"
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Host == r.Host {
			dest = u.RequestURI()
		}
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}
