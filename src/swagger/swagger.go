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
	title      string
	version    string
	baseURL    string                        // static override; takes precedence over baseURLFn
	baseURLFn  func(*http.Request) string    // dynamic resolver; used when baseURL is empty
}

// New creates a Handler. baseURL can be left empty to auto-detect from the request.
func New(title, version, baseURL string) *Handler {
	return &Handler{title: title, version: version, baseURL: baseURL}
}

// SetBaseURLResolver registers a trusted dynamic base-URL resolver.
// When set, it is called instead of the bare request-header fallback when
// the static baseURL field is empty. The resolver is expected to honour
// the PART 12 trusted-proxy rules (e.g. Server.baseURL).
func (h *Handler) SetBaseURLResolver(fn func(*http.Request) string) {
	h.baseURLFn = fn
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

// ServeUI writes the self-contained HTML Swagger viewer.
func (h *Handler) ServeUI(w http.ResponseWriter, r *http.Request) {
	base := h.resolveBase(r)
	specURL := base + "/api/v1/server/swagger"
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, h.renderUI(specURL))
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
	for _, route := range Routes() {
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

// renderUI returns a self-contained HTML page that fetches the JSON spec from
// specURL and renders it as interactive documentation. No CDN assets are used.
func (h *Handler) renderUI(specURL string) string {
	return `<!DOCTYPE html>
<html lang="en" dir="ltr">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + h.title + ` — API Docs</title>
    <style>
` + CSS() + `
    </style>
  </head>
  <body>
    <header>
      <div>
        <span style="font-size:1.15rem;font-weight:600;color:var(--accent-purple)">` + h.title + `</span>
        <span class="version">v` + h.version + `</span>
      </div>
      <button class="theme-btn" onclick="toggleTheme()">Toggle theme</button>
    </header>
    <main class="swagger-ui" id="app">
      <p style="color:var(--text-muted);margin-top:2rem">Loading API specification…</p>
    </main>
    <script>
(function() {
  var specURL = ` + "`" + specURL + "`" + `;

  function toggleTheme() {
    var cur = document.documentElement.getAttribute('data-theme');
    document.documentElement.setAttribute('data-theme', cur === 'light' ? 'dark' : 'light');
    try { localStorage.setItem('theme', document.documentElement.getAttribute('data-theme')); } catch(e) {}
  }
  window.toggleTheme = toggleTheme;

  try {
    var saved = localStorage.getItem('theme');
    if (saved) document.documentElement.setAttribute('data-theme', saved);
  } catch(e) {}

  function esc(s) {
    return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
  }

  function statusClass(code) {
    if (code >= 200 && code < 300) return 'status-2xx';
    if (code >= 400 && code < 500) return 'status-4xx';
    return 'status-5xx';
  }

  function schemaSnippet(schema) {
    if (!schema) return '';
    try { return JSON.stringify(schema, null, 2); } catch(e) { return ''; }
  }

  function renderParam(p) {
    return '<tr>' +
      '<td><span class="param-name">' + esc(p.name) + '</span> <span class="param-in">(' + esc(p.in) + ')</span></td>' +
      '<td>' + (p.required ? '<span class="param-req">required</span>' : '<span style="color:var(--text-muted)">optional</span>') + '</td>' +
      '<td>' + esc(p.description || '') + '</td>' +
      '<td><code style="font-size:0.8rem;color:var(--accent-yellow)">' + esc((p.schema && p.schema.type) || '') + '</code></td>' +
      '</tr>';
  }

  function renderResponse(code, resp) {
    var sc = statusClass(parseInt(code));
    var schema = '';
    if (resp.content) {
      var ct = Object.keys(resp.content)[0];
      if (ct && resp.content[ct] && resp.content[ct].schema) {
        schema = '<div class="body-schema">' + esc(schemaSnippet(resp.content[ct].schema)) + '</div>';
      }
    }
    return '<div class="response-block">' +
      '<span class="status-code ' + sc + '">' + esc(code) + '</span>' +
      '<div><div>' + esc(resp.description || '') + '</div>' + schema + '</div>' +
      '</div>';
  }

  function renderOp(method, path, op, idx) {
    var id = 'op-' + idx;
    var params = (op.parameters || []).map(renderParam).join('');
    var paramSection = params ? '<div class="section-title">Parameters</div><table class="param-table"><thead><tr><th>Name</th><th>Required</th><th>Description</th><th>Type</th></tr></thead><tbody>' + params + '</tbody></table>' : '';

    var bodySection = '';
    if (op.requestBody && op.requestBody.content) {
      var ct = Object.keys(op.requestBody.content)[0];
      var schema = ct && op.requestBody.content[ct] && op.requestBody.content[ct].schema ? op.requestBody.content[ct].schema : null;
      bodySection = '<div class="section-title">Request Body</div>';
      if (op.requestBody.description) bodySection += '<p style="font-size:0.85rem;color:var(--text-muted);margin-bottom:0.4rem">' + esc(op.requestBody.description) + '</p>';
      if (schema) bodySection += '<div class="body-schema">' + esc(schemaSnippet(schema)) + '</div>';
    }

    var responses = op.responses || {};
    var respSection = '<div class="section-title">Responses</div>' +
      Object.keys(responses).sort().map(function(c) { return renderResponse(c, responses[c]); }).join('');

    return '<div class="opblock" id="' + id + '">' +
      '<div class="opblock-summary" onclick="toggleOp(\'' + id + '\')">' +
      '<span class="method method-' + method.toLowerCase() + '">' + esc(method) + '</span>' +
      '<span class="opblock-path">' + esc(path) + '</span>' +
      '<span class="opblock-summary-desc">' + esc(op.summary || '') + '</span>' +
      '</div>' +
      '<div class="opblock-body">' +
      (op.description ? '<p style="color:var(--text-muted);margin-bottom:0.75rem;font-size:0.9rem">' + esc(op.description) + '</p>' : '') +
      paramSection + bodySection + respSection +
      '</div></div>';
  }

  function toggleOp(id) {
    var el = document.getElementById(id);
    if (el) el.classList.toggle('open');
  }
  window.toggleOp = toggleOp;

  function render(spec) {
    var tags = {};
    var paths = spec.paths || {};
    Object.keys(paths).forEach(function(path) {
      var item = paths[path];
      Object.keys(item).forEach(function(method) {
        var op = item[method];
        var tag = (op.tags && op.tags[0]) || 'other';
        if (!tags[tag]) tags[tag] = [];
        tags[tag].push({ method: method.toUpperCase(), path: path, op: op });
      });
    });

    var info = spec.info || {};
    var servers = (spec.servers || []).map(function(s) {
      return '<code>' + esc(s.url) + '</code>' + (s.description ? ' <span style="color:var(--text-muted)">— ' + esc(s.description) + '</span>' : '');
    }).join(', ');

    var html = '<div class="info-block"><strong style="color:var(--text)">' + esc(info.title || '') + '</strong>' +
      (info.description ? '<p>' + esc(info.description) + '</p>' : '') + '</div>';
    if (servers) html += '<div class="servers"><strong>Server:</strong> ' + servers + '</div>';

    var idx = 0;
    Object.keys(tags).sort().forEach(function(tag) {
      html += '<div class="tag-section"><div class="tag-label">' + esc(tag) + '</div>';
      tags[tag].forEach(function(item) {
        html += renderOp(item.method, item.path, item.op, idx++);
      });
      html += '</div>';
    });

    html += '<footer>OpenAPI 3.0.3 · <a href="' + esc(specURL) + '" style="color:var(--accent-cyan)">JSON spec</a></footer>';
    document.getElementById('app').innerHTML = html;
  }

  fetch(specURL)
    .then(function(r) { return r.json(); })
    .then(render)
    .catch(function(e) {
      document.getElementById('app').innerHTML = '<p style="color:var(--accent-red);margin-top:2rem">Failed to load spec: ' + esc(String(e)) + '</p>';
    });
})();
    </script>
  </body>
</html>
`
}
