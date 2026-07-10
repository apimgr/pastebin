package config

import (
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for the pastebin server.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Paste     PasteConfig     `yaml:"paste"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Web       WebConfig       `yaml:"web"`
}

// ServerConfig holds listener and runtime settings.
type ServerConfig struct {
	Port    string `yaml:"port"`
	Address string `yaml:"address"`
	FQDN    string `yaml:"fqdn"`
	Mode    string `yaml:"mode"`
	// APIVersion is the API route prefix segment (default v1): /api/{api_version}/.
	APIVersion string `yaml:"api_version"`
	BaseURL    string `yaml:"base_url"` // override for URL generation
	// Branding holds the site title/tagline/description (PART 12/16).
	Branding BrandingConfig `yaml:"branding"`
	// SEO holds search-engine metadata such as keywords (PART 12/16).
	SEO SEOConfig `yaml:"seo"`
	// User and Group are the system user/group to drop privileges to (PART 12).
	// Empty or "{auto}" means auto-detect; keep current privileges if unset.
	User  string `yaml:"user"`
	Group string `yaml:"group"`
	// PIDFile enables writing a PID file on start (PART 12).
	PIDFile bool `yaml:"pidfile"`
	// Daemonize detaches from the terminal on start (PART 12). Default false.
	Daemonize bool `yaml:"daemonize"`
	// Scheduler configures built-in background tasks (PART 12/18).
	Scheduler SchedulerConfig `yaml:"scheduler"`
	// Token is the operator token (server.token). Auto-generated on first run.
	// All operator-protected API endpoints require: Authorization: Bearer <token>
	Token string `yaml:"token"`
	// DataDir is the runtime data directory. Resolved at startup by main from paths.GetDataDir.
	// Used by middleware (blocklist) and tasks that need access to security databases.
	DataDir       string              `yaml:"data_dir"`
	Metrics       MetricsConfig       `yaml:"metrics"`
	GeoIP         GeoIPConfig         `yaml:"geoip"`
	Tor           TorConfig           `yaml:"tor"`
	Logging       LoggingConfig       `yaml:"logging"`
	Notifications NotificationsConfig `yaml:"notifications"`
	TLS           TLSConfig           `yaml:"tls"`
	// Backup configures backup encryption, compliance, and retention (PART 21).
	Backup BackupConfig `yaml:"backup"`
	// Update controls the self-update channel and scheduling behaviour (PART 22).
	Update UpdateConfig `yaml:"update"`
	// Cache configures the in-process or remote cache driver (PART 9/12).
	Cache CacheConfig `yaml:"cache"`
	// Limits controls HTTP server request and body limits (PART 12).
	Limits LimitsConfig `yaml:"limits"`
	// TrustedProxies lists additional proxy IPs/CIDRs beyond the private ranges
	// that are always trusted (loopback, RFC 1918, etc.) (PART 12).
	TrustedProxies TrustedProxiesConfig `yaml:"trusted_proxies"`
	// URLDetection controls the domain-learning subsystem (PART 12).
	URLDetection URLDetectionConfig `yaml:"url_detection"`
	// Termbin configures the raw-TCP termbin/fiche compatibility listener.
	Termbin TermbinConfig `yaml:"termbin"`
	// Maintenance configures the runtime self-healing maintenance mode (PART 20).
	Maintenance MaintenanceConfig `yaml:"maintenance"`
	// Contact holds the per-role notification recipients (PART 12): admin
	// (server-internal alerts, never public), security (vulnerability reports,
	// surfaced in security.txt), and general (public /server/contact form).
	Contact ContactConfig `yaml:"contact"`
	// Pages holds operator-customizable content and settings for the static
	// /server pages (about, privacy, contact, help, terms) (PART 31).
	Pages PagesConfig `yaml:"pages"`
	// Privacy holds server-wide privacy, cookie-consent, data-handling, and
	// third-party disclosure settings driving the /server/privacy page and the
	// site-wide cookie-consent banner (PART 31).
	Privacy PrivacyConfig `yaml:"privacy"`
	// Tracking configures the server-wide analytics platform (PART 31). Disabled
	// by default; when set it drives the tracking script and third-party
	// auto-population on the privacy page.
	Tracking TrackingConfig `yaml:"tracking"`
}

// ContactConfig is the unified notification-recipient tree (PART 12). Each role
// carries an email address plus any number of named webhook transports. Empty
// role-specific values fall back to the admin role. The only knob an operator
// must set is server.contact.admin.email; everything else is optional.
type ContactConfig struct {
	// Admin receives server-internal alerts (error spikes, cert/backup failures,
	// panics). NEVER public. Universal fallback for empty role addresses.
	Admin ContactRole `yaml:"admin"`
	// Security receives vulnerability reports. Public — surfaced in security.txt's
	// Contact: line and the PGP keypair UID. Empty → falls back to admin.
	Security ContactRole `yaml:"security"`
	// Abuse receives abuse reports (spam, harassment, illegal content, DMCA /
	// takedown). Public — shown in the contact page "Abuse Reports" section when
	// set. Empty → delivery falls back to general, then admin; never
	// auto-advertises abuse@{fqdn} (an unprovisioned mailbox would bounce).
	Abuse ContactRole `yaml:"abuse"`
	// General receives /server/contact form submissions. Public — shown as the
	// footer "Contact us" address. Empty → falls back to admin.
	General ContactRole `yaml:"general"`
}

// ContactRole is a single notification recipient: an email address and a map of
// named webhook transports (telegram, discord, slack, generic, ...) (PART 12).
type ContactRole struct {
	Email    string            `yaml:"email"`
	Webhooks map[string]string `yaml:"webhooks"`
}

// PagesConfig holds operator-customizable content for the static /server pages
// (PART 31). Empty content fields fall back to the built-in default templates.
type PagesConfig struct {
	About   PageContentConfig `yaml:"about"`
	Privacy PageContentConfig `yaml:"privacy"`
	Contact ContactPageConfig `yaml:"contact"`
	Help    PageContentConfig `yaml:"help"`
	Terms   PageContentConfig `yaml:"terms"`
}

// PageContentConfig is a single static page's optional Markdown content override.
type PageContentConfig struct {
	Content string `yaml:"content"`
}

// ContactPageConfig configures the /server/contact form (PART 31). When Enabled
// is false the form is hidden and only static contact details are shown.
type ContactPageConfig struct {
	// Enabled turns the contact form on. Default true.
	Enabled bool `yaml:"enabled"`
	// Captcha selects the spam-prevention challenge: "simple" (built-in),
	// "recaptcha", or "hcaptcha". Default "simple".
	Captcha string `yaml:"captcha"`
	// SuccessMessage is shown after a successful submission.
	SuccessMessage string `yaml:"success_message"`
}

// PrivacyConfig holds server-wide privacy, cookie-consent, and data-handling
// settings (PART 31). Messaging adapts dynamically to Data.Sold: when data is
// sold the consent banner, analytics description, and data-usage content switch
// to their "may be sold" variants and a CCPA "Do Not Sell" opt-out is surfaced.
type PrivacyConfig struct {
	Data       DataPolicy       `yaml:"data"`
	Retention  RetentionPolicy  `yaml:"retention"`
	Consent    ConsentConfig    `yaml:"consent"`
	Cookies    CookieCategories `yaml:"cookies"`
	ThirdParty ThirdPartyConfig `yaml:"third_party"`
	Content    PrivacyContent   `yaml:"content"`
}

// DataPolicy controls data-handling disclosure and CCPA applicability (PART 31).
type DataPolicy struct {
	// Sold reports whether user data is sold to third parties. Default false.
	// The MIT license permits downstream operators to set this true.
	Sold bool `yaml:"sold"`
	// StoredOnServer reports data lives on this server, not third-party cloud.
	StoredOnServer bool `yaml:"stored_on_server"`
	// Sharing enumerates the conditions under which data may be shared.
	Sharing []SharingCondition `yaml:"sharing"`
}

// SharingCondition describes one circumstance under which data may be shared.
type SharingCondition struct {
	Condition string `yaml:"condition"`
	When      string `yaml:"when"`
	Data      string `yaml:"data"`
}

// RetentionPolicy describes how long data is kept and user data rights (PART 31).
type RetentionPolicy struct {
	Period            string `yaml:"period"`
	ExportAvailable   bool   `yaml:"export_available"`
	DeletionAvailable bool   `yaml:"deletion_available"`
}

// ConsentConfig configures the site-wide cookie-consent banner (PART 31).
type ConsentConfig struct {
	// ShowUntilAcknowledged keeps the banner on every frontend page until the
	// visitor accepts or declines.
	ShowUntilAcknowledged bool `yaml:"show_until_acknowledged"`
	// DefaultEnabled selects the opt-out model: non-essential cookies default on.
	DefaultEnabled bool `yaml:"default_enabled"`
	// Message is shown when Data.Sold is false.
	Message string `yaml:"message"`
	// MessageIfSold is shown when Data.Sold is true.
	MessageIfSold string            `yaml:"message_if_sold"`
	Policy        ConsentPolicyLink `yaml:"policy"`
	Buttons       ConsentButtons    `yaml:"buttons"`
	// Position places the banner: "bottom" (default) or "top".
	Position string `yaml:"position"`
	// ShowPreferences shows the granular "Manage Preferences" link.
	ShowPreferences bool `yaml:"show_preferences"`
	// PreferencesText is the label for the "Manage Preferences" link.
	PreferencesText string `yaml:"preferences_text"`
}

// ConsentPolicyLink is the privacy-policy link shown in the consent banner.
type ConsentPolicyLink struct {
	Text string `yaml:"text"`
	URL  string `yaml:"url"`
}

// ConsentButtons holds the accept/decline button labels for the banner.
type ConsentButtons struct {
	Decline string `yaml:"decline"`
	Accept  string `yaml:"accept"`
}

// CookieCategories groups the three cookie tiers shown in the preferences UI.
type CookieCategories struct {
	Essential   CookieCategory  `yaml:"essential"`
	Preferences CookieCategory  `yaml:"preferences"`
	Analytics   AnalyticsCookie `yaml:"analytics"`
}

// CookieCategory is a single cookie tier: whether it is on and its description.
type CookieCategory struct {
	Enabled     bool   `yaml:"enabled"`
	Description string `yaml:"description"`
}

// AnalyticsCookie extends CookieCategory with sold/not-sold description suffixes.
type AnalyticsCookie struct {
	CookieCategory           `yaml:",inline"`
	DescriptionSuffixNotSold string `yaml:"description_suffix_not_sold"`
	DescriptionSuffixSold    string `yaml:"description_suffix_sold"`
}

// ThirdPartyConfig lists third-party services that receive data. Analytics
// entries are auto-populated from server.tracking; operators may add more.
type ThirdPartyConfig struct {
	Services []ThirdPartyService `yaml:"services"`
}

// ThirdPartyService is a single third-party recipient shown on the privacy page.
type ThirdPartyService struct {
	Name      string `yaml:"name"`
	Purpose   string `yaml:"purpose"`
	DataSent  string `yaml:"data_sent"`
	PolicyURL string `yaml:"policy_url"`
}

// PrivacyContent holds the Markdown body blocks rendered on the privacy page.
type PrivacyContent struct {
	DataCollection string `yaml:"data_collection"`
	// DataUsage is used when Data.Sold is false.
	DataUsage string `yaml:"data_usage"`
	// DataUsageIfSold is used when Data.Sold is true.
	DataUsageIfSold string `yaml:"data_usage_if_sold"`
	DataSecurity    string `yaml:"data_security"`
}

// TrackingConfig configures the server-wide analytics platform (PART 31).
type TrackingConfig struct {
	// Type selects the analytics platform; empty or "none" disables tracking.
	Type string `yaml:"type"`
	// ID is the platform tracking/site identifier (format depends on Type).
	ID string `yaml:"id"`
	// URL is the self-hosted instance URL (required for matomo/piwik/owa/umami).
	URL string `yaml:"url"`
}

// GetConsentMessage returns the banner message matching the data-sold setting.
func (p *PrivacyConfig) GetConsentMessage() string {
	if p.Data.Sold {
		return p.Consent.MessageIfSold
	}
	return p.Consent.Message
}

// GetAnalyticsDescription returns the analytics cookie description with the
// sold/not-sold suffix appended.
func (p *PrivacyConfig) GetAnalyticsDescription() string {
	base := p.Cookies.Analytics.Description
	if p.Data.Sold {
		return strings.TrimSpace(base + " " + p.Cookies.Analytics.DescriptionSuffixSold)
	}
	return strings.TrimSpace(base + " " + p.Cookies.Analytics.DescriptionSuffixNotSold)
}

// GetDataUsageContent returns the data-usage Markdown matching the sold setting.
func (p *PrivacyConfig) GetDataUsageContent() string {
	if p.Data.Sold {
		return p.Content.DataUsageIfSold
	}
	return p.Content.DataUsage
}

// IsCCPAApplicable reports whether the CCPA "Do Not Sell" opt-out must be shown.
func (p *PrivacyConfig) IsCCPAApplicable() bool {
	return p.Data.Sold
}

// trackingTypeNames maps analytics type keys to human-friendly platform names.
var trackingTypeNames = map[string]string{
	"google":     "Google Analytics",
	"matomo":     "Matomo",
	"piwik":      "Piwik",
	"owa":        "Open Web Analytics",
	"fathom":     "Fathom Analytics",
	"plausible":  "Plausible Analytics",
	"umami":      "Umami",
	"simple":     "Simple Analytics",
	"cloudflare": "Cloudflare Web Analytics",
}

// trackingPolicyURLs maps analytics types to their public privacy-policy URLs.
var trackingPolicyURLs = map[string]string{
	"google":     "https://policies.google.com/privacy",
	"matomo":     "https://matomo.org/privacy-policy/",
	"piwik":      "https://matomo.org/privacy-policy/",
	"fathom":     "https://usefathom.com/privacy",
	"plausible":  "https://plausible.io/privacy",
	"umami":      "https://umami.is/privacy",
	"simple":     "https://simpleanalytics.com/privacy",
	"cloudflare": "https://www.cloudflare.com/privacypolicy/",
}

// Enabled reports whether an analytics platform is configured.
func (t TrackingConfig) Enabled() bool {
	return t.Type != "" && t.Type != "none"
}

// TypeName returns the human-friendly platform name for the configured type.
func (t TrackingConfig) TypeName() string {
	if n, ok := trackingTypeNames[t.Type]; ok {
		return n
	}
	return t.Type
}

// analyticsService builds the third-party service entry for the configured
// analytics platform, or false when tracking is disabled (PART 31).
func (t TrackingConfig) analyticsService() (ThirdPartyService, bool) {
	if !t.Enabled() {
		return ThirdPartyService{}, false
	}
	return ThirdPartyService{
		Name:      t.TypeName(),
		Purpose:   "Usage analytics",
		DataSent:  "Page views, browser type, country (anonymized IP)",
		PolicyURL: trackingPolicyURLs[t.Type],
	}, true
}

// EffectiveThirdParty returns the configured third-party services with the
// analytics platform auto-prepended when server.tracking is enabled and not
// already present (PART 31).
func (c *Config) EffectiveThirdParty() []ThirdPartyService {
	services := append([]ThirdPartyService(nil), c.Server.Privacy.ThirdParty.Services...)
	svc, ok := c.Server.Tracking.analyticsService()
	if !ok {
		return services
	}
	for _, s := range services {
		if s.Name == svc.Name {
			return services
		}
	}
	return append([]ThirdPartyService{svc}, services...)
}

// MaintenanceConfig configures the runtime self-healing maintenance mode (PART 20).
// On a critical error (database connection loss or file-write failure) the server
// enters maintenance mode, rejects write operations with HTTP 503, and continuously
// attempts self-healing until the issue clears.
type MaintenanceConfig struct {
	// SelfHealing controls the background retry/recovery loop.
	SelfHealing MaintenanceSelfHealingConfig `yaml:"self_healing"`
	// Cleanup controls resource reclamation attempted during disk-related healing.
	Cleanup MaintenanceCleanupConfig `yaml:"cleanup"`
	// Notify controls maintenance enter/exit email notifications.
	Notify MaintenanceNotifyConfig `yaml:"notify"`
}

// MaintenanceSelfHealingConfig controls the self-healing retry loop (PART 20).
type MaintenanceSelfHealingConfig struct {
	// Enabled gates the background retry loop. Default true.
	Enabled bool `yaml:"enabled"`
	// RetryInterval is the delay between self-healing attempts. Default "30s".
	RetryInterval string `yaml:"retry_interval"`
	// MaxAttempts caps retries; 0 means unlimited (keep trying forever). Default 0.
	MaxAttempts int `yaml:"max_attempts"`
}

// MaintenanceCleanupConfig controls resource reclamation during healing (PART 20).
type MaintenanceCleanupConfig struct {
	// DiskThreshold is the disk-usage percentage that triggers cleanup. Default 90.
	DiskThreshold int `yaml:"disk_threshold"`
	// LogRetentionDays is how many days of logs to keep during cleanup. Default 7.
	LogRetentionDays int `yaml:"log_retention_days"`
	// BackupKeepCount is how many recent backups to retain during cleanup. Default 5.
	BackupKeepCount int `yaml:"backup_keep_count"`
}

// MaintenanceNotifyConfig controls maintenance enter/exit notifications (PART 20).
type MaintenanceNotifyConfig struct {
	// OnEnter sends a notification when entering maintenance mode. Default true.
	OnEnter bool `yaml:"on_enter"`
	// OnExit sends a notification when exiting maintenance mode. Default true.
	OnExit bool `yaml:"on_exit"`
}

// TermbinConfig configures the raw-TCP termbin/fiche-protocol listener.
// Clients connect, stream content, half-close, and receive a single
// "{base}/{id}\n" URL line. Disabled by default; opt-in per operator.
type TermbinConfig struct {
	// Enabled turns the raw-TCP listener on. Default false.
	Enabled bool `yaml:"enabled"`
	// Port is the TCP port to listen on. termbin/fiche default is 9999.
	Port int `yaml:"port"`
	// MaxSize is the maximum bytes accepted per connection. Default 32768.
	MaxSize int64 `yaml:"max_size"`
	// Timeout is the read deadline for a single upload. Default "5s".
	Timeout string `yaml:"timeout"`
}

// LimitsConfig controls HTTP server timeouts and body size limits (PART 12).
type LimitsConfig struct {
	// MaxBodySize is the maximum request body in bytes. Accepts "10MB", "1MiB", or plain integer.
	MaxBodySize  int64  `yaml:"max_body_size"`
	ReadTimeout  string `yaml:"read_timeout"`  // e.g. "30s"
	WriteTimeout string `yaml:"write_timeout"` // e.g. "30s"
	IdleTimeout  string `yaml:"idle_timeout"`  // e.g. "120s"
}

// TrustedProxiesConfig lists additional proxy IPs/CIDRs/DNS names beyond the
// private ranges that are always trusted (PART 12).
// Private ranges (127/8, 10/8, 172.16/12, 192.168/16, fc00::/7, etc.) are
// always trusted without config. Add public proxy IPs here only.
type TrustedProxiesConfig struct {
	Additional []string `yaml:"additional"`
}

// URLDetectionConfig controls the domain-learning subsystem (PART 12).
// When enabled, the server observes incoming Host headers, infers the base
// domain (eTLD+1) and wildcard domain, and uses them for CORS and URL building.
type URLDetectionConfig struct {
	// Learning enables domain learning. Default true.
	Learning bool `yaml:"learning"`
	// MinSamples is the number of observations of a hostname within SampleWindow
	// before it is promoted to baseDomain / wildcardDomain.
	MinSamples int `yaml:"min_samples"`
	// SampleWindow is the rolling time window for MinSamples observations.
	SampleWindow time.Duration `yaml:"sample_window"`
	// LogChanges emits a log line whenever baseDomain or wildcardDomain changes.
	LogChanges bool `yaml:"log_changes"`
	// LiveReload causes the CORS header to update immediately when a new domain is
	// learned, without restarting the server.
	LiveReload bool `yaml:"live_reload"`
}

// CacheConfig selects and configures the cache driver (PART 9/12).
// Defaults to "memory" (in-process, lost on restart).
// Valkey/Redis is recommended for production to persist counters across restarts.
type CacheConfig struct {
	// Type selects the driver: "none", "memory" (default), "valkey", "redis".
	Type string `yaml:"type"`
	// URL is the connection string. Takes precedence over individual fields.
	// Format: redis://user:pass@host:port/db  or  valkey://...
	URL      string `yaml:"url"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	// TLS enables TLS for the connection.
	TLS           bool `yaml:"tls"`
	TLSSkipVerify bool `yaml:"tls_skip_verify"`
	PoolSize      int  `yaml:"pool_size"`
	MinIdle       int  `yaml:"min_idle"`
	// Timeout is the dial/read/write timeout for remote drivers.
	Timeout string `yaml:"timeout"` // e.g. "5s"
	// Prefix is prepended to every key to avoid namespace collisions.
	Prefix string `yaml:"prefix"`
	// TTL is the default time-to-live. e.g. "1h", "30m".
	TTL string `yaml:"ttl"`
}

