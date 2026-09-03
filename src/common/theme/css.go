package theme

import (
	"fmt"
	"strings"
)

// ToCSSVariables converts a ThemePalette to CSS custom property declarations.
// The resulting string can be embedded inside a :root { ... } block.
func (p ThemePalette) ToCSSVariables() string {
	var sb strings.Builder
	vars := []struct {
		name  string
		value string
	}{
		{"--color-background", p.Background},
		{"--color-foreground", p.Foreground},
		{"--color-primary", p.Primary},
		{"--color-secondary", p.Secondary},
		{"--color-accent", p.Accent},
		{"--color-success", p.Success},
		{"--color-warning", p.Warning},
		{"--color-error", p.Error},
		{"--color-info", p.Info},
		{"--color-surface", p.Surface},
		{"--color-surface-alt", p.SurfaceAlt},
		{"--color-border", p.Border},
		{"--color-muted", p.Muted},
	}
	for _, v := range vars {
		fmt.Fprintf(&sb, "  %s: %s;\n", v.name, v.value)
	}
	return sb.String()
}

// ToRootCSS returns a full :root { ... } block for embedding in a stylesheet.
func (p ThemePalette) ToRootCSS() string {
	return ":root {\n" + p.ToCSSVariables() + "}\n"
}
