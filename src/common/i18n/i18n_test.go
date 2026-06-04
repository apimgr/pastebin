package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestIsSupported covers all 7 supported language codes, unsupported codes,
// and normalization (mixed case, whitespace).
func TestIsSupported(t *testing.T) {
	cases := []struct {
		name string
		lang string
		want bool
	}{
		{"en_supported", "en", true},
		{"fr_supported", "fr", true},
		{"de_supported", "de", true},
		{"es_supported", "es", true},
		{"pt_supported", "pt", true},
		{"ja_supported", "ja", true},
		{"zh_supported", "zh", true},
		{"uppercase_EN", "EN", true},
		{"mixed_case_Fr", "Fr", true},
		{"leading_space", " en", true},
		{"trailing_space", "en ", true},
		{"both_spaces", " en ", true},
		{"unsupported_ru", "ru", false},
		{"unsupported_it", "it", false},
		{"unsupported_empty", "", false},
		{"unsupported_xx", "xx", false},
		{"unsupported_en_US", "en-US", false},
		{"unsupported_en_dash_us", "en_US", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsSupported(tc.lang)
			if got != tc.want {
				t.Errorf("IsSupported(%q) = %v, want %v", tc.lang, got, tc.want)
			}
		})
	}
}

// TestTranslate covers known key in en, unknown key fallback, unsupported lang fallback,
// and that a missing key in a foreign lang falls back to en before returning the key itself.
func TestTranslate(t *testing.T) {
	cases := []struct {
		name string
		lang string
		key  string
		want string
	}{
		{"en_known_key", "en", "common.save", "Save"},
		{"fr_known_key", "fr", "common.save", "Enregistrer"},
		{"en_nav_home", "en", "nav.home", "Home"},
		{"unsupported_lang_falls_back_to_en", "xx", "common.save", "Save"},
		{"unknown_key_returns_key", "en", "no.such.key", "no.such.key"},
		{"unknown_key_unsupported_lang_returns_key", "ru", "no.such.key", "no.such.key"},
		{"uppercase_lang_normalized", "EN", "common.save", "Save"},
		{"space_in_lang_normalized", " en ", "common.save", "Save"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Translate(tc.lang, tc.key)
			if got != tc.want {
				t.Errorf("Translate(%q, %q) = %q, want %q", tc.lang, tc.key, got, tc.want)
			}
		})
	}
}

// TestTranslateFormat covers placeholder substitution, multiple placeholders,
// and odd/missing placeholder args (no panic, no substitution).
func TestTranslateFormat(t *testing.T) {
	cases := []struct {
		name string
		lang string
		key  string
		args []interface{}
		want string
	}{
		{
			"single_placeholder",
			"en", "auth.min_length",
			[]interface{}{"min", "8"},
			"At least 8 characters",
		},
		{
			"two_placeholders",
			"en", "common.page_x_of_y",
			[]interface{}{"current", "3", "total", "10"},
			"Page 3 of 10",
		},
		{
			"no_placeholders",
			"en", "common.save",
			nil,
			"Save",
		},
		{
			"missing_placeholder_arg_left_in_string",
			"en", "auth.min_length",
			nil,
			"At least {min} characters",
		},
		{
			"odd_args_last_pair_ignored",
			"en", "common.page_x_of_y",
			[]interface{}{"current", "1", "total"},
			"Page 1 of {total}",
		},
		{
			"unsupported_lang_falls_back_to_en",
			"xx", "common.save",
			nil,
			"Save",
		},
		{
			"swagger_title_placeholder",
			"en", "swagger.title",
			[]interface{}{"app_name", "MyApp"},
			"API Documentation - MyApp",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslateFormat(tc.lang, tc.key, tc.args...)
			if got != tc.want {
				t.Errorf("TranslateFormat(%q, %q, %v) = %q, want %q", tc.lang, tc.key, tc.args, got, tc.want)
			}
		})
	}
}