// TorConfig configures the Tor hidden service and optional outbound network.
type TorConfig struct {
	// Binary path; empty = auto-detect in PATH and common locations.
	Binary string `yaml:"binary"`

	// Outbound network settings.
	UseNetwork          bool `yaml:"use_network"`
	AllowUserPreference bool `yaml:"allow_user_preference"`

	// Performance.
	MaxCircuits      int `yaml:"max_circuits"`
	CircuitTimeout   int `yaml:"circuit_timeout"`
	BootstrapTimeout int `yaml:"bootstrap_timeout"`

	// Security.
	SafeLogging               bool `yaml:"safe_logging"`
	MaxStreamsPerCircuit      int  `yaml:"max_streams_per_circuit"`
	CloseCircuitOnStreamLimit bool `yaml:"close_circuit_on_stream_limit"`

	// Bandwidth.
	BandwidthRate       string `yaml:"bandwidth_rate"`
	BandwidthBurst      string `yaml:"bandwidth_burst"`
	MaxMonthlyBandwidth string `yaml:"max_monthly_bandwidth"`

	// Hidden service.
	NumIntroPoints int `yaml:"num_intro_points"`
	VirtualPort    int `yaml:"virtual_port"`

	// OnionAddress is the v3 .onion address (56 chars) assigned to this hidden service.
	// Set automatically on first successful Tor bootstrap; may be overridden to use a
	// pre-generated key. Used by baseURL() priority-0 check (PART 12) and Tor privacy
	// rules — clearnet contact email is NEVER shown when Host matches this address.
	OnionAddress string `yaml:"onion_address"`

	// ContactEmail is the Tor-specific operator contact address.
	// Shown on Tor responses instead of any clearnet contact email (PART 12/31).
	// When empty, no contact email is disclosed on Tor responses.
	ContactEmail string `yaml:"contact_email"`
}

// GeoIPConfig configures GeoIP detection and country blocking.
type GeoIPConfig struct {
	Enabled        bool                 `yaml:"enabled"`
	Dir            string               `yaml:"dir"` // path to MMDB database directory
	DenyCountries  []string             `yaml:"deny_countries"`
	AllowCountries []string             `yaml:"allow_countries"`
	Databases      GeoIPDatabasesConfig `yaml:"databases"`
}

