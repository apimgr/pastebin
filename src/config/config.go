package config

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

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
	Port          string              `yaml:"port"`
	Address       string              `yaml:"address"`
	FQDN          string              `yaml:"fqdn"`
	Mode          string              `yaml:"mode"`
	// APIVersion is the API route prefix segment (default v1): /api/{api_version}/.
	APIVersion    string              `yaml:"api_version"`
	BaseURL       string              `yaml:"base_url"` // override for URL generation
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
	Token         string              `yaml:"token"`
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
	// UpdateBranch controls which release channel is used for self-updates (PART 22).
	// Accepted values: "stable" (default), "beta", "daily".
	UpdateBranch string `yaml:"update_branch"`
	// Cache configures the in-process or remote cache driver (PART 9/12).
	Cache CacheConfig `yaml:"cache"`
	// Limits controls HTTP server request and body limits (PART 12).
	Limits LimitsConfig `yaml:"limits"`
	// TrustedProxies lists additional proxy IPs/CIDRs beyond the private ranges
	// that are always trusted (loopback, RFC 1918, etc.) (PART 12).
	TrustedProxies TrustedProxiesConfig `yaml:"trusted_proxies"`
	// Termbin configures the raw-TCP termbin/fiche compatibility listener.
	Termbin TermbinConfig `yaml:"termbin"`
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
	MaxBodySize   int64  `yaml:"max_body_size"`
	ReadTimeout   string `yaml:"read_timeout"`  // e.g. "30s"
	WriteTimeout  string `yaml:"write_timeout"` // e.g. "30s"
	IdleTimeout   string `yaml:"idle_timeout"`  // e.g. "120s"
}

// TrustedProxiesConfig lists additional proxy IPs/CIDRs/DNS names beyond the
// private ranges that are always trusted (PART 12).
// Private ranges (127/8, 10/8, 172.16/12, 192.168/16, fc00::/7, etc.) are
// always trusted without config. Add public proxy IPs here only.
type TrustedProxiesConfig struct {
	Additional []string `yaml:"additional"`
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
	TLS           bool   `yaml:"tls"`
	TLSSkipVerify bool   `yaml:"tls_skip_verify"`
	PoolSize      int    `yaml:"pool_size"`
	MinIdle       int    `yaml:"min_idle"`
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
	MaxStreamsPerCircuit       int  `yaml:"max_streams_per_circuit"`
	CloseCircuitOnStreamLimit bool `yaml:"close_circuit_on_stream_limit"`

	// Bandwidth.
	BandwidthRate       string `yaml:"bandwidth_rate"`
	BandwidthBurst      string `yaml:"bandwidth_burst"`
	MaxMonthlyBandwidth string `yaml:"max_monthly_bandwidth"`

	// Hidden service.
	NumIntroPoints int `yaml:"num_intro_points"`
	VirtualPort    int `yaml:"virtual_port"`
}

// GeoIPConfig configures GeoIP detection and country blocking.
type GeoIPConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Dir            string            `yaml:"dir"` // path to MMDB database directory
	DenyCountries  []string          `yaml:"deny_countries"`
	AllowCountries []string          `yaml:"allow_countries"`
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
	Token    string `yaml:"token"`
	// AllowedIPs lists additional IPs or CIDRs that may reach /metrics (PART 20).
	// Loopback (127.0.0.1, ::1) is always allowed. Non-loopback requests from IPs
	// not in this list are rejected with 403 before the bearer-token check.
	AllowedIPs []string `yaml:"allowed_ips"`
}

// LoggingConfig controls access log format and log level.
type LoggingConfig struct {
	AccessFormat string `yaml:"access_format"`
	Level        string `yaml:"level"`
}

// DatabaseConfig selects and configures the storage backend.
type DatabaseConfig struct {
	Type string `yaml:"type"` // only "sqlite" for now
	Path string `yaml:"path"` // path to the SQLite database file
}

// PasteConfig controls paste-specific behaviour.
type PasteConfig struct {
	MaxSizeBytes    int64  `yaml:"max_size_bytes"`    // max paste size (default 10 MiB)
	DefaultExpiry   string `yaml:"default_expiry"`    // "never" or expiry code
	DefaultLanguage string `yaml:"default_language"`  // "text"
	MaxBurnAfter    int    `yaml:"max_burn_after"`    // cap on burn_after (default 9999)
	AllowUnlisted   bool   `yaml:"allow_unlisted"`    // allow unlisted pastes (default true)
}