// TestTranslatePlural covers count=0, count=1, count=2 for languages with
// different plural rules: en (1→one, else→other), fr (≤1→one, else→other),
// zh/ja (always→other), pt (1→one, else→other).
func TestTranslatePlural(t *testing.T) {
	cases := []struct {
		name  string
		lang  string
		key   string
		count int
		want  string
	}{
		// English: standard one/other; count=0 uses "other" (no "zero" form in pluralForm),
		// so the code returns the "other" value "0 items", not the "zero" key "No items".
		{"en_zero", "en", "plurals.items", 0, "0 items"},
		{"en_one", "en", "plurals.items", 1, "1 item"},
		{"en_two", "en", "plurals.items", 2, "2 items"},
		{"en_ten", "en", "plurals.items", 10, "10 items"},

		// French: 0 and 1 both use "one"
		{"fr_zero", "fr", "plurals.items", 0, "0 élément"},
		{"fr_one", "fr", "plurals.items", 1, "1 élément"},
		{"fr_two", "fr", "plurals.items", 2, "2 éléments"},

		// Chinese: all counts use "other"
		{"zh_zero", "zh", "plurals.items", 0, "0 个项目"},
		{"zh_one", "zh", "plurals.items", 1, "1 个项目"},
		{"zh_two", "zh", "plurals.items", 2, "2 个项目"},

		// Japanese: all counts use "other"
		{"ja_zero", "ja", "plurals.items", 0, "0件"},
		{"ja_one", "ja", "plurals.items", 1, "1件"},
		{"ja_two", "ja", "plurals.items", 2, "2件"},

		// Portuguese: pluralForm("pt", 0) returns "other", so count=0 → "0 items"
		// (the "zero" key in the locale file is never consulted by pluralForm).
		{"pt_zero", "pt", "plurals.items", 0, "0 items"},
		{"pt_one", "pt", "plurals.items", 1, "1 item"},
		{"pt_two", "pt", "plurals.items", 2, "2 items"},

		// Count substitution works for "days"
		{"en_one_day", "en", "plurals.days", 1, "1 day"},
		{"en_many_days", "en", "plurals.days", 5, "5 days"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslatePlural(tc.lang, tc.key, tc.count)
			if got != tc.want {
				t.Errorf("TranslatePlural(%q, %q, %d) = %q, want %q", tc.lang, tc.key, tc.count, got, tc.want)
			}
		})
	}
}

// TestLangFromRequest covers the full priority chain:
// ?lang= query param (supported and unsupported), lang cookie,
// Accept-Language header with quality values and subtags, and "en" fallback.
func TestLangFromRequest(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		cookie     string
		acceptLang string
		want       string
	}{
		// Query parameter wins over everything else
		{"query_param_supported", "/?lang=fr", "", "", "fr"},
		{"query_param_unsupported_falls_back_to_en", "/?lang=ru", "", "", "en"},
		{"query_param_uppercase", "/?lang=FR", "", "", "fr"},

		// Cookie wins when no query param
		{"cookie_supported", "/", "fr", "", "fr"},
		{"cookie_unsupported_falls_back_to_en", "/", "ru", "", "en"},

		// Accept-Language header
		{"accept_lang_simple", "/", "", "de", "de"},
		{"accept_lang_en_US_base_match", "/", "", "en-US,en;q=0.9", "en"},
		{"accept_lang_es_with_quality", "/", "", "en-US,en;q=0.9,es;q=0.8", "en"},
		{"accept_lang_first_preferred_match", "/", "", "fr;q=0.9,de;q=0.8", "fr"},
		{"accept_lang_only_subtag", "/", "", "zh-CN;q=1.0", "zh"},
		{"accept_lang_unsupported_falls_back_to_en", "/", "", "ru;q=1.0,uk;q=0.9", "en"},

		// Fallback to "en"
		{"no_hints_returns_en", "/", "", "", "en"},

		// Query param beats cookie
		{"query_beats_cookie", "/?lang=de", "fr", "", "de"},

		// Cookie beats Accept-Language
		{"cookie_beats_accept_lang", "/", "ja", "fr", "ja"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "lang", Value: tc.cookie})
			}
			if tc.acceptLang != "" {
				req.Header.Set("Accept-Language", tc.acceptLang)
			}
			got := LangFromRequest(req)
			if got != tc.want {
				t.Errorf("LangFromRequest (url=%q cookie=%q accept=%q) = %q, want %q",
					tc.url, tc.cookie, tc.acceptLang, got, tc.want)
			}
		})
	}
}

