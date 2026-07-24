package server

import (
	"log"
	"net/http"
	"sync"
	"text/template"

	"github.com/apimgr/pastebin/src/common/theme"
)

// cssTemplateData is passed to each *.css.tmpl file so the stylesheets can
// render both palettes as CSS custom properties, keeping
// src/common/theme/colors.go the single source of truth for color values
// shared across Web CSS, TUI, Swagger, and GraphQL (AI.md 24320).
type cssTemplateData struct {
	Dark  theme.ThemePalette
	Light theme.ThemePalette
}

// cssFileNames are the split stylesheets required by AI.md PART 16
// ("One file per context"). Load order (common -> components -> public) is
// enforced by the <link> order in partial/head.tmpl, not by this map.
var cssFileNames = []string{"common", "components", "public"}

type parsedCSSTemplate struct {
	tmpl *template.Template
	err  error
}

var (
	cssTemplateOnce sync.Once
	cssTemplates    map[string]*parsedCSSTemplate
)

// cssParsed lazily parses the embedded {name}.css.tmpl files (text/template,
// not html/template — CSS has no HTML entities to escape) on first use.
func cssParsed(name string) (*template.Template, error) {
	cssTemplateOnce.Do(func() {
		cssTemplates = make(map[string]*parsedCSSTemplate, len(cssFileNames))
		for _, n := range cssFileNames {
			t, err := template.ParseFS(staticFS, "static/css/"+n+".css.tmpl")
			cssTemplates[n] = &parsedCSSTemplate{tmpl: t, err: err}
		}
	})
	entry, ok := cssTemplates[name]
	if !ok {
		return nil, http.ErrMissingFile
	}
	return entry.tmpl, entry.err
}

// handleCSS renders one of common.css / components.css / public.css with the
// canonical dark/light ThemePalette values (AI.md 24290-24355, PART 16).
// Registered as exact chi routes so they take priority over the generic
// /static/* wildcard file server, which would otherwise try to serve the
// renamed .tmpl source files verbatim under a mismatched name.
func (s *Server) handleCSS(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := cssParsed(name)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		data := cssTemplateData{
			Dark:  theme.ThemePaletteDark,
			Light: theme.ThemePaletteLight,
		}
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("%s.css template render failed: %v", name, err)
		}
	}
}
