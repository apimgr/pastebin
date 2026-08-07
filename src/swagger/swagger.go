// Package swagger provides OpenAPI 3.0.3 spec generation and a self-contained
// Swagger UI viewer for the pastebin API. No external CDN assets are used.
package swagger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Handler serves OpenAPI-related endpoints.
// JSON spec: GET /api/v1/server/swagger
// HTML UI:   GET /server/swagger (and /server/docs/swagger)
type Handler struct {
	title   string
	version string
	// apiVersion is the {api_version} route segment (PART 14); defaults to "v1".
	apiVersion string
	// static override; takes precedence over baseURLFn
	baseURL string
	// dynamic resolver; used when baseURL is empty
	baseURLFn func(*http.Request) string
	// assetPrefixFn resolves the {baseurl} path prefix (PART 12) for
	// static asset links (CSS/JS); returns "" when unset.
	assetPrefixFn func(*http.Request) string
	// themeFn resolves the project-wide theme (dark/light/auto) for the
	// current request so the viewer renders class="theme-{mode}" in sync
	// with the rest of the site; returns "dark" when unset.
	themeFn func(*http.Request) string
	// csrfTokenFn resolves the CSRF token bound to the request's csrf_token
	// cookie, used by the no-JS theme-toggle POST form; returns "" when unset.
	csrfTokenFn func(*http.Request) string
}

// New creates a Handler. baseURL can be left empty to auto-detect from the request.
// apiVersion is the {api_version} route segment (e.g. "v1"); empty defaults to "v1".
func New(title, version, baseURL, apiVersion string) *Handler {
	if apiVersion == "" {
		apiVersion = "v1"
	}
	return &Handler{title: title, version: version, baseURL: baseURL, apiVersion: apiVersion}
}

// SetBaseURLResolver registers a trusted dynamic base-URL resolver.
// When set, it is called instead of the bare request-header fallback when
// the static baseURL field is empty. The resolver is expected to honour
// the PART 12 trusted-proxy rules (e.g. Server.baseURL).
func (h *Handler) SetBaseURLResolver(fn func(*http.Request) string) {
	h.baseURLFn = fn
}

// SetAssetPrefixResolver registers the {baseurl} path-prefix resolver used to
// build the CSS/JS asset links emitted by the UI (PART 12 baseurl resolution).
func (h *Handler) SetAssetPrefixResolver(fn func(*http.Request) string) {
	h.assetPrefixFn = fn
}

// assetPrefix returns the resolved baseurl path prefix, or "" when unset.
func (h *Handler) assetPrefix(r *http.Request) string {
	if h.assetPrefixFn == nil {
		return ""
	}
	return h.assetPrefixFn(r)
}

// SetThemeResolver registers the project-wide theme resolver (reads the same
// `theme` cookie the rest of the site uses) so the Swagger UI viewer stays
// synchronized with the site-wide theme toggle instead of keeping its own
// independent state.
func (h *Handler) SetThemeResolver(fn func(*http.Request) string) {
	h.themeFn = fn
}

// theme returns the resolved theme mode ("dark", "light", or "auto"),
// defaulting to "dark" when no resolver is registered.
func (h *Handler) theme(r *http.Request) string {
	if h.themeFn == nil {
		return "dark"
	}
	if t := h.themeFn(r); t != "" {
		return t
	}
	return "dark"
}

// SetCSRFTokenResolver registers the CSRF token resolver used by the no-JS
// theme-toggle POST form (double-submit cookie pattern, PART 16).
func (h *Handler) SetCSRFTokenResolver(fn func(*http.Request) string) {
	h.csrfTokenFn = fn
}

// csrfToken returns the resolved CSRF token, or "" when unset.
func (h *Handler) csrfToken(r *http.Request) string {
	if h.csrfTokenFn == nil {
		return ""
	}
	return h.csrfTokenFn(r)
}

