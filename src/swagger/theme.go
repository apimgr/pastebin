package swagger

import (
	"fmt"

	"github.com/apimgr/pastebin/src/common/theme"
)

// themeVarsBlock renders the CSS custom-property declarations shared by all
// four Swagger UI theme blocks, sourcing values from the canonical
// theme.ThemePalette (src/common/theme/colors.go, single source of truth
// per AI.md 24320) instead of hardcoded hex literals.
func themeVarsBlock(p theme.ThemePalette) string {
	return fmt.Sprintf(`  --bg: %s;
  --bg-alt: %s;
  --bg-elevated: %s;
  --text: %s;
  --text-muted: %s;
  --accent-cyan: %s;
  --accent-green: %s;
  --accent-orange: %s;
  --accent-red: %s;
  --accent-purple: %s;
  --accent-pink: %s;
  --accent-yellow: %s;
  --border: %s;
  --method-get: %s;
  --method-post: %s;
  --method-put: %s;
  --method-delete: %s;
  --method-patch: %s;
`,
		p.Background, p.Surface, p.SurfaceAlt, p.Foreground, p.Muted,
		p.Info, p.Success, p.Warning, p.Error, p.Primary, p.Accent, p.Secondary,
		p.Border, p.Info, p.Success, p.Warning, p.Error, p.Primary)
}

// CSS returns the theme CSS for the Swagger UI viewer.
// Supports dark (default), light, and auto (system preference) themes.
func CSS() string {
	dark := themeVarsBlock(theme.ThemePaletteDark)
	light := themeVarsBlock(theme.ThemePaletteLight)
	return `/* Swagger UI theme — dark default, light alternate, auto follows OS */
:root {
` + dark + `}

@media (prefers-color-scheme: light) {
  :root {
` + light + `  }
}

[data-theme="light"] {
` + light + `}

[data-theme="dark"] {
` + dark + `}

*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  background: var(--bg);
  color: var(--text);
  line-height: 1.6;
  font-size: 15px;
}

.swagger-ui { max-width: 960px; margin: 0 auto; padding: 1rem 1.5rem 4rem; }

header {
  background: var(--bg-alt);
  border-bottom: 1px solid var(--border);
  padding: 0.75rem 1.5rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: sticky;
  top: 0;
  z-index: 10;
}

header h1 { font-size: 1.15rem; color: var(--accent-purple); font-weight: 600; }
header .version { font-size: 0.8rem; color: var(--text-muted); margin-left: 0.75rem; }

.theme-btn {
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  color: var(--text);
  padding: 0.3rem 0.7rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
}

.info-block {
  margin: 1.5rem 0;
  padding: 1rem 1.25rem;
  background: var(--bg-alt);
  border-radius: 6px;
  border-left: 4px solid var(--accent-purple);
}

.info-block p { color: var(--text-muted); margin-top: 0.4rem; font-size: 0.9rem; }

.servers { margin: 1rem 0 1.5rem; font-size: 0.85rem; color: var(--text-muted); }
.servers strong { color: var(--text); }
.servers code {
  background: var(--bg-elevated);
  padding: 0.15rem 0.4rem;
  border-radius: 3px;
  font-family: monospace;
  color: var(--accent-cyan);
}

.tag-section { margin: 2rem 0 0.5rem; }
.tag-label {
  font-size: 1rem;
  font-weight: 600;
  color: var(--accent-yellow);
  border-bottom: 1px solid var(--border);
  padding-bottom: 0.4rem;
  margin-bottom: 0.75rem;
  text-transform: capitalize;
}

.opblock {
  border: 1px solid var(--border);
  border-radius: 6px;
  margin-bottom: 0.6rem;
  overflow: hidden;
}

.opblock-summary {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.65rem 1rem;
  cursor: pointer;
  background: var(--bg-alt);
  user-select: none;
}

.opblock-summary:hover { filter: brightness(1.08); }

.method {
  font-size: 0.7rem;
  font-weight: 700;
  padding: 0.25rem 0.55rem;
  border-radius: 4px;
  min-width: 60px;
  text-align: center;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  background: var(--bg-elevated);
}

.method-get    { color: var(--method-get);    border: 1px solid var(--method-get); }
.method-post   { color: var(--method-post);   border: 1px solid var(--method-post); }
.method-put    { color: var(--method-put);    border: 1px solid var(--method-put); }
.method-delete { color: var(--method-delete); border: 1px solid var(--method-delete); }
.method-patch  { color: var(--method-patch);  border: 1px solid var(--method-patch); }

.opblock-path { font-family: monospace; font-size: 0.9rem; color: var(--text); }
.opblock-summary-desc { font-size: 0.85rem; color: var(--text-muted); margin-left: auto; }

.opblock-body {
  display: none;
  padding: 1rem 1.25rem;
  border-top: 1px solid var(--border);
}

.opblock.open .opblock-body { display: block; }

.section-title {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin: 0.75rem 0 0.4rem;
}

.param-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.85rem;
  margin-bottom: 0.5rem;
}

.param-table th {
  text-align: left;
  padding: 0.35rem 0.6rem;
  background: var(--bg-elevated);
  color: var(--text-muted);
  font-weight: 600;
  font-size: 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.param-table td {
  padding: 0.4rem 0.6rem;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
}

.param-table tr:last-child td { border-bottom: none; }

.param-name { font-family: monospace; color: var(--accent-cyan); }
.param-in   { font-size: 0.75rem; color: var(--text-muted); font-style: italic; }
.param-req  { color: var(--accent-red); font-weight: 600; font-size: 0.75rem; }

.response-block {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  padding: 0.4rem 0;
  border-bottom: 1px solid var(--border);
  font-size: 0.85rem;
}

.response-block:last-child { border-bottom: none; }

.status-code {
  font-family: monospace;
  font-weight: 700;
  min-width: 42px;
  padding: 0.15rem 0.4rem;
  border-radius: 4px;
  text-align: center;
}

.status-2xx { color: var(--accent-green); background: rgba(80,250,123,0.1); }
.status-4xx { color: var(--accent-orange); background: rgba(255,184,108,0.1); }
.status-5xx { color: var(--accent-red); background: rgba(255,85,85,0.1); }

.body-schema {
  background: var(--bg-elevated);
  border-radius: 4px;
  padding: 0.6rem 0.8rem;
  font-family: monospace;
  font-size: 0.8rem;
  color: var(--accent-cyan);
  white-space: pre;
  overflow-x: auto;
  margin-top: 0.4rem;
}

footer {
  margin-top: 3rem;
  padding-top: 1rem;
  border-top: 1px solid var(--border);
  font-size: 0.8rem;
  color: var(--text-muted);
  text-align: center;
}
`
}
