package graphql

// nextTheme returns the following mode in the dark → light → auto → dark
// cycle used by the site-wide no-JS theme toggle (AI.md "Themes
// (NON-NEGOTIABLE - PROJECT-WIDE)"). The GraphiQL viewer posts to the same
// /theme endpoint as the rest of the site so its theme stays synchronized
// with the project-wide `theme` cookie instead of keeping independent state.
// Component styles live in the shared static/css/components.css stylesheet
// (PART 16 "one file per context") using the canonical --color-* variables
// defined once in common.css, not a standalone stylesheet with its own names.
func nextTheme(cur string) string {
	switch cur {
	case "dark":
		return "light"
	case "light":
		return "auto"
	default:
		return "dark"
	}
}
