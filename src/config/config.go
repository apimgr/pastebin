package config

import (
	"os"
	"strconv"

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
	Port    string        `yaml:"port"`
	Address string        `yaml:"address"`
	FQDN    string        `yaml:"fqdn"`
	Mode    string        `yaml:"mode"`
	BaseURL string        `yaml:"base_url"` // override for URL generation
	Metrics MetricsConfig `yaml:"metrics"`
	GeoIP   GeoIPConfig   `yaml:"geoip"`
	Tor     TorConfig     `yaml:"tor"`
	Logging LoggingConfig `yaml:"logging"`
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
}

// WebConfig holds web-UI settings.
type WebConfig struct {
	SiteTitle string    `yaml:"site_title"`
	Theme     string    `yaml:"theme"` // "dark" | "light" | "auto"
	Robots    RobotsConfig `yaml:"robots"`
	Security  SecurityConfig `yaml:"security"`
}

// RobotsConfig sets robots.txt allow/deny lists.
type RobotsConfig struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

// SecurityConfig holds security.txt contact info.
type SecurityConfig struct {
	Contact string `yaml:"contact"`
	CORS    string `yaml:"cors"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    "3010",
			Address: "0.0.0.0",
			FQDN:    "localhost",
			Mode:    "production",
			Metrics: MetricsConfig{
				Enabled:  false,
				Endpoint: "/metrics",
				Token:    "",
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
		},
		Web: WebConfig{
			SiteTitle: "Pastebin",
			Theme:     "dark",
			Robots: RobotsConfig{
				Allow: []string{"/"},
				Deny:  []string{},
			},
			Security: SecurityConfig{
				Contact: "mailto:admin@example.com",
				CORS:    "*",
			},
		},
	}
}

// Load reads config from path, creating it with defaults if absent.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.loadEnv()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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
	return cfg, nil
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
}

// Save writes config to path.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}
