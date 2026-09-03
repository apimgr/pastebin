package config

import (
	"github.com/microcosm-cc/bluemonday"
)

// footerPolicy is the strict sanitization policy for operator footer branding
// (PART 16). It permits only basic text-formatting tags, safe links, and images
// from https/data URLs; scripts, event handlers, javascript: URLs, forms, and
// the style attribute are stripped.
func footerPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "span", "div")
	p.AllowElements("strong", "b", "em", "i", "u", "s", "small")
	p.AllowElements("h1", "h2", "h3", "h4", "h5", "h6")
	p.AllowElements("ul", "ol", "li")
	p.AllowAttrs("href", "title", "target", "rel").OnElements("a")
	p.RequireNoReferrerOnLinks(true)
	p.AllowAttrs("src", "alt", "title", "width", "height").OnElements("img")
	p.AllowURLSchemes("https", "data")
	p.AllowAttrs("class", "id").Globally()
	return p
}

// SanitizeFooterHTML sanitizes operator footer branding HTML (PART 16). Empty
// input and a single space (the "disable branding" sentinel) pass through
// unchanged; all other input is stripped down to the safe policy above.
func SanitizeFooterHTML(html string) string {
	if html == "" || html == " " {
		return html
	}
	return footerPolicy().Sanitize(html)
}

// FooterCustomHTML returns the sanitized operator footer branding for rendering.
// The single-space "disable" sentinel resolves to an empty string so no branding
// row is emitted.
func (c *Config) FooterCustomHTML() string {
	raw := c.Web.Footer.CustomHTML
	if raw == " " {
		return ""
	}
	return SanitizeFooterHTML(raw)
}