// GeoIPDatabasesConfig controls which MMDB files to download and use.
type GeoIPDatabasesConfig struct {
	ASN     bool `yaml:"asn"`
	Country bool `yaml:"country"`
	City    bool `yaml:"city"`
	WHOIS   bool `yaml:"whois"`
}

// MetricsConfig configures the /metrics endpoint.
type MetricsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
	// IncludeSystem exposes system_* gauges (CPU, memory, disk) when true.
	IncludeSystem bool `yaml:"include_system"`
	// IncludeRuntime exposes pastebin_go_* runtime gauges when true.
	IncludeRuntime bool   `yaml:"include_runtime"`
	Token          string `yaml:"token"`
	// DurationBuckets overrides the http_request_duration_seconds histogram buckets.
	DurationBuckets []float64 `yaml:"duration_buckets"`
	// SizeBuckets overrides the http_request_size_bytes / http_response_size_bytes buckets.
	SizeBuckets []float64 `yaml:"size_buckets"`
	// AllowedIPs lists additional IPs or CIDRs that may reach /metrics (PART 20).
	// Loopback (127.0.0.1, ::1) is always allowed. Non-loopback requests from IPs
	// not in this list are rejected with 403 before the bearer-token check.
	AllowedIPs []string `yaml:"allowed_ips"`
}

// LoggingConfig controls access log format and log level.
type LoggingConfig struct {
	AccessFormat string      `yaml:"access_format"`
	Level        string      `yaml:"level"`
	Audit        AuditConfig `yaml:"audit"`
}

// AuditConfig controls the JSON Lines audit log (AI.md `server.logs.audit`).
// The audit log records configuration, security, backup, and server-lifecycle
// events as one JSON object per line.
type AuditConfig struct {
	Enabled          bool                 `yaml:"enabled"`
	Filename         string               `yaml:"filename"`
	Format           string               `yaml:"format"`
	Rotate           string               `yaml:"rotate"`
	Keep             string               `yaml:"keep"`
	Compress         bool                 `yaml:"compress"`
	IncludeUserAgent bool                 `yaml:"include_user_agent"`
	MaskEmails       bool                 `yaml:"mask_emails"`
	TokenUsage       bool                 `yaml:"token_usage"`
	Events           AuditEventCategories `yaml:"events"`
}

// AuditEventCategories toggles which categories of events are recorded.
type AuditEventCategories struct {
	Configuration bool `yaml:"configuration"`
	Security      bool `yaml:"security"`
	Backup        bool `yaml:"backup"`
	Server        bool `yaml:"server"`
}

// DatabaseConfig selects and configures the storage backend.
type DatabaseConfig struct {
	Type string `yaml:"type"` // only "sqlite" for now
	Path string `yaml:"path"` // path to the SQLite database file
}

// PasteConfig controls paste-specific behaviour.
type PasteConfig struct {
	MaxSizeBytes    int64  `yaml:"max_size_bytes"`   // max paste size (default 10 MiB)
	DefaultExpiry   string `yaml:"default_expiry"`   // "never" or expiry code
	DefaultLanguage string `yaml:"default_language"` // "text"
	MaxBurnAfter    int    `yaml:"max_burn_after"`   // cap on burn_after (default 9999)
	AllowUnlisted   bool   `yaml:"allow_unlisted"`   // allow unlisted pastes (default true)
}

// RateLimitEndpoint configures the per-IP request quota for one endpoint class (PART 12).
type RateLimitEndpoint struct {
	// Requests is the maximum number of requests allowed per IP within Window seconds.
	Requests int `yaml:"requests"`
	// Window is the sliding window in seconds.
	Window int `yaml:"window"`
}

// RateLimitConfig controls request throttling (PART 12).
// Endpoint classes: Read (GET/HEAD), Write (POST/PUT/PATCH/DELETE), Health (status endpoints).
// GlobalBurst is the absolute ceiling across all classes combined.
type RateLimitConfig struct {
	Enabled     bool              `yaml:"enabled"`
	Read        RateLimitEndpoint `yaml:"read"`
	Write       RateLimitEndpoint `yaml:"write"`
	Health      RateLimitEndpoint `yaml:"health"`
	GlobalBurst int               `yaml:"global_burst"`
}

// WebConfig holds web-UI settings.
type WebConfig struct {
	SiteTitle string         `yaml:"site_title"`
	Theme     string         `yaml:"theme"` // "dark" | "light" | "auto"
	Robots    RobotsConfig   `yaml:"robots"`
	Security  SecurityConfig `yaml:"security"`
	HSTS      HSTSConfig     `yaml:"hsts"`
	CSP       CSPConfig      `yaml:"csp"`
	// Headers controls which advanced security headers are emitted.
	Headers HeadersConfig `yaml:"headers"`
	// CSRF controls double-submit cookie CSRF protection (PART 16).
	CSRF CSRFConfig `yaml:"csrf"`
	// Healthz controls the optional root-level /healthz alias.
	Healthz HealthzConfig `yaml:"healthz"`
	// Footer controls operator footer branding (PART 16).
	Footer FooterConfig `yaml:"footer"`
}

// FooterConfig holds operator footer branding shown above the default
// application footer (PART 16 Footer Customization).
type FooterConfig struct {
	// CustomHTML is operator-supplied branding HTML rendered above the default
	// footer. It is sanitized before rendering — scripts, event handlers, and
	// dangerous elements are stripped. An empty value uses the default branding;
	// a single space (" ") disables branding and shows only the default footer.
	CustomHTML string `yaml:"custom_html"`
}

// HealthzConfig controls the optional /healthz root alias (PART 13).
// The canonical route is always /server/healthz; this alias is opt-in only.
type HealthzConfig struct {
	// Root.Enabled mounts /healthz → /server/healthz when true.
	// Default: false. Only enable for tooling that requires a root-level probe.
	Root struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"root"`
}

// HeadersConfig controls which advanced security headers the server emits (PART 11).
type HeadersConfig struct {
	// SecFetchValidation rejects cross-site state-changing requests when
	// Sec-Fetch-Site: cross-site is present and no Bearer token is provided.
	// Absence of the header is treated as pass-through for legacy-browser compat.
	SecFetchValidation bool `yaml:"sec_fetch_validation"`
}

// CSRFConfig controls CSRF double-submit cookie protection (PART 16).
// CSRF only applies to cookie-authenticated browser forms; Bearer/API-token
// requests and public endpoints are always exempt.
type CSRFConfig struct {
	Enabled bool `yaml:"enabled"`
	// TokenLength is the CSRF token size in bytes. Default: 32.
	TokenLength int    `yaml:"token_length"`
	CookieName  string `yaml:"cookie_name"`
	HeaderName  string `yaml:"header_name"`
	// Secure controls the Secure cookie flag: "auto" | "true" | "false".
	// "auto" sets Secure when the request was received over HTTPS.
	Secure string `yaml:"secure"`
	// ExemptPaths lists paths that bypass CSRF validation.
	// Glob patterns are supported (e.g., /api/v1/webhooks/*).
	ExemptPaths []string `yaml:"exempt_paths"`
}

// NotificationsConfig holds email notification settings.
type NotificationsConfig struct {
	Email EmailConfig `yaml:"email"`
}

// EmailConfig holds SMTP and sender settings for outbound email.
type EmailConfig struct {
	Enabled     bool              `yaml:"enabled"`
	SMTP        SMTPConfig        `yaml:"smtp"`
	From        EmailFrom         `yaml:"from"`
	ReplyTo     string            `yaml:"reply_to"`
	TemplateDir string            `yaml:"template_dir"` // custom override dir; empty = use embedded defaults
	Events      EmailEventsConfig `yaml:"events"`
}

// EmailEventsConfig controls which operator events trigger an email (AI.md:26587-26598).
// All fields default to false except the high-signal events that are true by default.
type EmailEventsConfig struct {
	Startup         bool `yaml:"startup"`
	Shutdown        bool `yaml:"shutdown"`
	BackupComplete  bool `yaml:"backup_complete"`
	BackupFailed    bool `yaml:"backup_failed"`
	SSLExpiring     bool `yaml:"ssl_expiring"`
	SSLRenewed      bool `yaml:"ssl_renewed"`
	SecurityAlert   bool `yaml:"security_alert"`
	SchedulerError  bool `yaml:"scheduler_error"`
	UpdateAvailable bool `yaml:"update_available"`
	UpdateInstalled bool `yaml:"update_installed"`
}

// SMTPConfig holds connection settings for the outbound SMTP server.
type SMTPConfig struct {
	// Host is the SMTP server hostname or IP. Empty = auto-detect on first run.
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"` // default 587
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// TLS controls connection security: "auto", "starttls", "tls", "none"
	TLS string `yaml:"tls"`
}

// EmailFrom holds sender identity fields.
type EmailFrom struct {
	// Name defaults to the site title when empty.
	Name string `yaml:"name"`
	// Email defaults to no-reply@{fqdn} when empty.
	Email string `yaml:"email"`
}

// RobotsConfig sets robots.txt allow/deny lists.
type RobotsConfig struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// SecurityConfig holds security.txt metadata and server-wide encryption keys.
// The security contact recipient is NOT stored here — it is the canonical
// server.contact.security.email (PART 12 → "Canonical Contact Keys Only").
type SecurityConfig struct {
	CORS string `yaml:"cors"`
	// ReportURL is the primary security.txt Contact line — the repository's
	// GitHub private vulnerability reporting URL, listed FIRST per RFC 9116
	// preference order (PART 11). Empty → derived from the repo owner/name as
	// https://github.com/{org}/{name}/security/advisories/new.
	ReportURL string `yaml:"report_url"`
	// Expires is the security.txt Expires field. Empty → auto-calculated one
	// year from the time the file is served (RFC 9116 requires this field).
	Expires string `yaml:"expires"`
	// PreferredLanguages is the security.txt Preferred-Languages field: a
	// comma-separated list of RFC 5646 language tags. Empty → "en".
	PreferredLanguages string `yaml:"preferred_languages"`
	// EncryptionKey is the 32-byte AES-256-GCM key (hex-encoded) used for at-rest
	// encryption of sensitive server data (DNS credentials, security reports, etc.).
	// Auto-generated on first run; stored in server.yml; included in every backup.
	EncryptionKey string `yaml:"encryption_key"`
	// ContactEmail is the operator recipient for coordinated-disclosure security
	// reports (PART 11 → Coordinated Disclosure Pipeline). Empty → falls back to
	// the canonical server.contact.security.email.
	ContactEmail string `yaml:"contact_email"`
	// Keyservers is the list of PGP keyserver submission endpoints the project's
	// public key is published to on generate/rotate (PART 11).
	Keyservers []string `yaml:"keyservers"`
	// PublishPGPKey enables serving /.well-known/pgp-key.asc, the Encryption line
	// in security.txt, and keyserver publishing. Parsed via config.ParseBool.
	PublishPGPKey string `yaml:"publish_pgp_key"`
	// Allowlist is a list of IP addresses or CIDR ranges that bypass blocklist,
	// rate limiting, and geoip checks. Auth checks are never bypassed.
	// Single IPs are automatically expanded to /32 (IPv4) or /128 (IPv6).
	Allowlist []string `yaml:"allowlist"`
	// Blocklists configures the IP/domain blocklist sources downloaded daily by
	// the scheduler into {data_dir}/security/blocklists/ (PART 18/19).
	Blocklists BlocklistsConfig `yaml:"blocklists"`
	// CVE configures the CVE database source downloaded daily by the scheduler
	// into {data_dir}/security/cve/ (PART 18/19).
	CVE CVEConfig `yaml:"cve"`
}

