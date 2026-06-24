package graphql

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Handler handles both the GraphQL POST API and the GraphiQL browser UI.
type Handler struct {
	resolver *Resolver
	title    string
}

// New creates a Handler using the given database and site title.
func New(db DB, title string) *Handler {
	return &Handler{
		resolver: NewResolver(db),
		title:    title,
	}
}

// ServeHTTP dispatches GET (GraphiQL UI) and POST (GraphQL query) requests.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Serve GraphiQL UI unless the client sends a query string (GET query).
		if q := r.URL.Query().Get("query"); q != "" {
			h.serveQuery(w, r, q, nil)
			return
		}
		h.serveUI(w, r)
	case http.MethodPost:
		h.servePost(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// servePost decodes a JSON-encoded GraphQL request and executes it.
func (h *Handler) servePost(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query     string                 `json:"query"`
		Variables map[string]interface{} `json:"variables"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"errors": []map[string]interface{}{{"message": "invalid JSON: " + err.Error()}},
		})
		return
	}
	h.serveQuery(w, r, body.Query, body.Variables)
}

// serveQuery executes a query and writes the GraphQL response.
func (h *Handler) serveQuery(w http.ResponseWriter, _ *http.Request, query string, vars map[string]interface{}) {
	data, errs := h.resolver.Resolve(query, vars)
	resp := map[string]interface{}{"data": data}
	if len(errs) > 0 {
		resp["errors"] = errs
	}
	writeJSON(w, http.StatusOK, resp)
}

// serveUI renders the self-contained GraphiQL HTML interface.
func (h *Handler) serveUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, h.renderUI(r))
}

// writeJSON encodes v as indented JSON and writes it to w.
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, "json encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	w.Write(data)
	w.Write([]byte("\n"))
}

// renderUI returns a self-contained HTML page with an interactive query editor.
// No external CDN assets are loaded.
func (h *Handler) renderUI(r *http.Request) string {
	return `<!DOCTYPE html>
<html lang="en" dir="ltr">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + h.title + ` — GraphQL</title>
    <style>
` + CSS() + `
    </style>
  </head>
  <body>
    <header>
      <h1>` + h.title + ` GraphQL API</h1>
      <button class="theme-btn" onclick="toggleTheme()">Toggle theme</button>
    </header>
    <div class="graphiql-container">
      <div class="pane">
        <div class="pane-header">
          <span>Query</span>
          <button class="execute-button" onclick="runQuery()">&#9654; Run</button>
        </div>
        <textarea id="query" spellcheck="false" autocomplete="off" placeholder="# Enter GraphQL query here&#10;{&#10;  pastes(page: 1, limit: 10) {&#10;    total page limit&#10;    pastes { id title language created_at }&#10;  }&#10;}">{ pastes(page: 1, limit: 5) { total page limit pastes { id title language created_at } } }</textarea>
        <div class="vars-pane">
          <div class="pane-header"><span>Variables (JSON)</span></div>
          <textarea id="vars" spellcheck="false" placeholder='{"id": "abc123"}'></textarea>
        </div>
      </div>
      <div class="pane">
        <div class="pane-header"><span>Response</span></div>
        <div class="result-window" id="result">Run a query to see results.</div>
      </div>
      <div class="schema-panel">
        <h3>Schema</h3>
        <pre>` + escapeHTML(SchemaSDL) + `</pre>
      </div>
    </div>
    <script>
(function() {
  function toggleTheme() {
    var cur = document.documentElement.getAttribute('data-theme');
    document.documentElement.setAttribute('data-theme', cur === 'light' ? 'dark' : 'light');
    try { localStorage.setItem('graphql-theme', document.documentElement.getAttribute('data-theme')); } catch(e) {}
  }
  window.toggleTheme = toggleTheme;

  try {
    var saved = localStorage.getItem('graphql-theme');
    if (saved) document.documentElement.setAttribute('data-theme', saved);
  } catch(e) {}

  function runQuery() {
    var query = document.getElementById('query').value;
    var varsRaw = document.getElementById('vars').value.trim();
    var variables = {};
    if (varsRaw) {
      try { variables = JSON.parse(varsRaw); }
      catch(e) {
        showResult({errors: [{message: 'Invalid variables JSON: ' + e.message}]}, true);
        return;
      }
    }
    var result = document.getElementById('result');
    result.textContent = 'Running…';
    result.className = 'result-window';
    fetch('/api/graphql', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({query: query, variables: variables})
    })
    .then(function(r) { return r.json(); })
    .then(function(data) { showResult(data, !!data.errors); })
    .catch(function(e) { showResult({errors: [{message: String(e)}]}, true); });
  }
  window.runQuery = runQuery;

  function showResult(data, isError) {
    var result = document.getElementById('result');
    result.textContent = JSON.stringify(data, null, 2);
    result.className = 'result-window' + (isError ? ' error' : '');
  }

  document.addEventListener('keydown', function(e) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      runQuery();
    }
  });
})();
    </script>
  </body>
</html>
`
}

// escapeHTML escapes HTML special characters in s.
func escapeHTML(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			out = append(out, []byte("&amp;")...)
		case '<':
			out = append(out, []byte("&lt;")...)
		case '>':
			out = append(out, []byte("&gt;")...)
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
