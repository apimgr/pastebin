package server

// SEO meta tag generation and site-verification validation (PART 16
// "Branding & SEO" / "Site Verification Meta Tags", AI.md 24380-24537).
//
// Title/tagline/description/keywords/author/og_image/twitter_handle feed the
// standard SEO + OpenGraph + Twitter Card tags, always rendered. Verification
// codes (google/bing/yandex/baidu/pinterest/facebook) and operator-supplied
// custom tags are rendered only when configured AND valid — an invalid or
// empty value is silently dropped from output (never rendered), while
// startup validation additionally logs the error so the operator notices.

import (
	"html"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/apimgr/pastebin/src/config"
)

// Provider verification-code format rules (AI.md 24501-24508).
var (
	seoVerifyGoogleRe    = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	seoVerifyBingRe      = regexp.MustCompile(`^[A-F0-9]+$`)
	seoVerifyYandexRe    = regexp.MustCompile(`^[a-f0-9]+$`)
	seoVerifyBaiduRe     = regexp.MustCompile(`^[a-zA-Z0-9]+$`)
	seoVerifyPinterestRe = regexp.MustCompile(`^[a-f0-9]+$`)
	seoVerifyFacebookRe  = regexp.MustCompile(`^[a-z0-9]+$`)
	// seoCustomNameRe validates a custom tag's name/property attribute value:
	// alphanumeric plus hyphens/underscores only (AI.md 24531). Custom tag
	// names/properties may contain colons (e.g. "fb:app_id"), so ':' is
	// permitted in addition to the spec's alphanumeric+hyphen/underscore set.
	seoCustomNameRe = regexp.MustCompile(`^[a-zA-Z0-9_:-]+$`)
)

type seoVerifyRule struct {
	label   string
	value   string
	metaKey string
	isProp  bool
	pattern *regexp.Regexp
	maxLen  int
}

// seoVerifyRules returns the six named-provider verification rules in
// rendering order (AI.md 24487-24496).
func seoVerifyRules(v config.SEOVerificationConfig) []seoVerifyRule {
	return []seoVerifyRule{
		{"google", v.Google, "google-site-verification", false, seoVerifyGoogleRe, 43},
		{"bing", v.Bing, "msvalidate.01", false, seoVerifyBingRe, 32},
		{"yandex", v.Yandex, "yandex-verification", false, seoVerifyYandexRe, 32},
		{"baidu", v.Baidu, "baidu-site-verification", false, seoVerifyBaiduRe, 32},
		{"pinterest", v.Pinterest, "p:domain_verify", false, seoVerifyPinterestRe, 32},
		{"facebook", v.Facebook, "fb:domain_verification", true, seoVerifyFacebookRe, 64},
	}
}

// ValidateSEOVerification checks every configured verification code and
// custom tag against PART 16's format/length rules and returns one error
// message per invalid entry. Called at startup so operators are warned about
// misconfigured codes (AI.md 24551 "Server validates codes on startup and
// logs errors for invalid formats") — validation failures are warnings only
// and never abort startup; the offending tag is simply never rendered.
func ValidateSEOVerification(v config.SEOVerificationConfig) []string {
	var errs []string
	for _, rule := range seoVerifyRules(v) {
		if rule.value == "" {
			continue
		}
		if len(rule.value) > rule.maxLen {
			errs = append(errs, "seo.verification."+rule.label+": exceeds max length of "+itoaSEO(rule.maxLen))
			continue
		}
		if !rule.pattern.MatchString(rule.value) {
			errs = append(errs, "seo.verification."+rule.label+": does not match required format")
		}
	}
	for i, c := range v.Custom {
		name := strings.TrimSpace(c.Name)
		prop := strings.TrimSpace(c.Property)
		if name == "" && prop == "" {
			errs = append(errs, "seo.verification.custom["+itoaSEO(i)+"]: requires name or property")
			continue
		}
		if name != "" && prop != "" {
			errs = append(errs, "seo.verification.custom["+itoaSEO(i)+"]: only one of name or property is allowed")
			continue
		}
		attr := name
		if attr == "" {
			attr = prop
		}
		if !seoCustomNameRe.MatchString(attr) {
			errs = append(errs, "seo.verification.custom["+itoaSEO(i)+"]: name/property must be alphanumeric, hyphens, or underscores only")
		}
		if strings.TrimSpace(c.Content) == "" {
			errs = append(errs, "seo.verification.custom["+itoaSEO(i)+"]: content is required")
		} else if len(c.Content) > 256 {
			errs = append(errs, "seo.verification.custom["+itoaSEO(i)+"]: content exceeds max length of 256")
		}
	}
	return errs
}