// BlocklistSource is a single downloadable blocklist: File is the destination
// filename within the blocklists directory; URL is the download source.
type BlocklistSource struct {
	File string `yaml:"file"`
	URL  string `yaml:"url"`
}

// BlocklistsConfig holds the configurable blocklist download sources.
type BlocklistsConfig struct {
	Enabled bool              `yaml:"enabled"`
	Sources []BlocklistSource `yaml:"sources"`
}

// CVEConfig holds the configurable CVE database download settings.
type CVEConfig struct {
	Enabled bool `yaml:"enabled"`
	// File is the destination filename within the cve directory.
	File string `yaml:"file"`
	// Source is the download URL for the CVE feed.
	Source string `yaml:"source"`
}

// TLSConfig holds SSL/TLS and Let's Encrypt settings (PART 12/15).
// The yaml key is `tls` per PART 15: "TLS config: server.tls.* keys in config".
type TLSConfig struct {
	Enabled bool `yaml:"enabled"`
	// Cert and Key are optional manual certificate override paths.
	// Leave empty for auto-detection (certbot dirs, app-managed, self-signed).
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
	// MinVersion is the minimum negotiated TLS version: TLS1.2 or TLS1.3.
	MinVersion string `yaml:"min_version"`
	// LetsEncrypt holds ACME settings, nested under ssl.letsencrypt per spec.
	LetsEncrypt LetsEncryptConfig `yaml:"letsencrypt"`
	// DNSProvider selects the dns-01 provider name (e.g. cloudflare, rfc2136).
	DNSProvider string `yaml:"dns_provider"`
	// DNSCredentialsEncrypted stores AES-256-GCM encrypted JSON of provider credentials.
	// Encrypted using web.security.encryption_key. Never store plaintext credentials here.
	DNSCredentialsEncrypted string `yaml:"dns_credentials_encrypted"`
}

// LetsEncryptConfig holds ACME settings nested under server.tls.letsencrypt (PART 12/15).
type LetsEncryptConfig struct {
	Enabled bool   `yaml:"enabled"`
	Email   string `yaml:"email"`
	// Challenge selects the ACME challenge type: http-01, tls-alpn-01, dns-01.
	Challenge string `yaml:"challenge"`
	// Staging uses the LE staging CA for testing.
	Staging bool `yaml:"staging"`
}

// BrandingConfig holds site branding shown in the UI and metadata (PART 12/16).
// Tagline, Description, Features, and Links source the /server/about page content
// (PART 16 page-content sourcing); each has a real default drawn from IDEA.md so
// the About page is never blank or generic.
type BrandingConfig struct {
	Title       string         `yaml:"title"`
	Tagline     string         `yaml:"tagline"`
	Description string         `yaml:"description"`
	Features    []string       `yaml:"features"`
	Links       []BrandingLink `yaml:"links"`
}

// BrandingLink is a single labeled hyperlink shown on the /server/about page.
type BrandingLink struct {
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}

// defaultBrandingTagline, defaultBrandingDescription, defaultBrandingFeatures, and
// defaultBrandingLinks are the real /server/about defaults sourced from IDEA.md
// (PART 16). They are used both to seed a fresh config and as the runtime fallback
// when an operator leaves a branding field empty.
const (
	defaultBrandingTagline     = "A fast, public paste sharing service — no account required."
	defaultBrandingDescription = "Pastebin is a fast, public paste sharing service. Share code snippets, configuration files, logs, and any text content instantly — no account required. It is a drop-in replacement for pastebin.com, microbin, and lenpaste, so existing scripts work without changes."
)

// defaultBrandingFeatures is the real feature list drawn from the IDEA.md in-scope
// items (PART 16 About page sourcing).
var defaultBrandingFeatures = []string{
	"Syntax highlighting for 20+ languages",
	"Configurable expiry (1 hour to 2 years, or never)",
	"Burn-after-N-views for ephemeral content",
	"Public and unlisted paste visibility",
	"Delete token — pastes are yours to remove",
	"Full REST API with JSON support",
	"Compatible with pastebin.com, microbin, and lenpaste CLIs",
	"No accounts, no tracking, no ads",
}

// defaultBrandingLinks is the real link list drawn from IDEA.md (PART 16).
var defaultBrandingLinks = []BrandingLink{
	{Label: "Source", URL: "https://github.com/apimgr/pastebin"},
	{Label: "apimgr", URL: "https://github.com/apimgr"},
}

// EffectiveTitle returns the configured branding title or the default project name.
func (b BrandingConfig) EffectiveTitle() string {
	if v := strings.TrimSpace(b.Title); v != "" {
		return v
	}
	return "Pastebin"
}

// EffectiveTagline returns the configured tagline or the IDEA.md default.
func (b BrandingConfig) EffectiveTagline() string {
	if v := strings.TrimSpace(b.Tagline); v != "" {
		return v
	}
	return defaultBrandingTagline
}

// EffectiveDescription returns the configured description or the IDEA.md default.
func (b BrandingConfig) EffectiveDescription() string {
	if v := strings.TrimSpace(b.Description); v != "" {
		return v
	}
	return defaultBrandingDescription
}

// EffectiveFeatures returns the configured feature list or the IDEA.md default.
func (b BrandingConfig) EffectiveFeatures() []string {
	if len(b.Features) > 0 {
		return b.Features
	}
	return defaultBrandingFeatures
}

// EffectiveLinks returns the configured link list or the IDEA.md default.
func (b BrandingConfig) EffectiveLinks() []BrandingLink {
	if len(b.Links) > 0 {
		return b.Links
	}
	return defaultBrandingLinks
}

// SEOConfig holds search-engine metadata (PART 12/16).
type SEOConfig struct {
	Keywords []string `yaml:"keywords"`
}

// BackupConfig holds backup, encryption, compliance, and retention settings (PART 21).
type BackupConfig struct {
	Encryption BackupEncryptionConfig `yaml:"encryption"`
	Compliance BackupComplianceConfig `yaml:"compliance"`
	Retention  BackupRetentionConfig  `yaml:"retention"`
}

// BackupEncryptionConfig controls AES-256-GCM + Argon2id backup encryption (PART 21).
type BackupEncryptionConfig struct {
	// Enabled turns on AES-256-GCM encryption for all backup archives.
	// Password is prompted interactively; never stored in config.
	Enabled bool `yaml:"enabled"`
}

// BackupComplianceConfig enables compliance metadata in backup manifests (PART 21).
type BackupComplianceConfig struct {
	// Enabled includes compliance metadata (retention policy, legal hold) in manifests.
	Enabled bool `yaml:"enabled"`
}

// BackupRetentionConfig controls how many backup archives are kept (PART 21).
type BackupRetentionConfig struct {
	// MaxBackups is the total maximum number of backup files to retain.
	MaxBackups  int `yaml:"max_backups"`
	KeepWeekly  int `yaml:"keep_weekly"`
	KeepMonthly int `yaml:"keep_monthly"`
	KeepYearly  int `yaml:"keep_yearly"`
}

// UpdateConfig controls self-update channel and scheduled check behaviour (PART 22).
type UpdateConfig struct {
	// Branch selects the release channel: "stable" (default), "beta", or "daily".
	Branch string `yaml:"branch"`
	// AutoInstall causes the daily update_check task to install eligible updates
	// automatically. Default false — the task notifies only.
	AutoInstall bool `yaml:"auto_install"`
	// DeferDays delays eligibility of a release: a release is only considered once
	// it has been public for this many days (0 = adopt immediately).
	DeferDays int `yaml:"defer_days"`
}

// SchedulerConfig configures the built-in task scheduler (PART 12/18).
// Tasks maps a task name to its per-task settings; absent entries use code defaults.
type SchedulerConfig struct {
	Enabled bool `yaml:"enabled"`
	// Timezone for cron expressions (default: America/New_York, override with TZ).
	Timezone string `yaml:"timezone"`
	// CatchUpWindow re-runs missed tasks on startup if due within this duration
	// (default: 1h). Accepts Go duration strings ("1h", "30m").
	CatchUpWindow string                   `yaml:"catch_up_window"`
	Tasks         map[string]SchedulerTask `yaml:"tasks"`
}

// SchedulerTask holds the settings for a single scheduled task (PART 12/18).
type SchedulerTask struct {
	Enabled     bool   `yaml:"enabled"`
	Schedule    string `yaml:"schedule"`
	RetryOnFail bool   `yaml:"retry_on_fail"`
	RetryDelay  string `yaml:"retry_delay"`
	// MaxAge and MaxSize apply to log_rotation.
	MaxAge  string `yaml:"max_age"`
	MaxSize string `yaml:"max_size"`
	// Retention applies to the backup task (max backups to keep).
	Retention int `yaml:"retention"`
	// RenewBefore applies to ssl_renewal (e.g. "7d").
	RenewBefore string `yaml:"renew_before"`
}

// HSTSConfig controls Strict-Transport-Security emission.
type HSTSConfig struct {
	Enabled           bool  `yaml:"enabled"`
	MaxAgeSeconds     int64 `yaml:"max_age_seconds"`    // default 63072000 (2 years)
	IncludeSubdomains bool  `yaml:"include_subdomains"` // default true
	Preload           bool  `yaml:"preload"`            // default true
}

// CSPConfig controls Content-Security-Policy emission.
type CSPConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Mode              string `yaml:"mode"` // enforce | report-only
	ScriptSrcExtra    string `yaml:"script_src_extra"`
	StyleSrcExtra     string `yaml:"style_src_extra"`
	ImgSrcExtra       string `yaml:"img_src_extra"`
	FontSrcExtra      string `yaml:"font_src_extra"`
	ConnectSrcExtra   string `yaml:"connect_src_extra"`
	FrameSrcExtra     string `yaml:"frame_src_extra"`
	FormActionExtra   string `yaml:"form_action_extra"`
	ScriptSrcOverride string `yaml:"script_src_override"`
	// frame-ancestors value for the embeddable /emb/{id} endpoint only.
	// Default "*" (any site may iframe embeds); operator can restrict to
	// specific origins, e.g. "'self' https://example.com". Empty = "*".
	EmbedFrameAncestors string `yaml:"embed_frame_ancestors"`
}