// TestGetLanguage covers the CLI flag path, LC_ALL env, LANG env, and "en" fallback.
// It also exercises validateLang indirectly for supported and unsupported values.
func TestGetLanguage(t *testing.T) {
	cases := []struct {
		name     string
		flagLang string
		lcAll    string
		langEnv  string
		want     string
	}{
		// Explicit flag takes highest priority
		{"flag_supported", "fr", "", "", "fr"},
		{"flag_unsupported_falls_to_en", "ru", "", "", "en"},
		{"flag_uppercase_normalized", "FR", "", "", "fr"},
		{"flag_space_normalized", " fr ", "", "", "fr"},

		// LC_ALL overrides LANG
		{"lc_all_wins", "", "de_DE.UTF-8", "es_ES.UTF-8", "de"},
		{"lc_all_unsupported_falls_to_en", "", "ru_RU.UTF-8", "", "en"},
		{"lc_all_bare", "", "ja", "", "ja"},

		// LANG used when LC_ALL absent
		{"lang_env", "", "", "pt_BR.UTF-8", "pt"},
		{"lang_env_bare", "", "", "zh", "zh"},
		{"lang_env_unsupported", "", "", "it_IT.UTF-8", "en"},

		// Nothing set → "en"
		{"all_empty", "", "", "", "en"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LC_ALL", tc.lcAll)
			t.Setenv("LANG", tc.langEnv)
			got := GetLanguage(tc.flagLang)
			if got != tc.want {
				t.Errorf("GetLanguage(%q) [LC_ALL=%q LANG=%q] = %q, want %q",
					tc.flagLang, tc.lcAll, tc.langEnv, got, tc.want)
			}
		})
	}
}

// TestPluralForm exercises the pluralForm helper indirectly through TranslatePlural
// using the "days" key (which has no "zero" entry, so zero triggers "other" fallback
// on all languages — this is the documented fallback path).
func TestPluralFormFallback(t *testing.T) {
	cases := []struct {
		name  string
		lang  string
		count int
		want  string
	}{
		// "days" has no zero key; pluralForm returns "other" for 0 in most langs.
		// French is special: pluralForm("fr", 0) returns "one" (French treats 0 as singular),
		// so days.one = "{count} jour" → "0 jour".
		{"en_zero_days_fallback", "en", 0, "0 days"},
		{"fr_zero_days_fallback", "fr", 0, "0 jour"},
		{"zh_zero_days_fallback", "zh", 0, "0 天"},
		{"ja_zero_days_fallback", "ja", 0, "0日"},
		{"pt_zero_days_fallback", "pt", 0, "0 days"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TranslatePlural(tc.lang, "plurals.days", tc.count)
			if got != tc.want {
				t.Errorf("TranslatePlural(%q, plurals.days, %d) = %q, want %q",
					tc.lang, tc.count, got, tc.want)
			}
		})
	}
}

// TestLocaleFS verifies the embedded FS is accessible and non-empty.
func TestLocaleFS(t *testing.T) {
	fs := LocaleFS()
	data, err := fs.ReadFile("locales/en.json")
	if err != nil {
		t.Fatalf("LocaleFS ReadFile locales/en.json: %v", err)
	}
	if len(data) == 0 {
		t.Error("LocaleFS: en.json is empty")
	}
}
