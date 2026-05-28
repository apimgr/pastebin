package config

import (
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
	DeletePerM int  `yaml:"delete_per_minute"` // paste deletes per IP per minute
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
// Server.Port is intentionally empty so that Load + ResolvePort can apply
// the "random 64xxx on first run" rule described in PART 5.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:    "",
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

// ParseBool converts common boolean string representations to bool.
// Accepts: "true", "1", "yes", "on" → true; "false", "0", "no", "off" → false.
// Returns an error for any unrecognised value.
func ParseBool(s string) (bool, error) {
	switch s {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("config: unrecognised boolean value %q", s)
	}
}