// DefaultConfig returns a config with sensible defaults.
// Server.Port is intentionally empty so that Load + ResolvePort can apply
// the "random 64xxx on first run" rule described in PART 5.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:       "",
			Address:    "0.0.0.0",
			FQDN:       "",
			Mode:       "production",
			APIVersion: "v1",
			Branding: BrandingConfig{
				Title:       "Pastebin",
				Tagline:     defaultBrandingTagline,
				Description: defaultBrandingDescription,
				Features:    defaultBrandingFeatures,
				Links:       defaultBrandingLinks,
			},
			SEO: SEOConfig{
				Keywords: []string{},
			},
			PIDFile:   true,
			Daemonize: false,
			TLS: TLSConfig{
				MinVersion: "TLS1.2",
				LetsEncrypt: LetsEncryptConfig{
					Challenge: "http-01",
				},
			},
			Scheduler: SchedulerConfig{
				Enabled: true,
				Tasks:   map[string]SchedulerTask{},
			},
			Metrics: MetricsConfig{
				Enabled:         true,
				Endpoint:        "/metrics",
				IncludeSystem:   true,
				IncludeRuntime:  true,
				Token:           "",
				DurationBuckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
				SizeBuckets:     []float64{100, 1000, 10000, 100000, 1000000, 10000000},
				AllowedIPs:      []string{},
			},
			GeoIP: GeoIPConfig{
				Enabled: true,
				// resolved at startup to {data_dir}/security/geoip
				Dir:            "",
				DenyCountries:  []string{},
				AllowCountries: []string{},
				Databases: GeoIPDatabasesConfig{
					ASN:     true,
					Country: true,
					City:    true,
					WHOIS:   true,
				},
			},
			Tor: TorConfig{
				Binary:                    "",
				UseNetwork:                false,
				AllowUserPreference:       true,
				MaxCircuits:               32,
				CircuitTimeout:            60,
				BootstrapTimeout:          180,
				SafeLogging:               true,
				MaxStreamsPerCircuit:      100,
				CloseCircuitOnStreamLimit: true,
				BandwidthRate:             "1 MB",
				BandwidthBurst:            "2 MB",
				MaxMonthlyBandwidth:       "100 GB",
				NumIntroPoints:            3,
				VirtualPort:               80,
			},
			Logging: LoggingConfig{
				AccessFormat: "apache",
				Level:        "info",
				Audit: AuditConfig{
					Enabled:          true,
					Filename:         "audit.log",
					Format:           "json",
					Rotate:           "daily",
					Keep:             "none",
					Compress:         false,
					IncludeUserAgent: true,
					MaskEmails:       true,
					TokenUsage:       false,
					Events: AuditEventCategories{
						Configuration: true,
						Security:      true,
						Backup:        true,
						Server:        true,
					},
				},
			},
			Notifications: NotificationsConfig{
				Email: EmailConfig{
					Enabled: false,
					SMTP: SMTPConfig{
						Host: "",
						Port: 587,
						TLS:  "auto",
					},
					// Per-event defaults from AI.md:26591-26597: high-signal events on, low-noise off.
					Events: EmailEventsConfig{
						Startup:         false,
						Shutdown:        false,
						BackupComplete:  false,
						BackupFailed:    true,
						SSLExpiring:     true,
						SSLRenewed:      false,
						SecurityAlert:   true,
						SchedulerError:  true,
						UpdateAvailable: false,
						UpdateInstalled: true,
					},
				},
			},
			Update: UpdateConfig{
				Branch:      "stable",
				AutoInstall: false,
				DeferDays:   0,
			},
			Backup: BackupConfig{
				Encryption: BackupEncryptionConfig{Enabled: false},
				Compliance: BackupComplianceConfig{Enabled: false},
				Retention: BackupRetentionConfig{
					MaxBackups:  30,
					KeepWeekly:  4,
					KeepMonthly: 12,
					KeepYearly:  3,
				},
			},
			Cache: CacheConfig{
				Type:     "memory",
				Host:     "localhost",
				Port:     6379,
				PoolSize: 10,
				MinIdle:  2,
				Timeout:  "5s",
				Prefix:   "pastebin:",
				TTL:      "1h",
			},
			Limits: LimitsConfig{
				// 10 MiB
				MaxBodySize:  10 << 20,
				ReadTimeout:  "30s",
				WriteTimeout: "30s",
				IdleTimeout:  "120s",
			},
			TrustedProxies: TrustedProxiesConfig{
				Additional: []string{},
			},
			URLDetection: URLDetectionConfig{
				Learning:     true,
				MinSamples:   3,
				SampleWindow: 5 * time.Minute,
				LogChanges:   true,
				LiveReload:   true,
			},
			Termbin: TermbinConfig{
				Enabled: true,
				Port:    9999,
				MaxSize: 32768,
				Timeout: "5s",
			},
			Maintenance: MaintenanceConfig{
				SelfHealing: MaintenanceSelfHealingConfig{
					Enabled:       true,
					RetryInterval: "30s",
					MaxAttempts:   0,
				},
				Cleanup: MaintenanceCleanupConfig{
					DiskThreshold:    90,
					LogRetentionDays: 7,
					BackupKeepCount:  5,
				},
				Notify: MaintenanceNotifyConfig{
					OnEnter: true,
					OnExit:  true,
				},
			},
			Contact: ContactConfig{
				Admin:    ContactRole{Email: "admin@{fqdn}", Webhooks: map[string]string{}},
				Security: ContactRole{Email: "security@{fqdn}", Webhooks: map[string]string{}},
				Abuse:    ContactRole{Email: "", Webhooks: map[string]string{}},
				General:  ContactRole{Email: "", Webhooks: map[string]string{}},
			},
			Pages: PagesConfig{
				Contact: ContactPageConfig{
					Enabled:        true,
					Captcha:        "simple",
					SuccessMessage: "Thank you for your message. We'll respond soon.",
				},
			},
			Privacy: PrivacyConfig{
				Data: DataPolicy{
					Sold:           false,
					StoredOnServer: true,
					Sharing: []SharingCondition{
						{
							Condition: "analytics",
							When:      "Tracking configured (server.tracking.type set) AND user consents",
							Data:      "Anonymized: page views, browser type, country",
						},
						{
							Condition: "email",
							When:      "SMTP configured for sending emails",
							Data:      "Email address, message content",
						},
						{
							Condition: "user_initiated",
							When:      "User explicitly shares content (social buttons, exports)",
							Data:      "Whatever user chooses to share",
						},
					},
				},
				Retention: RetentionPolicy{
					Period:            "Account data is retained while your account is active. Upon account deletion, all personal data is permanently deleted within 30 days. Anonymized analytics data may be retained for up to 12 months.",
					ExportAvailable:   true,
					DeletionAvailable: true,
				},
				Consent: ConsentConfig{
					ShowUntilAcknowledged: true,
					DefaultEnabled:        true,
					Message:               "In accordance with the EU GDPR law this message is being displayed. We use cookies for essential site functionality and, with your consent, for preferences and analytics. Your data is stored on our servers and is never sold.",
					MessageIfSold:         "In accordance with the EU GDPR law this message is being displayed. We use cookies for essential site functionality and, with your consent, for preferences and analytics. Your data may be shared with or sold to third parties as described in our Privacy Policy.",
					Policy: ConsentPolicyLink{
						Text: "Privacy Policy",
						URL:  "/server/privacy",
					},
					Buttons: ConsentButtons{
						Decline: "Decline",
						Accept:  "I Agree",
					},
					Position:        "bottom",
					ShowPreferences: true,
					PreferencesText: "Manage Preferences",
				},
				Cookies: CookieCategories{
					Essential: CookieCategory{
						Enabled:     true,
						Description: "Required for the site to function. Includes security tokens (CSRF) and site preferences. These cookies are strictly necessary and cannot be disabled.",
					},
					Preferences: CookieCategory{
						Enabled:     true,
						Description: "Remember your settings such as theme (dark/light), language, and UI preferences. Disabling will reset to defaults on each visit.",
					},
					Analytics: AnalyticsCookie{
						CookieCategory: CookieCategory{
							Enabled:     true,
							Description: "Help us understand how visitors use our site to improve the experience.",
						},
						DescriptionSuffixNotSold: "Analytics data is anonymized and never sold.",
						DescriptionSuffixSold:    "Analytics data may be shared with third parties.",
					},
				},
				ThirdParty: ThirdPartyConfig{
					Services: []ThirdPartyService{},
				},
				Content: PrivacyContent{
					DataCollection:  "**We collect only what is necessary to provide our service:**\n\n**Usage Information (with consent):**\n- Pages visited and features used\n- Browser type and device information\n- Approximate location (country/region from IP, not precise)\n\n**Technical Information:**\n- IP address (for security and abuse prevention)\n- API token identifiers (hashed, never stored in plain text)\n\n**We do NOT collect:**\n- Payment information (unless explicitly required by the service)\n- Precise location data\n- Data from other websites or apps\n",
					DataUsage:       "**Your data is used solely to:**\n\n- **Provide the service:** Authentication, core functionality\n- **Improve the experience:** Performance optimization, bug fixes, feature improvements\n- **Ensure security:** Prevent abuse, detect fraud, protect your account\n- **Communicate:** Service updates, security alerts, and (with consent) product news\n\n**Your data is NEVER:**\n- Sold to third parties\n- Used for targeted advertising\n- Shared without your explicit consent (except as required by law)\n",
					DataUsageIfSold: "**Your data may be used to:**\n\n- **Provide the service:** Authentication, core functionality\n- **Improve the experience:** Performance optimization, bug fixes, feature improvements\n- **Ensure security:** Prevent abuse, detect fraud, protect your account\n- **Communicate:** Service updates, security alerts, and (with consent) product news\n- **Third-party sharing:** Your data may be shared with or sold to third parties for analytics, advertising, or other purposes as described below\n\n**Your rights:**\n- You can opt out of data sales via your account privacy settings\n- You can request deletion of your data at any time\n- See \"Your Rights\" section below for details\n",
					DataSecurity:    "**How we protect your data:**\n\n- All data is stored on our servers (not third-party cloud services unless specified)\n- API tokens are stored as SHA-256 hashes (never in plain text)\n- All connections are encrypted (HTTPS/TLS)\n- Regular security audits and updates\n- Access controls and audit logging for operator actions\n",
				},
			},
			Tracking: TrackingConfig{
				Type: "",
				ID:   "",
				URL:  "",
			},
		},
		Database: DatabaseConfig{
			Type: "sqlite",
			// resolved at startup relative to data dir
			Path: "",
		},
		Paste: PasteConfig{
			// 10 MiB
			MaxSizeBytes:    10 << 20,
			DefaultExpiry:   "never",
			DefaultLanguage: "text",
			MaxBurnAfter:    9999,
			AllowUnlisted:   true,
		},
		RateLimit: RateLimitConfig{
			Enabled:     true,
			Read:        RateLimitEndpoint{Requests: 120, Window: 60},
			Write:       RateLimitEndpoint{Requests: 10, Window: 60},
			Health:      RateLimitEndpoint{Requests: 120, Window: 60},
			GlobalBurst: 240,
		},
		Web: WebConfig{
			SiteTitle: "Pastebin",
			Theme:     "dark",
			Robots: RobotsConfig{
				Allow: []string{"/"},
				Deny:  []string{},
			},
			Security: SecurityConfig{
				CORS: "*",
				// auto-generated on first run
				EncryptionKey: "",
				Blocklists: BlocklistsConfig{
					Enabled: true,
					Sources: []BlocklistSource{
						{
							File: "firehol_level1.txt",
							URL:  "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset",
						},
						{
							File: "spamhaus_drop.txt",
							URL:  "https://www.spamhaus.org/drop/drop.txt",
						},
					},
				},
				CVE: CVEConfig{
					Enabled: false,
					File:    "nvd.json",
					Source:  "https://nvd.nist.gov/feeds/json/cve/1.1",
				},
			},
			HSTS: HSTSConfig{
				Enabled: true,
				// 2 years (preload-list eligible)
				MaxAgeSeconds:     63072000,
				IncludeSubdomains: true,
				Preload:           true,
			},
			CSP: CSPConfig{
				Enabled:             true,
				Mode:                "enforce",
				EmbedFrameAncestors: "*",
			},
			Headers: HeadersConfig{
				SecFetchValidation: true,
			},
			CSRF: CSRFConfig{
				Enabled:     true,
				TokenLength: 32,
				CookieName:  "csrf_token",
				HeaderName:  "X-CSRF-Token",
				Secure:      "auto",
				ExemptPaths: []string{},
			},
		},
	}
}

