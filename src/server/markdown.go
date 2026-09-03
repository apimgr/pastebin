package server

import (
	"bytes"
	"html/template"
	"sync"

	"github.com/yuin/goldmark"
)

// markdownInstance is the shared goldmark renderer used to convert operator-set
// privacy/config Markdown into HTML. It runs with CommonMark defaults and does
// NOT enable raw-HTML passthrough (WithUnsafe), so operator content cannot
// inject <script> or other active markup.
var (
	markdownOnce     sync.Once
	markdownInstance goldmark.Markdown
)

// markdownEngine lazily builds the shared goldmark instance.
func markdownEngine() goldmark.Markdown {
	markdownOnce.Do(func() {
		markdownInstance = goldmark.New()
	})
	return markdownInstance
}

// renderMarkdown converts trusted operator-configured Markdown to template.HTML.
// Only operator config content (server.privacy.content.*) is passed here — never
// end-user paste content, which must always flow through html/template escaping.
func renderMarkdown(src string) template.HTML {
	if src == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := markdownEngine().Convert([]byte(src), &buf); err != nil {
		// Fall back to escaped plain text on conversion failure.
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}