// logSEOVerificationErrors runs ValidateSEOVerification and logs each finding
// (startup-only warning path; never fatal — PART 16).
func logSEOVerificationErrors(v config.SEOVerificationConfig) {
	for _, msg := range ValidateSEOVerification(v) {
		log.Printf("config: invalid %s", msg)
	}
}

// itoaSEO avoids pulling in strconv just for small non-negative indices/lengths.
func itoaSEO(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// seoMetaTags renders the always-present SEO/OpenGraph/Twitter tags plus any
// valid, configured site-verification tags (AI.md 24433-24497) as a single
// template.HTML block. All dynamic values are HTML-escaped; verification
// codes/custom attribute names are further constrained by regex before
// being written into an attribute, so no unescaped operator input reaches
// the output.
func (s *Server) seoMetaTags(r *http.Request) template.HTML {
	cfg := s.liveCfg()
	b := cfg.Server.Branding
	seo := cfg.Server.SEO

	title := html.EscapeString(b.EffectiveTitle())
	description := html.EscapeString(b.EffectiveDescription())

	var sb strings.Builder
	if len(seo.Keywords) > 0 {
		sb.WriteString(`<meta name="keywords" content="`)
		sb.WriteString(html.EscapeString(strings.Join(seo.Keywords, ", ")))
		sb.WriteString("\">\n")
	}
	if a := strings.TrimSpace(seo.Author); a != "" {
		sb.WriteString(`<meta name="author" content="`)
		sb.WriteString(html.EscapeString(a))
		sb.WriteString("\">\n")
	}

	sb.WriteString(`<meta property="og:title" content="`)
	sb.WriteString(title)
	sb.WriteString("\">\n")
	sb.WriteString(`<meta property="og:description" content="`)
	sb.WriteString(description)
	sb.WriteString("\">\n")
	ogImage := strings.TrimSpace(seo.OGImage)
	if ogImage != "" {
		sb.WriteString(`<meta property="og:image" content="`)
		sb.WriteString(html.EscapeString(ogImage))
		sb.WriteString("\">\n")
	}
	sb.WriteString(`<meta property="og:type" content="website">` + "\n")
	sb.WriteString(`<meta property="og:url" content="`)
	sb.WriteString(html.EscapeString(s.baseURL(r) + r.URL.Path))
	sb.WriteString("\">\n")

	sb.WriteString(`<meta name="twitter:card" content="summary_large_image">` + "\n")
	sb.WriteString(`<meta name="twitter:title" content="`)
	sb.WriteString(title)
	sb.WriteString("\">\n")
	sb.WriteString(`<meta name="twitter:description" content="`)
	sb.WriteString(description)
	sb.WriteString("\">\n")
	if ogImage != "" {
		sb.WriteString(`<meta name="twitter:image" content="`)
		sb.WriteString(html.EscapeString(ogImage))
		sb.WriteString("\">\n")
	}
	if h := strings.TrimSpace(seo.TwitterHandle); h != "" {
		sb.WriteString(`<meta name="twitter:site" content="`)
		sb.WriteString(html.EscapeString(h))
		sb.WriteString("\">\n")
	}

	for _, rule := range seoVerifyRules(seo.Verification) {
		v := strings.TrimSpace(rule.value)
		if v == "" || len(v) > rule.maxLen || !rule.pattern.MatchString(v) {
			continue
		}
		if rule.isProp {
			sb.WriteString(`<meta property="`)
		} else {
			sb.WriteString(`<meta name="`)
		}
		sb.WriteString(rule.metaKey)
		sb.WriteString(`" content="`)
		sb.WriteString(html.EscapeString(v))
		sb.WriteString("\">\n")
	}

	for _, c := range seo.Verification.Custom {
		content := strings.TrimSpace(c.Content)
		if content == "" || len(content) > 256 {
			continue
		}
		name := strings.TrimSpace(c.Name)
		prop := strings.TrimSpace(c.Property)
		var attrKind, attrVal string
		switch {
		case name != "" && prop == "":
			attrKind, attrVal = "name", name
		case prop != "" && name == "":
			attrKind, attrVal = "property", prop
		default:
			continue
		}
		if !seoCustomNameRe.MatchString(attrVal) {
			continue
		}
		sb.WriteString(`<meta `)
		sb.WriteString(attrKind)
		sb.WriteString(`="`)
		sb.WriteString(attrVal)
		sb.WriteString(`" content="`)
		sb.WriteString(html.EscapeString(content))
		sb.WriteString("\">\n")
	}

	return template.HTML(sb.String())
}