// Load reads config from path, creating it with defaults if absent.
// On first run (or when encryption_key is empty) an AES-256-GCM key is
// auto-generated and persisted to server.yml immediately.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.loadEnv()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if genErr := cfg.ensureEncryptionKey(); genErr != nil {
				log.Printf("config: warning: could not generate encryption key: %v", genErr)
			}
			if genErr := cfg.ensureServerToken(); genErr != nil {
				log.Printf("config: warning: could not generate server token: %v", genErr)
			} else {
				log.Printf("config: generated server.token — copy it from %s", path)
			}
			// Write canonical defaults to disk. Never bake runtime env-var overrides
			// (MODE, DEBUG, etc.) into the persisted config — env vars are runtime-only.
			// Copy only the generated secrets so they survive the next startup.
			saveCfg := DefaultConfig()
			saveCfg.Server.FQDN = cfg.ResolveFQDN()
			saveCfg.Web.Security.EncryptionKey = cfg.Web.Security.EncryptionKey
			saveCfg.Server.Token = cfg.Server.Token
			_ = Save(path, saveCfg)
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}

	// Env always wins over file.
	cfg.loadEnv()

	// Validate and sanitize — replace invalid values with defaults, never crash.
	Validate(cfg)

	// Generate and persist encryption key if missing (upgrade path for older configs).
	needSave := false
	if cfg.Web.Security.EncryptionKey == "" {
		if genErr := cfg.ensureEncryptionKey(); genErr != nil {
			log.Printf("config: warning: could not generate encryption key: %v", genErr)
		} else {
			needSave = true
		}
	}

	// Generate and persist server.token if missing (upgrade path for older configs).
	if cfg.Server.Token == "" {
		if genErr := cfg.ensureServerToken(); genErr != nil {
			log.Printf("config: warning: could not generate server token: %v", genErr)
		} else {
			needSave = true
			log.Printf("config: generated server.token — copy it from %s", path)
		}
	}

	// Generate and persist a per-webhook signing secret for every configured
	// webhook URL that lacks one (PART 17).
	if gen, genErr := cfg.ensureWebhookSecrets(); genErr != nil {
		log.Printf("config: warning: could not generate webhook secret: %v", genErr)
	} else if gen {
		needSave = true
	}

	if needSave {
		_ = Save(path, cfg)
	}

	return cfg, nil
}

// ensureEncryptionKey generates a 32-byte AES-256-GCM key and stores it
// hex-encoded in Web.Security.EncryptionKey. Idempotent — no-ops if already set.
func (c *Config) ensureEncryptionKey() error {
	if c.Web.Security.EncryptionKey != "" {
		return nil
	}
	var key [32]byte
	if _, err := crand.Read(key[:]); err != nil {
		return err
	}
	c.Web.Security.EncryptionKey = fmt.Sprintf("%x", key)
	return nil
}

// tokenCharset is the base62 alphabet used for operator and resource-owner tokens (PART 11).
const tokenCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// ensureServerToken generates a "tok_" + 32 base62 char operator token and
// stores it in Server.Token. Idempotent — no-ops if already set.
func (c *Config) ensureServerToken() error {
	if c.Server.Token != "" {
		return nil
	}
	raw := make([]byte, 32)
	if _, err := crand.Read(raw); err != nil {
		return err
	}
	b := make([]byte, 32)
	for i, v := range raw {
		b[i] = tokenCharset[int(v)%len(tokenCharset)]
	}
	c.Server.Token = "tok_" + string(b)
	return nil
}

// EncryptionKey returns the decoded 32-byte AES-256-GCM key, or an error if
// the key is absent or malformed.
func (c *Config) EncryptionKey() ([]byte, error) {
	s := c.Web.Security.EncryptionKey
	if s == "" {
		return nil, fmt.Errorf("encryption_key not configured")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("encryption_key is not valid hex: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("encryption_key must be 32 bytes (got %d)", len(b))
	}
	return b, nil
}

// fqdn returns the server FQDN used to expand {fqdn} tokens in contact
// addresses. It follows the AI.md {fqdn} resolution order (PART 12): the
// configured/DOMAIN value first, then os.Hostname(), then $HOSTNAME, then the
// first public IP, and only "localhost" as the last resort. Reverse-proxy
// header detection (priority 1) is request-scoped and handled at the HTTP layer,
// so it is out of scope here.
func (c *Config) fqdn() string {
	if f := strings.TrimSpace(c.Server.FQDN); f != "" && !strings.EqualFold(f, "localhost") {
		return f
	}
	if h, err := os.Hostname(); err == nil {
		if h = strings.TrimSpace(h); h != "" && !strings.EqualFold(h, "localhost") {
			return h
		}
	}
	if h := strings.TrimSpace(os.Getenv("HOSTNAME")); h != "" && !strings.EqualFold(h, "localhost") {
		return h
	}
	if ip := firstPublicIP(); ip != "" {
		return ip
	}
	return "localhost"
}

// ResolveFQDN exposes the configuration-scoped {fqdn} resolution chain (PART 12
// priorities 2–7: configured/DOMAIN value → os.Hostname() → $HOSTNAME → first
// public IP → "localhost") to the HTTP layer. The request-scoped reverse-proxy
// header step (priority 1) is applied by the caller before falling back here.
func (c *Config) ResolveFQDN() string {
	return c.fqdn()
}

// firstPublicIP returns the first globally routable unicast address bound to a
// local interface, preferring IPv6 over IPv4, or "" when none is found. Private,
// loopback, and link-local addresses are excluded (AI.md PART 12 {fqdn}
// resolution priorities 5 and 6).
func firstPublicIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	var v4 string
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			continue
		}
		if ip.To4() == nil {
			return ip.String()
		}
		if v4 == "" {
			v4 = ip.String()
		}
	}
	return v4
}

// expandFQDN substitutes the {fqdn} placeholder in a contact value with the
// resolved server FQDN.
func (c *Config) expandFQDN(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "{fqdn}", c.fqdn())
}

// AdminEmail returns the resolved admin recipient — the universal fallback for
// every other role (PART 12). Falls back to admin@{fqdn} when unset.
func (c *Config) AdminEmail() string {
	if e := c.expandFQDN(c.Server.Contact.Admin.Email); e != "" {
		return e
	}
	return "admin@" + c.fqdn()
}

// SecurityEmail returns the resolved security-report recipient (PART 12).
// Empty → falls back to the admin address. Public: security.txt Contact line.
func (c *Config) SecurityEmail() string {
	if e := c.expandFQDN(c.Server.Contact.Security.Email); e != "" {
		return e
	}
	return c.AdminEmail()
}

// SecurityReportEmail returns the operator recipient for coordinated-disclosure
// security reports (PART 11). Prefers web.security.contact_email, then falls
// back to the canonical security.txt contact (SecurityEmail).
func (c *Config) SecurityReportEmail() string {
	if e := c.expandFQDN(strings.TrimSpace(c.Web.Security.ContactEmail)); e != "" {
		return e
	}
	return c.SecurityEmail()
}

// PublishPGPKeyEnabled reports whether the project PGP key is published
// (pgp-key.asc, security.txt Encryption line, keyserver submission) (PART 11).
func (c *Config) PublishPGPKeyEnabled() bool {
	return IsTruthy(c.Web.Security.PublishPGPKey)
}

// SecurityReportURL returns the primary security.txt Contact line — the repo's
// GitHub private vulnerability reporting URL, listed first per RFC 9116 (PART
// 11). Falls back to the canonical apimgr/pastebin advisories endpoint.
func (c *Config) SecurityReportURL() string {
	if u := strings.TrimSpace(c.Web.Security.ReportURL); u != "" {
		return u
	}
	return "https://github.com/apimgr/pastebin/security/advisories/new"
}

// SecurityPreferredLanguages returns the security.txt Preferred-Languages value
// (RFC 5646 tags, comma-separated). Empty config → "en" (PART 11).
func (c *Config) SecurityPreferredLanguages() string {
	if l := strings.TrimSpace(c.Web.Security.PreferredLanguages); l != "" {
		return l
	}
	return "en"
}

// GeneralEmail returns the resolved /server/contact recipient (PART 12).
// Empty → falls back to the admin address. Public: footer "Contact us".
func (c *Config) GeneralEmail() string {
	if e := c.expandFQDN(c.Server.Contact.General.Email); e != "" {
		return e
	}
	return c.AdminEmail()
}

// GeneralEmailPublic returns the /server/contact address ONLY when the operator
// explicitly set server.contact.general.email; otherwise it returns "" (PART 12).
// Unlike GeneralEmail it never falls back to the admin address, because the admin
// email is server-internal and MUST NEVER be exposed publicly (AI.md Privacy &
// Public Exposure: admin.email NEVER public). The contact form still delivers to
// the admin fallback via GeneralEmail; only the displayed "Contact us" address is
// suppressed when general is unset.
func (c *Config) GeneralEmailPublic() string {
	return c.expandFQDN(c.Server.Contact.General.Email)
}

// AbuseEmail returns the resolved abuse-report delivery recipient (PART 12):
// abuse.email if set → general.email if set → admin.email. Used for routing
// incoming abuse reports; never auto-advertises abuse@{fqdn}.
func (c *Config) AbuseEmail() string {
	if e := c.expandFQDN(c.Server.Contact.Abuse.Email); e != "" {
		return e
	}
	return c.GeneralEmail()
}

// AbuseEmailPublic returns the address shown in the contact page "Abuse Reports"
// section (PART 12): abuse.email if explicitly set → else the public general
// address (general.email if set, otherwise ""). It NEVER falls back to the admin
// address, which is server-internal and must not be exposed publicly.
func (c *Config) AbuseEmailPublic() string {
	if e := c.expandFQDN(c.Server.Contact.Abuse.Email); e != "" {
		return e
	}
	return c.GeneralEmailPublic()
}

// ContactWebhook returns the webhook URL for a role+transport, falling back
// through the role's documented chain to the admin transport when empty (PART
// 12). abuse → general → admin; security/general → admin.
func (c *Config) ContactWebhook(role, transport string) string {
	pick := func(r ContactRole) string {
		if r.Webhooks == nil {
			return ""
		}
		return strings.TrimSpace(r.Webhooks[transport])
	}
	var v string
	switch role {
	case "security":
		v = pick(c.Server.Contact.Security)
	case "abuse":
		if v = pick(c.Server.Contact.Abuse); v == "" {
			v = pick(c.Server.Contact.General)
		}
	case "general":
		v = pick(c.Server.Contact.General)
	default:
		v = pick(c.Server.Contact.Admin)
	}
	if v != "" {
		return v
	}
	return pick(c.Server.Contact.Admin)
}

// webhookSecretSuffix is appended to a transport name to form the config key
// that stores its per-webhook signing secret (PART 17): webhooks.<name>_secret.
const webhookSecretSuffix = "_secret"

// WebhookTarget is a resolved webhook destination for dispatch: the transport
// adapter name, the destination URL, and the per-webhook signing secret.
type WebhookTarget struct {
	Transport string
	URL       string
	Secret    string
}

// contactRole returns the ContactRole for a role name
// (admin/security/abuse/general), defaulting to admin for any unknown name.
func (c *Config) contactRole(role string) ContactRole {
	switch role {
	case "security":
		return c.Server.Contact.Security
	case "abuse":
		return c.Server.Contact.Abuse
	case "general":
		return c.Server.Contact.General
	default:
		return c.Server.Contact.Admin
	}
}

// WebhookSecret returns the per-webhook signing secret for a role+transport,
// paired with whichever role actually supplies the URL (role-specific value if
// set, else the admin fallback) (PART 17).
func (c *Config) WebhookSecret(role, transport string) string {
	r := c.contactRole(role)
	if r.Webhooks != nil && strings.TrimSpace(r.Webhooks[transport]) != "" {
		return strings.TrimSpace(r.Webhooks[transport+webhookSecretSuffix])
	}
	if c.Server.Contact.Admin.Webhooks != nil {
		return strings.TrimSpace(c.Server.Contact.Admin.Webhooks[transport+webhookSecretSuffix])
	}
	return ""
}

