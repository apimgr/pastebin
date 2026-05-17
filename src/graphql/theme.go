package graphql

// CSS returns the theme CSS for the GraphiQL-style query UI.
// Supports dark (default), light, and auto (system preference) themes.
func CSS() string {
	return `/* GraphQL UI theme — dark default, light alternate, auto follows OS */
:root {
  --bg: #282a36;
  --bg-alt: #1e1f29;
  --bg-elevated: #44475a;
  --text: #f8f8f2;
  --text-muted: #6272a4;
  --accent-cyan: #8be9fd;
  --accent-green: #50fa7b;
  --accent-orange: #ffb86c;
  --accent-red: #ff5555;
  --accent-purple: #bd93f9;
  --accent-pink: #ff79c6;
  --accent-yellow: #f1fa8c;
  --border: #44475a;
}

@media (prefers-color-scheme: light) {
  :root {
    --bg: #ffffff;
    --bg-alt: #f5f5f5;
    --bg-elevated: #e0e0e0;
    --text: #1a1a1a;
    --text-muted: #666666;
    --accent-cyan: #0066cc;
    --accent-green: #008000;
    --accent-orange: #ff8c00;
    --accent-red: #cc0000;
    --accent-purple: #6600cc;
    --accent-pink: #c00060;
    --accent-yellow: #806000;
    --border: #cccccc;
  }
}

[data-theme="light"] {
  --bg: #ffffff;
  --bg-alt: #f5f5f5;
  --bg-elevated: #e0e0e0;
  --text: #1a1a1a;
  --text-muted: #666666;
  --accent-cyan: #0066cc;
  --accent-green: #008000;
  --accent-orange: #ff8c00;
  --accent-red: #cc0000;
  --accent-purple: #6600cc;
  --accent-pink: #c00060;
  --accent-yellow: #806000;
  --border: #cccccc;
}

[data-theme="dark"] {
  --bg: #282a36;
  --bg-alt: #1e1f29;
  --bg-elevated: #44475a;
  --text: #f8f8f2;
  --text-muted: #6272a4;
  --accent-cyan: #8be9fd;
  --accent-green: #50fa7b;
  --accent-orange: #ffb86c;
  --accent-red: #ff5555;
  --accent-purple: #bd93f9;
  --accent-pink: #ff79c6;
  --accent-yellow: #f1fa8c;
  --border: #44475a;
}

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