// ServeSpec writes the OpenAPI 3.0.3 JSON specification.
// SetEscapeHTML(false) prevents < > & in description fields from being mangled.
func (h *Handler) ServeSpec(w http.ResponseWriter, r *http.Request) {
	spec := h.buildSpec(h.resolveBase(r))
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(spec); err != nil {
		http.Error(w, "spec generation error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes())
}

// ServeUI writes the HTML Swagger viewer shell. Styling loads from the
// shared components.css stylesheet and interactivity from the site's single
// JS file (PART 16 — no inline <style>/<script>); the CSP ships
// script-src 'self' only, so the viewer's rendering script must live in
// static/js/app.js.
func (h *Handler) ServeUI(w http.ResponseWriter, r *http.Request) {
	base := h.resolveBase(r)
	specURL := base + "/api/" + h.apiVersion + "/server/swagger"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, h.renderUI(r, specURL))
}

// resolveBase returns the effective base URL for the current request.
// Priority: static baseURL field → registered trusted resolver → bare connection.
// X-Forwarded-* headers are never read here; the resolver (e.g. Server.baseURL)
// applies the PART 12 trusted-proxy gate instead (header-spoofing guard).
func (h *Handler) resolveBase(r *http.Request) string {
	if h.baseURL != "" {
		return h.baseURL
	}
	if h.baseURLFn != nil {
		return h.baseURLFn(r)
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// buildSpec generates the OpenAPI 3.0.3 document from the route annotations.
func (h *Handler) buildSpec(base string) map[string]interface{} {
	paths := map[string]interface{}{}
	for _, route := range Routes(h.apiVersion) {
		p, ok := paths[route.Path].(map[string]interface{})
		if !ok {
			p = map[string]interface{}{}
		}

		op := map[string]interface{}{
			"summary":     route.Summary,
			"description": route.Description,
			"tags":        []string{route.Tag},
			"operationId": operationID(route.Method, route.Path),
		}

		if len(route.Params) > 0 {
			params := make([]map[string]interface{}, 0, len(route.Params))
			for _, param := range route.Params {
				p := map[string]interface{}{
					"name":        param.Name,
					"in":          param.In,
					"required":    param.Required,
					"description": param.Description,
					"schema":      param.Schema,
				}
				params = append(params, p)
			}
			op["parameters"] = params
		}

		if route.Body != nil {
			op["requestBody"] = map[string]interface{}{
				"required":    route.Body.Required,
				"description": route.Body.Description,
				"content": map[string]interface{}{
					route.Body.ContentType: map[string]interface{}{
						"schema": route.Body.Schema,
					},
				},
			}
		}

		if len(route.Responses) > 0 {
			responses := map[string]interface{}{}
			for code, resp := range route.Responses {
				r := map[string]interface{}{"description": resp.Description}
				if resp.ContentType != "" && resp.Schema != nil {
					r["content"] = map[string]interface{}{
						resp.ContentType: map[string]interface{}{
							"schema": resp.Schema,
						},
					}
				}
				responses[fmt.Sprintf("%d", code)] = r
			}
			op["responses"] = responses
		}

		p[strings.ToLower(route.Method)] = op
		paths[route.Path] = p
	}

	return map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":   h.title,
			"version": h.version,
			"description": "A fast, anonymous pastebin service with REST and GraphQL APIs. " +
				"Compatible with pastebin.com, microbin, and lenpaste.",
			"license": map[string]interface{}{
				"name": "MIT",
				"url":  "https://opensource.org/licenses/MIT",
			},
		},
		"servers": []map[string]interface{}{
			{"url": base, "description": "This server"},
		},
		"tags": []map[string]interface{}{
			{"name": "pastes", "description": "Paste creation and retrieval"},
			{"name": "server", "description": "Server health and metadata"},
		},
		"paths": paths,
	}
}

// operationID produces a unique, human-readable operation ID from method + path.
func operationID(method, path string) string {
	// e.g. GET /api/v1/pastes/{id} → getApiV1PastesId
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := strings.ToLower(method)
	for _, p := range parts {
		p = strings.Trim(p, "{}")
		if p == "" {
			continue
		}
		out += strings.ToUpper(p[:1]) + p[1:]
	}
	return out
}

// renderUI returns the Swagger UI viewer shell. Component styling lives in
// the shared static/css/components.css stylesheet and behaviour in the
// site's single JS file (static/js/app.js) — PART 16 forbids inline
// <style>/<script> and inline event-handler attributes, and the CSP ships
// script-src 'self' only. The theme class and toggle form mirror the
// project-wide mechanism (server-rendered `theme` cookie + no-JS POST to
// /theme) so this viewer never keeps independent theme state.
func (h *Handler) renderUI(r *http.Request, specURL string) string {
	assetPrefix := h.assetPrefix(r)
	theme := h.theme(r)
	return `<!DOCTYPE html>
<html lang="en" dir="ltr" class="theme-` + theme + `">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + h.title + ` — API Docs</title>
    <link rel="stylesheet" href="` + assetPrefix + `/static/css/components.css">
  </head>
  <body class="swagger-page">
    <header>
      <div>
        <h1>` + h.title + `</h1>
        <span class="version">v` + h.version + `</span>
      </div>
      <form class="theme-toggle" method="post" action="/theme"><input type="hidden" name="csrf_token" value="` + h.csrfToken(r) + `"><input type="hidden" name="theme" value="` + nextTheme(theme) + `"><button type="submit" class="theme-button" data-theme-toggle aria-label="Toggle theme">🌙</button></form>
    </header>
    <main class="swagger-ui" id="app" data-spec-url="` + specURL + `">
      <p class="loading-message">Loading API specification…</p>
    </main>
    <script src="` + assetPrefix + `/static/js/app.js"></script>
  </body>
</html>
`
}