// WebhookTargets returns every configured webhook destination for a role,
// applying the admin-role fallback and pairing each URL with its signing
// secret. `_secret` companion keys are not treated as transports (PART 17).
func (c *Config) WebhookTargets(role string) []WebhookTarget {
	seen := map[string]bool{}
	var out []WebhookTarget
	add := func(r ContactRole) {
		for name := range r.Webhooks {
			if strings.HasSuffix(name, webhookSecretSuffix) {
				continue
			}
			if seen[name] {
				continue
			}
			u := c.ContactWebhook(role, name)
			if strings.TrimSpace(u) == "" {
				continue
			}
			seen[name] = true
			out = append(out, WebhookTarget{Transport: name, URL: u, Secret: c.WebhookSecret(role, name)})
		}
	}
	add(c.contactRole(role))
	// Abuse resolves abuse → general → admin, so enumerate general's transports
	// too before the shared admin fallback (PART 12).
	if role == "abuse" {
		add(c.Server.Contact.General)
	}
	add(c.Server.Contact.Admin)
	return out
}

// ensureWebhookSecrets generates a random 32-byte hex signing secret for every
// configured webhook URL that lacks one, storing it in the same webhooks map as
// `<name>_secret`. Reports whether any secret was generated (PART 17).
func (c *Config) ensureWebhookSecrets() (bool, error) {
	generated := false
	roles := []*ContactRole{&c.Server.Contact.Admin, &c.Server.Contact.Security, &c.Server.Contact.Abuse, &c.Server.Contact.General}
	for _, r := range roles {
		if r.Webhooks == nil {
			continue
		}
		for name, url := range r.Webhooks {
			if strings.HasSuffix(name, webhookSecretSuffix) {
				continue
			}
			if strings.TrimSpace(url) == "" {
				continue
			}
			key := name + webhookSecretSuffix
			if strings.TrimSpace(r.Webhooks[key]) != "" {
				continue
			}
			var b [32]byte
			if _, err := crand.Read(b[:]); err != nil {
				return generated, err
			}
			r.Webhooks[key] = fmt.Sprintf("%x", b)
			generated = true
		}
	}
	return generated, nil
}

func (c *Config) loadEnv() {
	// PART 8: the canonical env override is {PROJECT_NAME}_PORT (PASTEBIN_PORT);
	// PORT is the generic docker alias. PASTEBIN_PORT wins when both are set.
	if v := os.Getenv("PORT"); v != "" {
		c.Server.Port = v
	}
	if v := os.Getenv("PASTEBIN_PORT"); v != "" {
		c.Server.Port = v
	}
	// LISTEN is the AI.md-canonical name (PART 5); ADDRESS is an accepted alias.
	if v := os.Getenv("ADDRESS"); v != "" {
		c.Server.Address = v
	}
	if v := os.Getenv("LISTEN"); v != "" {
		c.Server.Address = v
	}
	if v := os.Getenv("BASE_URL"); v != "" {
		c.Server.BaseURL = v
	}
	// DOMAIN is the highest-priority FQDN override (AI.md 7548, 7561).
	if v := os.Getenv("DOMAIN"); v != "" {
		c.Server.FQDN = v
	}
	// DATABASE_DRIVER selects the storage backend (AI.md 7550).
	if v := os.Getenv("DATABASE_DRIVER"); v != "" {
		c.Database.Type = v
	}
	// DATABASE_URL / DB_PATH set the connection string (AI.md 7551); DB_PATH is an alias.
	if v := os.Getenv("DB_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		c.Database.Path = v
	}
	// CACHE_URL is the container-canonical cache override (AI.md 31725), e.g.
	// "valkey://host:6379" or "redis://host:6379". The cache driver dispatches on
	// Type, so derive Type from the URL scheme; a URL alone would otherwise stay on
	// the default in-memory driver and silently ignore the remote endpoint.
	if v := os.Getenv("CACHE_URL"); v != "" {
		c.Server.Cache.URL = v
		switch {
		case strings.HasPrefix(v, "valkey://"), strings.HasPrefix(v, "valkeys://"):
			c.Server.Cache.Type = "valkey"
		case strings.HasPrefix(v, "redis://"), strings.HasPrefix(v, "rediss://"):
			c.Server.Cache.Type = "redis"
		}
	}
	// APPLICATION_NAME is the AI.md-canonical application title (AI.md 7579); SITE_TITLE is an alias.
	if v := os.Getenv("SITE_TITLE"); v != "" {
		c.Web.SiteTitle = v
	}
	if v := os.Getenv("APPLICATION_NAME"); v != "" {
		c.Server.Branding.Title = v
	}
	// APPLICATION_TAGLINE sets the application description/tagline (AI.md 7580).
	if v := os.Getenv("APPLICATION_TAGLINE"); v != "" {
		c.Server.Branding.Tagline = v
	}
	if v := os.Getenv("THEME"); v != "" {
		c.Web.Theme = v
	}
	if v := os.Getenv("MAX_SIZE_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.Paste.MaxSizeBytes = n
		}
	}
	if v := os.Getenv("SMTP_HOST"); v != "" {
		c.Server.Notifications.Email.SMTP.Host = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Server.Notifications.Email.SMTP.Port = n
		}
	}
	if v := os.Getenv("SMTP_USERNAME"); v != "" {
		c.Server.Notifications.Email.SMTP.Username = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		c.Server.Notifications.Email.SMTP.Password = v
	}
	if v := os.Getenv("SMTP_TLS"); v != "" {
		c.Server.Notifications.Email.SMTP.TLS = v
	}
	if v := os.Getenv("SMTP_FROM_NAME"); v != "" {
		c.Server.Notifications.Email.From.Name = v
	}
	if v := os.Getenv("SMTP_FROM_EMAIL"); v != "" {
		c.Server.Notifications.Email.From.Email = v
	}
	if v := os.Getenv("TERMBIN_ENABLED"); v != "" {
		if b, err := ParseBool(v, c.Server.Termbin.Enabled); err == nil {
			c.Server.Termbin.Enabled = b
		}
	}
	if v := os.Getenv("TERMBIN_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Server.Termbin.Port = n
		}
	}
	if v := os.Getenv("TERMBIN_MAX_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.Server.Termbin.MaxSize = n
		}
	}
	if v := os.Getenv("TERMBIN_TIMEOUT"); v != "" {
		c.Server.Termbin.Timeout = v
	}
	// MODE is a runtime variable — always overrides config file (AI.md PART 5).
	// Never baked into the persisted config; only applies at runtime.
	if v := os.Getenv("MODE"); v != "" {
		c.Server.Mode = v
	}
}

// Validate checks all config values, replaces invalid ones with defaults, and
// logs a warning for each replacement. It never returns an error; the server
// must always start with sane defaults even if the config file is malformed (PART 12).
func Validate(cfg *Config) {
	d := DefaultConfig()

	// Limits: timeouts must be parseable and positive.
	for _, pair := range []struct {
		name *string
		def  string
	}{
		{&cfg.Server.Limits.ReadTimeout, d.Server.Limits.ReadTimeout},
		{&cfg.Server.Limits.WriteTimeout, d.Server.Limits.WriteTimeout},
		{&cfg.Server.Limits.IdleTimeout, d.Server.Limits.IdleTimeout},
	} {
		if dur, err := time.ParseDuration(*pair.name); err != nil || dur <= 0 {
			log.Printf("[config] WARNING: invalid timeout %q, using default %s", *pair.name, pair.def)
			*pair.name = pair.def
		}
	}

	// Limits: max body size must be positive.
	if cfg.Server.Limits.MaxBodySize <= 0 {
		log.Printf("[config] WARNING: invalid max_body_size %d, using default %d",
			cfg.Server.Limits.MaxBodySize, d.Server.Limits.MaxBodySize)
		cfg.Server.Limits.MaxBodySize = d.Server.Limits.MaxBodySize
	}

	// Termbin: when enabled, port/size/timeout must be sane.
	if cfg.Server.Termbin.Enabled {
		if cfg.Server.Termbin.Port <= 0 || cfg.Server.Termbin.Port > 65535 {
			log.Printf("[config] WARNING: invalid termbin.port %d, using default %d",
				cfg.Server.Termbin.Port, d.Server.Termbin.Port)
			cfg.Server.Termbin.Port = d.Server.Termbin.Port
		}
		if cfg.Server.Termbin.MaxSize <= 0 {
			log.Printf("[config] WARNING: invalid termbin.max_size %d, using default %d",
				cfg.Server.Termbin.MaxSize, d.Server.Termbin.MaxSize)
			cfg.Server.Termbin.MaxSize = d.Server.Termbin.MaxSize
		}
		if dur, err := time.ParseDuration(cfg.Server.Termbin.Timeout); err != nil || dur <= 0 {
			log.Printf("[config] WARNING: invalid termbin.timeout %q, using default %s",
				cfg.Server.Termbin.Timeout, d.Server.Termbin.Timeout)
			cfg.Server.Termbin.Timeout = d.Server.Termbin.Timeout
		}
	}

	// Contact page captcha must be one of the supported challenge types.
	switch cfg.Server.Pages.Contact.Captcha {
	case "simple", "recaptcha", "hcaptcha":
	default:
		log.Printf("[config] WARNING: invalid pages.contact.captcha %q, using default %q",
			cfg.Server.Pages.Contact.Captcha, d.Server.Pages.Contact.Captcha)
		cfg.Server.Pages.Contact.Captcha = d.Server.Pages.Contact.Captcha
	}
	if cfg.Server.Pages.Contact.SuccessMessage == "" {
		cfg.Server.Pages.Contact.SuccessMessage = d.Server.Pages.Contact.SuccessMessage
	}

	// Web theme must be one of the valid values.
	switch cfg.Web.Theme {
	case "dark", "light", "auto":
	default:
		log.Printf("[config] WARNING: invalid web.theme %q, using default \"dark\"", cfg.Web.Theme)
		cfg.Web.Theme = "dark"
	}

	// Rate limit counts must be non-negative (zero means "disabled for this class").
	if cfg.RateLimit.Read.Requests < 0 {
		log.Printf("[config] WARNING: rate_limit.read.requests < 0, using default %d",
			d.RateLimit.Read.Requests)
		cfg.RateLimit.Read.Requests = d.RateLimit.Read.Requests
	}
	if cfg.RateLimit.Write.Requests < 0 {
		log.Printf("[config] WARNING: rate_limit.write.requests < 0, using default %d",
			d.RateLimit.Write.Requests)
		cfg.RateLimit.Write.Requests = d.RateLimit.Write.Requests
	}
	if cfg.RateLimit.Health.Requests < 0 {
		log.Printf("[config] WARNING: rate_limit.health.requests < 0, using default %d",
			d.RateLimit.Health.Requests)
		cfg.RateLimit.Health.Requests = d.RateLimit.Health.Requests
	}
	if cfg.RateLimit.GlobalBurst < 0 {
		log.Printf("[config] WARNING: rate_limit.global_burst < 0, using default %d",
			d.RateLimit.GlobalBurst)
		cfg.RateLimit.GlobalBurst = d.RateLimit.GlobalBurst
	}

	// Paste size must be positive.
	if cfg.Paste.MaxSizeBytes <= 0 {
		log.Printf("[config] WARNING: paste.max_size_bytes <= 0, using default %d",
			d.Paste.MaxSizeBytes)
		cfg.Paste.MaxSizeBytes = d.Paste.MaxSizeBytes
	}

	// Cache TTL and timeout must be parseable.
	for _, pair := range []struct {
		name *string
		def  string
	}{
		{&cfg.Server.Cache.Timeout, d.Server.Cache.Timeout},
		{&cfg.Server.Cache.TTL, d.Server.Cache.TTL},
	} {
		if dur, err := time.ParseDuration(*pair.name); err != nil || dur <= 0 {
			log.Printf("[config] WARNING: invalid cache duration %q, using default %s",
				*pair.name, pair.def)
			*pair.name = pair.def
		}
	}

	// Logging level must be a known value.
	switch cfg.Server.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		log.Printf("[config] WARNING: invalid logging.level %q, using default \"info\"",
			cfg.Server.Logging.Level)
		cfg.Server.Logging.Level = "info"
	}

	// Consent banner position must be top or bottom.
	switch cfg.Server.Privacy.Consent.Position {
	case "top", "bottom":
	default:
		log.Printf("[config] WARNING: invalid privacy.consent.position %q, using default %q",
			cfg.Server.Privacy.Consent.Position, d.Server.Privacy.Consent.Position)
		cfg.Server.Privacy.Consent.Position = d.Server.Privacy.Consent.Position
	}
	// Essential cookies are strictly necessary and cannot be disabled.
	cfg.Server.Privacy.Cookies.Essential.Enabled = true
	// Fall back to default text for any blank consent-banner field.
	for _, pair := range []struct {
		val *string
		def string
	}{
		{&cfg.Server.Privacy.Consent.Message, d.Server.Privacy.Consent.Message},
		{&cfg.Server.Privacy.Consent.MessageIfSold, d.Server.Privacy.Consent.MessageIfSold},
		{&cfg.Server.Privacy.Consent.Policy.Text, d.Server.Privacy.Consent.Policy.Text},
		{&cfg.Server.Privacy.Consent.Policy.URL, d.Server.Privacy.Consent.Policy.URL},
		{&cfg.Server.Privacy.Consent.Buttons.Decline, d.Server.Privacy.Consent.Buttons.Decline},
		{&cfg.Server.Privacy.Consent.Buttons.Accept, d.Server.Privacy.Consent.Buttons.Accept},
		{&cfg.Server.Privacy.Consent.PreferencesText, d.Server.Privacy.Consent.PreferencesText},
	} {
		if strings.TrimSpace(*pair.val) == "" {
			*pair.val = pair.def
		}
	}

	// Analytics configuration must be valid; on error warn and disable tracking
	// (warn-and-default: never fail startup, PART 5).
	if err := ValidateTracking(&cfg.Server.Tracking); err != nil {
		log.Printf("[config] WARNING: invalid server.tracking (%v), disabling analytics", err)
		cfg.Server.Tracking = TrackingConfig{}
	}
}

