package graphql

import (
	"fmt"

	"github.com/apimgr/pastebin/src/common/theme"
)

// themeVarsBlock renders the CSS custom-property declarations shared by all
// four GraphiQL theme blocks, sourcing values from the canonical
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
`,
		p.Background, p.Surface, p.SurfaceAlt, p.Foreground, p.Muted,
		p.Info, p.Success, p.Warning, p.Error, p.Primary, p.Accent, p.Secondary,
		p.Border)
}

// CSS returns the theme CSS for the GraphiQL-style query UI.
// Supports dark (default), light, and auto (system preference) themes.
func CSS() string {
	dark := themeVarsBlock(theme.ThemePaletteDark)
	light := themeVarsBlock(theme.ThemePaletteLight)
	return `/* GraphQL UI theme — dark default, light alternate, auto follows OS */
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
  height: 100vh;
  display: flex;
  flex-direction: column;
}

header {
  background: var(--bg-alt);
  border-bottom: 1px solid var(--border);
  padding: 0.6rem 1rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
}

header h1 { font-size: 1rem; color: var(--accent-purple); font-weight: 600; }

.theme-btn {
  background: var(--bg-elevated);
  border: 1px solid var(--border);
  color: var(--text);
  padding: 0.25rem 0.6rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
}

.graphiql-container {
  display: flex;
  flex: 1;
  overflow: hidden;
  gap: 0;
}

.pane {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-right: 1px solid var(--border);
}

.pane:last-child { border-right: none; }

.pane-header {
  background: var(--bg-alt);
  padding: 0.4rem 0.75rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
}

textarea, .result-window {
  flex: 1;
  width: 100%;
  resize: none;
  border: none;
  outline: none;
  background: var(--bg);
  color: var(--text);
  font-family: "Fira Code", "Cascadia Code", Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  padding: 0.75rem 1rem;
  overflow-y: auto;
}

.result-window {
  white-space: pre-wrap;
  word-break: break-word;
}

.result-window.error { color: var(--accent-red); }

.execute-button {
  background: var(--accent-green);
  color: #282a36;
  border: none;
  padding: 0.3rem 0.85rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.8rem;
  font-weight: 700;
  letter-spacing: 0.03em;
}

.execute-button:hover { filter: brightness(1.1); }

.vars-pane {
  flex-shrink: 0;
  border-top: 1px solid var(--border);
  max-height: 120px;
  display: flex;
  flex-direction: column;
}

.schema-panel {
  width: 260px;
  flex-shrink: 0;
  border-left: 1px solid var(--border);
  overflow-y: auto;
  padding: 0.75rem;
  font-size: 0.8rem;
}

.schema-panel h3 {
  color: var(--accent-purple);
  margin-bottom: 0.5rem;
  font-size: 0.85rem;
}

.schema-panel pre {
  color: var(--accent-cyan);
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
  font-family: monospace;
  font-size: 0.78rem;
}

@media (max-width: 700px) {
  .schema-panel { display: none; }
  .graphiql-container { flex-direction: column; }
}
`
}
