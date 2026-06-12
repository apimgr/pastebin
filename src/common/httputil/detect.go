package httputil

import (
	"net/http"
	"strings"
)

// projectName is the CLI binary prefix used in User-Agent detection.
const projectName = "pastebin"

// IsOurCliClient detects our own client binary (pastebin-cli).
// Our client is INTERACTIVE (TUI/GUI) — receives JSON and renders itself.
func IsOurCliClient(r *http.Request) bool {
	ua := r.Header.Get("User-Agent")
	return strings.HasPrefix(ua, projectName+"-cli/")
}

// IsTextBrowser detects text-mode browsers (lynx, w3m, links, etc.).
// Text browsers are INTERACTIVE but do NOT support JavaScript.
// They receive no-JS HTML (server-rendered, standard form POST).
func IsTextBrowser(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))

	textBrowsers := []string{
		"lynx/",     // Lynx — classic text browser
		"w3m/",      // w3m — text browser with table support
		"links ",    // Links — text browser (note: space after)
		"links/",    // Links alternative format
		"elinks/",   // ELinks — enhanced links
		"browsh/",   // Browsh — modern text browser
		"carbonyl/", // Carbonyl — Chromium in terminal
		"netsurf",   // NetSurf — lightweight browser (limited JS)
	}
	for _, browser := range textBrowsers {
		if strings.Contains(ua, browser) {
			return true
		}
	}
	return false
}

// IsHttpTool detects HTTP tools (curl, wget, httpie, etc.).
// HTTP tools are NON-INTERACTIVE — they just dump output.
func IsHttpTool(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))

	httpTools := []string{
		"curl/", "wget/", "httpie/",
		"libcurl/", "python-requests/",
		"go-http-client/", "axios/", "node-fetch/",
	}
	for _, tool := range httpTools {
		if strings.Contains(ua, tool) {
			return true
		}
	}

	// No User-Agent = likely HTTP tool (non-interactive).
	if ua == "" {
		return true
	}

	return false
}

// IsNonInteractiveClient reports whether the client needs pre-formatted text.
// Only HTTP tools are non-interactive.
// Our client and text browsers are INTERACTIVE (handle their own rendering).
func IsNonInteractiveClient(r *http.Request) bool {
	// Our client is INTERACTIVE — receives JSON.
	if IsOurCliClient(r) {
		return false
	}

	// Text browsers are INTERACTIVE — receive no-JS HTML, render it themselves.
	if IsTextBrowser(r) {
		return false
	}

	// HTTP tools are NON-INTERACTIVE — need pre-formatted text.
	if IsHttpTool(r) {
		return true
	}

	return false
}

// GetAPIResponseFormat determines the response format for /api/** routes.
// API routes default to JSON; plain text is returned for .txt extension,
// Accept: text/plain header, or non-interactive clients (curl, wget, etc.).
func GetAPIResponseFormat(r *http.Request) string {
	// .txt extension always wins regardless of other signals.
	if strings.HasSuffix(r.URL.Path, ".txt") {
		return "text"
	}

	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/plain") {
		return "text"
	}

	// Non-interactive clients (curl, wget, empty UA) get plain text.
	if IsNonInteractiveClient(r) {
		return "text"
	}

	// Default: JSON.
	return "json"
}
