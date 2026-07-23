package server

import (
	"log"
	"net/http"
	"sync"
	"text/template"

	"github.com/apimgr/pastebin/src/common/theme"
)

// mainCSSTemplateData is passed to main.css.tmpl so the stylesheet can
// render both palettes as CSS custom properties, keeping
// src/common/theme/colors.go the single source of truth for color values
// shared across Web CSS, TUI, Swagger, and GraphQL (AI.md 24320).
type mainCSSTemplateData struct {
	Dark  theme.ThemePalette
	Light theme.ThemePalette
}

var (
	mainCSSTemplateOnce sync.Once
	mainCSSTemplate     *template.Template
	mainCSSTemplateErr  error
)

// mainCSSParsed lazily parses the embedded main.css.tmpl (text/template,
// not html/template — CSS has no HTML entities to escape) on first use.
func mainCSSParsed() (*template.Template, error) {
	mainCSSTemplateOnce.Do(func() {
		mainCSSTemplate, mainCSSTemplateErr = template.ParseFS(staticFS, "static/css/main.css.tmpl")
	})
	return mainCSSTemplate, mainCSSTemplateErr
}

// handleMainCSS renders main.css.tmpl with the canonical dark/light
// ThemePalette values (AI.md 24290-24355, PART 16). Registered as an exact
// chi route so it takes priority over the generic /static/* wildcard file
// server, which would otherwise try to serve the renamed .tmpl source file
// verbatim under a mismatched name.
func (s *Server) handleMainCSS(w http.ResponseWriter, r *http.Request) {
	tmpl, err := mainCSSParsed()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	data := mainCSSTemplateData{
		Dark:  theme.ThemePaletteDark,
		Light: theme.ThemePaletteLight,
	}
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("main.css template render failed: %v", err)
	}
}