// RateLimitConfig controls request throttling.
type RateLimitConfig struct {
	Enabled    bool `yaml:"enabled"`
	CreatePerM int  `yaml:"create_per_minute"` // paste creates per IP per minute
	ReadPerM   int  `yaml:"read_per_minute"`   // paste reads per IP per minute
	DeletePerM int  `yaml:"delete_per_minute"` // paste deletes per IP per minute
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
	Enabled     bool       `yaml:"enabled"`
	SMTP        SMTPConfig `yaml:"smtp"`
	From        EmailFrom  `yaml:"from"`
	ReplyTo     string     `yaml:"reply_to"`
	TemplateDir string     `yaml:"template_dir"` // custom override dir; empty = use embedded defaults
}

// SMTPConfig holds connection settings for the outbound SMTP server.
type SMTPConfig struct {
	// Host is the SMTP server hostname or IP. Empty = auto-detect on first run.
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`     // default 587
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

// SecurityConfig holds security.txt contact info and server-wide encryption keys.
type SecurityConfig struct {
	Contact string `yaml:"contact"`
	CORS    string `yaml:"cors"`
	// EncryptionKey is the 32-byte AES-256-GCM key (hex-encoded) used for at-rest
	// encryption of sensitive server data (DNS credentials, security reports, etc.).
	// Auto-generated on first run; stored in server.yml; included in every backup.
	EncryptionKey string `yaml:"encryption_key"`
	// Allowlist is a list of IP addresses or CIDR ranges that bypass blocklist,
	// rate limiting, and geoip checks. Auth checks are never bypassed.
	// Single IPs are automatically expanded to /32 (IPv4) or /128 (IPv6).
	Allowlist []string `yaml:"allowlist"`
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
type BrandingConfig struct {
	Title       string `yaml:"title"`
	Tagline     string `yaml:"tagline"`
	Description string `yaml:"description"`
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

// SchedulerConfig configures the built-in task scheduler (PART 12/18).
// Tasks maps a task name to its per-task settings; absent entries use code defaults.
type SchedulerConfig struct {
	Enabled bool                       `yaml:"enabled"`
	Tasks   map[string]SchedulerTask   `yaml:"tasks"`
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
	MaxAgeSeconds     int64 `yaml:"max_age_seconds"`   // default 63072000 (2 years)
	IncludeSubdomains bool  `yaml:"include_subdomains"` // default true
	Preload           bool  `yaml:"preload"`            // default true
}

// CSPConfig controls Content-Security-Policy emission.
type CSPConfig struct {
	Enabled          bool   `yaml:"enabled"`
	Mode             string `yaml:"mode"`               // enforce | report-only
	ScriptSrcExtra   string `yaml:"script_src_extra"`
	StyleSrcExtra    string `yaml:"style_src_extra"`
	ImgSrcExtra      string `yaml:"img_src_extra"`
	FontSrcExtra     string `yaml:"font_src_extra"`
	ConnectSrcExtra  string `yaml:"connect_src_extra"`
	FrameSrcExtra    string `yaml:"frame_src_extra"`
	FormActionExtra  string `yaml:"form_action_extra"`
	ScriptSrcOverride string `yaml:"script_src_override"`
}

// DefaultConfig returns a config with sensible defaults.
// Server.Port is intentionally empty so that Load + ResolvePort can apply
// the "random 64xxx on first run" rule described in PART 5.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:       "",
			Address:    "0.0.0.0",
			FQDN:       "localhost",
			Mode:       "production",
			APIVersion: "v1",
			Branding: BrandingConfig{
				Title: "Pastebin",
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
				Enabled:    false,
				Endpoint:   "/metrics",
				Token:      "",
				AllowedIPs: []string{},
			},
			GeoIP: GeoIPConfig{
				Enabled:        false,
				Dir:            "", // resolved at startup to {data_dir}/security/geoip
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
				MaxStreamsPerCircuit:       100,
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
			},
			Notifications: NotificationsConfig{
				Email: EmailConfig{
					Enabled: false,
					SMTP: SMTPConfig{
						Host: "",
						Port: 587,
						TLS:  "auto",
					},
				},
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
				MaxBodySize:  10 << 20, // 10 MiB
				ReadTimeout:  "30s",
				WriteTimeout: "30s",
				IdleTimeout:  "120s",
			},
			TrustedProxies: TrustedProxiesConfig{
				Additional: []string{},
			},
			Termbin: TermbinConfig{
				Enabled: false,
				Port:    9999,
				MaxSize: 32768,
				Timeout: "5s",
			},
		},
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: "", // resolved at startup relative to data dir
		},
		Paste: PasteConfig{
			MaxSizeBytes:    10 << 20, // 10 MiB
			DefaultExpiry:   "never",
			DefaultLanguage: "text",
			MaxBurnAfter:    9999,
			AllowUnlisted:   true,
		},
		RateLimit: RateLimitConfig{
			Enabled:    true,
			CreatePerM: 10,
			ReadPerM:   120,
			DeletePerM: 10,
		},
		Web: WebConfig{
			SiteTitle: "Pastebin",
			Theme:     "dark",
			Robots: RobotsConfig{
				Allow: []string{"/"},
				Deny:  []string{},
			},
			Security: SecurityConfig{
				Contact:       "mailto:admin@example.com",
				CORS:          "*",
				EncryptionKey: "", // auto-generated on first run
			},
			HSTS: HSTSConfig{
				Enabled:           true,
				MaxAgeSeconds:     63072000, // 2 years (preload-list eligible)
				IncludeSubdomains: true,
				Preload:           true,
			},
			CSP: CSPConfig{
				Enabled: true,
				Mode:    "enforce",
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
			_ = Save(path, cfg)
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

func (c *Config) loadEnv() {
	if v := os.Getenv("PORT"); v != "" {
		c.Server.Port = v
	}
	if v := os.Getenv("ADDRESS"); v != "" {
		c.Server.Address = v
	}
	if v := os.Getenv("BASE_URL"); v != "" {
		c.Server.BaseURL = v
	}
	if v := os.Getenv("DB_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("SITE_TITLE"); v != "" {
		c.Web.SiteTitle = v
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

	// Web theme must be one of the valid values.
	switch cfg.Web.Theme {
	case "dark", "light", "auto":
	default:
		log.Printf("[config] WARNING: invalid web.theme %q, using default \"dark\"", cfg.Web.Theme)
		cfg.Web.Theme = "dark"
	}

	// Rate limit counts must be non-negative.
	if cfg.RateLimit.CreatePerM < 0 {
		log.Printf("[config] WARNING: rate_limit.create_per_minute < 0, using default %d",
			d.RateLimit.CreatePerM)
		cfg.RateLimit.CreatePerM = d.RateLimit.CreatePerM
	}
	if cfg.RateLimit.ReadPerM < 0 {
		log.Printf("[config] WARNING: rate_limit.read_per_minute < 0, using default %d",
			d.RateLimit.ReadPerM)
		cfg.RateLimit.ReadPerM = d.RateLimit.ReadPerM
	}
	if cfg.RateLimit.DeletePerM < 0 {
		log.Printf("[config] WARNING: rate_limit.delete_per_minute < 0, using default %d",
			d.RateLimit.DeletePerM)
		cfg.RateLimit.DeletePerM = d.RateLimit.DeletePerM
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
}

// Save writes config to path.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

// ResolvePort finalises cfg.Server.Port according to the PART 5 rules:
//
//   - Container environment → "80" (always, regardless of other settings)
//   - Explicit --port flag / $PORT env / config file value → use as-is
//   - No port configured (empty) → pick a random unused port in 64000-64999,
//     persist it to cfgPath so subsequent restarts use the same port
//
// The caller must apply CLI flag overrides to cfg BEFORE calling this function
// so that an explicit --port value takes precedence over the persisted value.
// cfgPath is the path used to save the selected random port.
func ResolvePort(cfgPath string, cfg *Config, inContainer bool) error {
	if inContainer {
		cfg.Server.Port = "80"
		return nil
	}
	if cfg.Server.Port != "" {
		return nil
	}

	// First run: no port configured — pick a random unused port in 64000-64999.
	port, err := randomUnusedPort(64000, 64999)
	if err != nil {
		return fmt.Errorf("port allocator: %w", err)
	}
	cfg.Server.Port = strconv.Itoa(port)

	// Persist so subsequent restarts use the same port.
	if saveErr := Save(cfgPath, cfg); saveErr != nil {
		// Non-fatal: log at the call site; the server can still start.
		return fmt.Errorf("port allocator: could not persist port to %s: %w", cfgPath, saveErr)
	}
	return nil
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
		prev.RateLimit.CreatePerM != next.RateLimit.CreatePerM ||
		prev.RateLimit.ReadPerM != next.RateLimit.ReadPerM ||
		prev.RateLimit.DeletePerM != next.RateLimit.DeletePerM {
		log.Printf("[config] hot-reload: rate_limit settings updated")
	}
	if prev.Web.Security.CORS != next.Web.Security.CORS {
		log.Printf("[config] hot-reload: cors policy updated")
	}
	if prev.Web.SiteTitle != next.Web.SiteTitle || prev.Web.Theme != next.Web.Theme {
		log.Printf("[config] hot-reload: branding settings updated")
	}
}


