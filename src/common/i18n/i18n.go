package i18n

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// localeFS embeds all locale JSON files at compile time
//
//go:embed locales/*.json
var localeFS embed.FS

// supported languages (code → true)
var supportedLangs = map[string]bool{
	"en": true,
	"es": true,
	"fr": true,
	"de": true,
	"zh": true,
	"ar": true,
	"ja": true,
}

// translations holds flattened key→value maps per language
var translations = map[string]map[string]string{}

func init() {
	for lang := range supportedLangs {
		data, err := localeFS.ReadFile("locales/" + lang + ".json")
		if err != nil {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		flat := map[string]string{}
		flattenJSON("", raw, flat)
		translations[lang] = flat
	}
}

// flattenJSON converts a nested JSON object into dot-separated flat keys.
func flattenJSON(prefix string, obj map[string]interface{}, out map[string]string) {
	for k, v := range obj {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case string:
			out[key] = val
		case map[string]interface{}:
			flattenJSON(key, val, out)
		}
	}
}

// IsSupported returns true if lang is a supported language code.
func IsSupported(lang string) bool {
	return supportedLangs[strings.ToLower(strings.TrimSpace(lang))]
}

// Translate returns the translated string for the given key.
// If lang is unsupported, falls back to "en". If key is missing, falls back to "en" then returns key.
func Translate(lang, key string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if !IsSupported(lang) {
		lang = "en"
	}
	if val, ok := translations[lang][key]; ok {
		return val
	}
	// fallback to English
	if val, ok := translations["en"][key]; ok {
		return val
	}
	return key
}

// TranslateFormat translates key and substitutes {variable} placeholders.
// args should be pairs of (variable, value) strings.
func TranslateFormat(lang, key string, args ...interface{}) string {
	s := Translate(lang, key)
	for i := 0; i+1 < len(args); i += 2 {
		placeholder := "{" + toString(args[i]) + "}"
		s = strings.ReplaceAll(s, placeholder, toString(args[i+1]))
	}
	return s
}

// TranslatePlural returns a pluralized translation for a count value.
// Looks up key.one, key.other, key.zero, etc. based on count.
func TranslatePlural(lang, key string, count int) string {
	form := pluralForm(lang, count)
	k := key + "." + form
	if val := Translate(lang, k); val != k {
		return strings.ReplaceAll(val, "{count}", toString(count))
	}
	// try "other" as fallback
	k = key + ".other"
	val := Translate(lang, k)
	return strings.ReplaceAll(val, "{count}", toString(count))
}

// pluralForm returns the CLDR plural category for a count in a given language.
func pluralForm(lang string, count int) string {
	switch lang {
	case "ar":
		// Arabic: six CLDR plural forms
		if count == 0 {
			return "zero"
		}
		if count == 1 {
			return "one"
		}
		if count == 2 {
			return "two"
		}
		mod100 := count % 100
		mod10 := count % 10
		if mod10 >= 3 && mod10 <= 10 {
			return "few"
		}
		if mod100 >= 11 && mod100 <= 99 {
			return "many"
		}
		if mod10 == 0 || mod100 == 100 {
			return "zero"
		}
		return "other"
	case "fr":
		// French: 0 and 1 are "one"
		if count <= 1 {
			return "one"
		}
		return "other"
	case "zh", "ja":
		// No plural forms
		return "other"
	default:
		// English, Spanish, German, etc.
		if count == 1 {
			return "one"
		}
		return "other"
	}
}

// LangFromRequest extracts the preferred language from an HTTP request.
// Priority: ?lang= param → lang cookie → Accept-Language header → "en"
func LangFromRequest(r *http.Request) string {
	// 1. Query parameter
	if lang := r.URL.Query().Get("lang"); lang != "" {
		if IsSupported(lang) {
			return strings.ToLower(lang)
		}
		return "en"
	}
	// 2. Cookie
	if c, err := r.Cookie("lang"); err == nil && c.Value != "" {
		if IsSupported(c.Value) {
			return strings.ToLower(c.Value)
		}
		return "en"
	}
	// 3. Accept-Language header
	if accept := r.Header.Get("Accept-Language"); accept != "" {
		if lang := parseBestMatch(accept); lang != "" {
			return lang
		}
	}
	return "en"
}

// parseBestMatch parses an Accept-Language header and returns the best supported match.
func parseBestMatch(header string) string {
	// Accept-Language: en-US,en;q=0.9,es;q=0.8
	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		// strip quality value
		if idx := strings.Index(part, ";"); idx != -1 {
			part = part[:idx]
		}
		lang := strings.ToLower(strings.TrimSpace(part))
		// Try full tag first (e.g. "en-us"), then base (e.g. "en")
		if IsSupported(lang) {
			return lang
		}
		if idx := strings.Index(lang, "-"); idx != -1 {
			base := lang[:idx]
			if IsSupported(base) {
				return base
			}
		}
	}
	return ""
}

// GetLanguage determines the output language from CLI flag, config, and env vars.
// Never errors; silently falls back to "en" for unsupported languages.
func GetLanguage(flagLang string) string {
	// 1. Explicit --lang flag
	if flagLang != "" {
		return validateLang(flagLang)
	}
	// 2. LC_ALL env var
	if lang := os.Getenv("LC_ALL"); lang != "" {
		return validateLang(strings.Split(lang, "_")[0])
	}
	// 3. LANG env var
	if lang := os.Getenv("LANG"); lang != "" {
		return validateLang(strings.Split(lang, "_")[0])
	}
	return "en"
}

// validateLang returns lang if supported, otherwise "en".
func validateLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if IsSupported(lang) {
		return lang
	}
	return "en"
}

// toString converts an interface{} to string.
func toString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case int:
		return fmt_int(s)
	default:
		b, _ := json.Marshal(v)
		return strings.Trim(string(b), `"`)
	}
}

// fmt_int converts int to string without importing fmt
func fmt_int(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// LocaleFS returns the embedded locale filesystem for serving via HTTP.
func LocaleFS() embed.FS {
	return localeFS
}