// ValidateTracking validates the analytics tracking configuration (PART 31).
// An empty or "none" type is valid (disabled). Returns a descriptive error for
// any malformed ID/URL so the caller can warn-and-disable.
func ValidateTracking(cfg *TrackingConfig) error {
	if cfg.Type == "" || cfg.Type == "none" {
		return nil
	}
	switch cfg.Type {
	case "google":
		if !regexp.MustCompile(`^(UA-\d+-\d+|G-[A-Z0-9]+)$`).MatchString(cfg.ID) {
			return errors.New("invalid Google Analytics ID format")
		}
	case "matomo", "piwik":
		if _, err := strconv.Atoi(cfg.ID); err != nil {
			return errors.New("matomo/piwik ID must be an integer")
		}
		if cfg.URL == "" {
			return errors.New("matomo/piwik requires URL")
		}
	case "owa":
		if cfg.ID == "" {
			return errors.New("OWA requires site ID")
		}
		if cfg.URL == "" {
			return errors.New("OWA requires URL")
		}
	case "fathom":
		if cfg.ID == "" {
			return errors.New("fathom requires site ID")
		}
	case "plausible":
		if cfg.ID == "" {
			return errors.New("plausible requires domain")
		}
	case "umami":
		if _, err := uuid.Parse(cfg.ID); err != nil {
			return errors.New("umami ID must be a valid UUID")
		}
		if cfg.URL == "" {
			return errors.New("umami requires URL")
		}
	case "simple":
	case "cloudflare":
		if cfg.ID == "" {
			return errors.New("cloudflare requires beacon token")
		}
	default:
		return fmt.Errorf("unknown tracking type: %s", cfg.Type)
	}
	// Any provided self-hosted URL must be a parseable absolute URL.
	if cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil {
			return fmt.Errorf("invalid tracking URL: %w", err)
		}
		if u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid tracking URL: %q is not absolute", cfg.URL)
		}
	}
	return nil
}

// Save writes config to path.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

// SetUpdateBranch loads the config at path, sets server.update.branch to the
// given value, and saves the file back. Creates the file and parent dirs if absent.
func SetUpdateBranch(path, branch string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	cfg, err := Load(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}
	cfg.Server.Update.Branch = branch
	return Save(path, cfg)
}

// ResolvePort finalises cfg.Server.Port according to the PART 5 / PART 8 rules.
// Precedence (highest first):
//
//   - Explicit --port flag / $PASTEBIN_PORT / $PORT env / config file value → use as-is
//   - No port configured (empty), container environment → "80" (docker default)
//   - No port configured (empty), non-container → pick a random unused port in
//     64000-64999, persist it to cfgPath so subsequent restarts use the same port
//
// The caller must apply CLI flag overrides to cfg BEFORE calling this function
// so that an explicit --port value takes precedence over the container default
// and the persisted value. cfgPath is the path used to save the selected random
// port. In a container an explicit port always wins; 80 is only the fallback
// default when nothing else set it.
func ResolvePort(cfgPath string, cfg *Config, inContainer bool) error {
	// An explicit value (flag, env, or config file) always wins, in or out of
	// a container.
	if cfg.Server.Port != "" {
		return nil
	}
	if inContainer {
		// Container default when nothing else configured the port.
		cfg.Server.Port = "80"
		return nil
	}

	// First run: no port configured — pick a random unused port in 64000-64999.
	port, err := randomUnusedPort(64000, 64999)
	if err != nil {
		return fmt.Errorf("port allocator: %w", err)
	}
	cfg.Server.Port = strconv.Itoa(port)

	// Persist only the port. Read the canonical config file, update Server.Port,
	// and save it back. Never save cfg directly: cfg carries runtime env-var
	// overrides (MODE, etc.) that must not be baked into the persisted config
	// (AI.md PART 5 — runtime variables are never persisted).
	if saveErr := persistPortOnly(cfgPath, cfg.Server.Port); saveErr != nil {
		// Non-fatal: log at the call site; the server can still start.
		return fmt.Errorf("port allocator: could not persist port to %s: %w", cfgPath, saveErr)
	}
	return nil
}

// persistPortOnly reads the config file at path, sets Server.Port to port, and
// saves the result. Using a clean file-round-trip ensures runtime env-var
// overrides are never written to disk (AI.md PART 5).
func persistPortOnly(path string, port string) error {
	base := DefaultConfig()
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, base)
	}
	base.Server.Port = port
	return Save(path, base)
}

// SplitPorts parses a port specification into an optional plain-HTTP port and an
// optional HTTPS port following the PART 15 dual-port rule ("First = HTTP,
// Second = HTTPS"). A single value (e.g. "8080") returns (value, "") and the
// caller applies the existing single-port TLS logic. A comma-separated pair
// (e.g. "80,443") returns the first field as the plain-HTTP port and the second
// as the HTTPS port. Surrounding whitespace on each field is trimmed.
func SplitPorts(spec string) (httpPort, httpsPort string) {
	parts := strings.SplitN(strings.TrimSpace(spec), ",", 2)
	first := strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		return first, strings.TrimSpace(parts[1])
	}
	return first, ""
}

// randomUnusedPort picks a random port in [lo, hi] that can be bound.
func randomUnusedPort(lo, hi int) (int, error) {
	ports := rand.Perm(hi - lo + 1)
	for _, offset := range ports {
		port := lo + offset
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			continue
		}
		ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("no unused port found in %d-%d", lo, hi)
}

// ConfigManager watches a config file and hot-reloads eligible settings.
// Restart-required settings are logged as warnings but not applied.
type ConfigManager struct {
	configPath  string
	current     *Config
	lastModTime time.Time
	mu          sync.RWMutex
}

// NewConfigManager constructs a ConfigManager for configPath with the already-loaded cfg as its initial state.
func NewConfigManager(configPath string, cfg *Config) *ConfigManager {
	var modTime time.Time
	if info, err := os.Stat(configPath); err == nil {
		modTime = info.ModTime()
	}
	return &ConfigManager{
		configPath:  configPath,
		current:     cfg,
		lastModTime: modTime,
	}
}

// Get returns the current active config. Safe for concurrent use.
func (m *ConfigManager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Start launches the background polling goroutine; it runs until stop is closed.
// Optional onChange callbacks are called after each successful hot-reload with the new config.
func (m *ConfigManager) Start(stop <-chan struct{}, onChange ...func(*Config)) {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if next := m.checkFileChanges(); next != nil {
					for _, fn := range onChange {
						fn(next)
					}
				}
			case <-stop:
				return
			}
		}
	}()
}

// checkFileChanges stat-checks the config file and reloads if the modtime changed.
// Returns the new config on a successful reload, nil otherwise.
func (m *ConfigManager) checkFileChanges() *Config {
	info, err := os.Stat(m.configPath)
	if err != nil {
		return nil
	}
	if !info.ModTime().After(m.lastModTime) {
		return nil
	}

	next, err := Load(m.configPath)
	if err != nil {
		log.Printf("[config] reload error: %v", err)
		return nil
	}

	m.mu.Lock()
	prev := m.current
	m.applyHotSettings(prev, next)
	m.current = next
	m.lastModTime = info.ModTime()
	m.mu.Unlock()

	log.Printf("[config] reloaded from %s", m.configPath)
	return next
}

// applyHotSettings logs warnings for restart-required changes and applies
// hot-reloadable settings from next into next (they are always applied on swap).
// The separate prev reference allows detecting which restart-required keys changed.
func (m *ConfigManager) applyHotSettings(prev, next *Config) {
	if prev.Server.Port != next.Server.Port {
		log.Printf("[config] WARNING: server.port changed from %s to %s — restart required",
			prev.Server.Port, next.Server.Port)
	}
	if prev.Server.Address != next.Server.Address {
		log.Printf("[config] WARNING: server.address changed (%s → %s) — restart required",
			prev.Server.Address, next.Server.Address)
	}
	if prev.Database.Type != next.Database.Type || prev.Database.Path != next.Database.Path {
		log.Printf("[config] WARNING: database settings changed — restart required")
	}
	if prev.Server.Tor.Binary != next.Server.Tor.Binary ||
		prev.Server.Tor.VirtualPort != next.Server.Tor.VirtualPort {
		log.Printf("[config] WARNING: tor settings changed — restart required")
	}

	// Hot-reloadable settings are applied by replacing the entire current pointer
	// on the outer swap. Log what changed for operator visibility.
	if prev.Server.Logging.Level != next.Server.Logging.Level {
		log.Printf("[config] hot-reload: logging.level %s → %s",
			prev.Server.Logging.Level, next.Server.Logging.Level)
	}
	if prev.RateLimit.Enabled != next.RateLimit.Enabled ||
		prev.RateLimit.Read != next.RateLimit.Read ||
		prev.RateLimit.Write != next.RateLimit.Write ||
		prev.RateLimit.Health != next.RateLimit.Health ||
		prev.RateLimit.GlobalBurst != next.RateLimit.GlobalBurst {
		log.Printf("[config] hot-reload: rate_limit settings updated")
	}
	if prev.Web.Security.CORS != next.Web.Security.CORS {
		log.Printf("[config] hot-reload: cors policy updated")
	}
	if prev.Web.SiteTitle != next.Web.SiteTitle || prev.Web.Theme != next.Web.Theme {
		log.Printf("[config] hot-reload: branding settings updated")
	}
}
