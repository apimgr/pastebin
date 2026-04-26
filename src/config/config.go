package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Database    DatabaseConfig    `yaml:"database"`
	Auth        AuthConfig        `yaml:"auth"`
	RateLimit   RateLimitConfig   `yaml:"rate-limit"`
	WebUI       WebUIConfig       `yaml:"web-ui"`
	WebRobots   WebRobotsConfig   `yaml:"web-robots"`
	WebSecurity WebSecurityConfig `yaml:"web-security"`
}

type ServerConfig struct {
	Port         string        `yaml:"port"`
	FQDN         string        `yaml:"fqdn"`
	Address      string        `yaml:"address"`
	Mode         string        `yaml:"mode"`
	UpdateBranch string        `yaml:"update_branch"`
	Metrics      MetricsConfig `yaml:"metrics"`
	Logging      LoggingConfig `yaml:"logging"`
	Admin        AdminConfig   `yaml:"admin"`
	Session      SessionConfig `yaml:"session"`
}

type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	APIToken string `yaml:"api_token"`
}

type SessionConfig struct {
	Timeout int `yaml:"timeout"`
}

type MetricsConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
}

type LoggingConfig struct {
	AccessFormat string `yaml:"access_format"`
	Level        string `yaml:"level"`
}

type DatabaseConfig struct {
	Type     string `yaml:"type"`     // sqlite, postgres, mysql
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Name     string `yaml:"name"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Path     string `yaml:"path"` // For SQLite
}

type AuthConfig struct {
	Enabled     bool          `yaml:"enabled"`
	JWTSecret   string        `yaml:"jwt_secret"`
	JWTExpiry   time.Duration `yaml:"jwt_expiry"`
	SessionName string        `yaml:"session_name"`
}

type RateLimitConfig struct {
	Enabled   bool          `yaml:"enabled"`
	WindowMS  time.Duration `yaml:"window_ms"`
	MaxReqs   int           `yaml:"max_requests"`
	BypassKey string        `yaml:"bypass_key"`
}

type WebUIConfig struct {
	Theme         string              `yaml:"theme"`
	SiteTitle     string              `yaml:"site_title"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

type NotificationsConfig struct {
	Enabled       bool     `yaml:"enabled"`
	Announcements []string `yaml:"announcements"`
}

type WebRobotsConfig struct {
	Allow []string `yaml:"allow"`
	Deny  []string `yaml:"deny"`
}

type WebSecurityConfig struct {
	Admin string `yaml:"admin"`
	CORS  string `yaml:"cors"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         "3010",
			FQDN:         "localhost",
			Address:      "0.0.0.0",
			Mode:         "production",
			UpdateBranch: "stable",
			Metrics: MetricsConfig{
				Enabled:  false,
				Endpoint: "/metrics",
			},
			Logging: LoggingConfig{
				AccessFormat: "apache",
				Level:        "info",
			},
			Admin: AdminConfig{
				Username: "admin",
				Password: "",
				APIToken: "",
			},
			Session: SessionConfig{
				Timeout: 3600,
			},
		},
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: "./data/pastebin.db",
		},
		Auth: AuthConfig{
			Enabled:     true,
			JWTSecret:   "change-me-in-production-super-secret-key",
			JWTExpiry:   7 * 24 * time.Hour,
			SessionName: "pastebin_session",
		},
		RateLimit: RateLimitConfig{
			Enabled:   true,
			WindowMS:  time.Hour,
			MaxReqs:   900,
			BypassKey: "",
		},
		WebUI: WebUIConfig{
			Theme:     "dark",
			SiteTitle: "Pastebin",
			Notifications: NotificationsConfig{
				Enabled:       true,
				Announcements: []string{},
			},
		},
		WebRobots: WebRobotsConfig{
			Allow: []string{"/", "/api"},
			Deny:  []string{"/auth"},
		},
		WebSecurity: WebSecurityConfig{
			Admin: "admin@example.com",
			CORS:  "*",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Override from environment variables
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

	// Env always overrides file
	cfg.loadEnv()

	return cfg, nil
}

func (c *Config) loadEnv() {
	if port := os.Getenv("PORT"); port != "" {
		c.Server.Port = port
	}
	if dbType := os.Getenv("DB_TYPE"); dbType != "" {
		c.Database.Type = dbType
	}
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		c.Database.Host = dbHost
	}
	if dbPort := os.Getenv("DB_PORT"); dbPort != "" {
		if p, err := strconv.Atoi(dbPort); err == nil {
			c.Database.Port = p
		}
	}
	if dbName := os.Getenv("DB_NAME"); dbName != "" {
		c.Database.Name = dbName
	}
	if dbUser := os.Getenv("DB_USER"); dbUser != "" {
		c.Database.User = dbUser
	}
	if dbPass := os.Getenv("DB_PASSWORD"); dbPass != "" {
		c.Database.Password = dbPass
	}
	if dbPath := os.Getenv("DB_PATH"); dbPath != "" {
		c.Database.Path = dbPath
	}
	if jwtSecret := os.Getenv("JWT_SECRET"); jwtSecret != "" {
		c.Auth.JWTSecret = jwtSecret
	}
	if siteTitle := os.Getenv("SITE_TITLE"); siteTitle != "" {
		c.WebUI.SiteTitle = siteTitle
	}
}

func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
