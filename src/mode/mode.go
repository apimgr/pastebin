package mode

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/apimgr/pastebin/src/config"
)

// Mode represents the application execution mode
type Mode string

const (
	// Production mode - optimized for performance and security
	Production Mode = "production"
	// Development mode - optimized for debugging and development
	Development Mode = "development"
)

var (
	// currentMode stores the active application mode
	currentMode Mode = Production
	// debugEnabled tracks whether --debug / DEBUG=true was set (PART 6).
	debugEnabled bool
	// mu protects concurrent access to currentMode and debugEnabled
	mu sync.RWMutex
)

// Get returns the current application mode
func Get() Mode {
	mu.RLock()
	defer mu.RUnlock()
	return currentMode
}

// Set sets the application mode
// Valid values: "production", "prod", "development", "dev"
func Set(mode string) error {
	parsed, err := ParseMode(mode)
	if err != nil {
		return err
	}

	mu.Lock()
	defer mu.Unlock()
	currentMode = parsed
	return nil
}

// SetAppMode is the spec-required name for setting the application mode (PART 6, AI.md line 9009).
// Invalid mode values are silently ignored; use Set() when error handling is needed.
func SetAppMode(m string) {
	_ = Set(m)
}

// ParseMode parses a mode string into a Mode constant
// Accepts: "dev", "development", "prod", "production" (case-insensitive)
func ParseMode(s string) (Mode, error) {
	normalized := strings.ToLower(strings.TrimSpace(s))

	switch normalized {
	case "development", "dev":
		return Development, nil
	case "production", "prod":
		return Production, nil
	default:
		return "", fmt.Errorf("invalid mode: %q (expected: production, prod, development, or dev)", s)
	}
}

// IsDevelopment returns true if the current mode is Development
func IsDevelopment() bool {
	return Get() == Development
}

// IsProduction returns true if the current mode is Production
func IsProduction() bool {
	return Get() == Production
}

// Initialize sets the mode based on priority order:
// 1. CLI flag (passed as parameter)
// 2. MODE environment variable
// 3. Default: production
func Initialize(cliMode string) error {
	// Priority 1: CLI flag
	if cliMode != "" {
		return Set(cliMode)
	}

	// Priority 2: Environment variable
	if envMode := os.Getenv("MODE"); envMode != "" {
		return Set(envMode)
	}

	// Priority 3: Default (already set to Production)
	return nil
}

// FromEnv reads MODE and DEBUG environment variables and applies them (PART 6, AI.md line 9068).
// Priority: CLI flag overrides already applied before this call take precedence.
// Reads MODE env var via Set() and DEBUG env var via config.IsTruthy().
func FromEnv() {
	if envMode := os.Getenv("MODE"); envMode != "" {
		_ = Set(envMode)
	}
	if config.IsTruthy(os.Getenv("DEBUG")) {
		SetDebugEnabled(true)
	}
}

// SetDebug enables or disables debug mode (--debug flag / DEBUG env var).
// Debug enables pprof, /debug/* endpoints, and full error detail regardless of mode.
func SetDebug(enabled bool) {
	mu.Lock()
	defer mu.Unlock()
	debugEnabled = enabled
}

// SetDebugEnabled is the spec-required alias for SetDebug (PART 6, AI.md line 9020).
func SetDebugEnabled(enabled bool) {
	SetDebug(enabled)
}

// IsDebug returns true when debug mode is active (--debug was passed or DEBUG env truthy).
func IsDebug() bool {
	mu.RLock()
	defer mu.RUnlock()
	return debugEnabled
}

// IsDebugEnabled is the spec-required alias for IsDebug (PART 6, AI.md line 9053).
func IsDebugEnabled() bool {
	return IsDebug()
}

// GetErrorDetail returns error details based on the current mode
// In development mode: returns full error details with stack traces
// In production mode: returns generic error message without internal details
func GetErrorDetail(err error) string {
	if err == nil {
		return ""
	}

	if IsDevelopment() {
		// Development mode: return full error details
		return err.Error()
	}

	// Production mode: return generic error message
	return "An internal error occurred. Please contact support if the problem persists."
}

// ShouldShowDebugEndpoints returns true when debug endpoints (/debug/pprof/*,
// /debug/vars) should be registered. Enabled only when --debug / DEBUG=true is
// set — NOT simply because the mode is development (PART 6).
func ShouldShowDebugEndpoints() bool {
	return IsDebug()
}

// CacheHeaders represents HTTP cache control headers
type CacheHeaders struct {
	CacheControl string
	Pragma       string
	Expires      string
}

// GetCacheHeaders returns appropriate cache headers based on the current mode
// Development mode: no-cache headers to prevent caching
// Production mode: aggressive caching headers for static files
func GetCacheHeaders() CacheHeaders {
	if IsDevelopment() {
		// Development mode: disable caching
		return CacheHeaders{
			CacheControl: "no-cache, no-store, must-revalidate",
			Pragma:       "no-cache",
			Expires:      "0",
		}
	}

	// Production mode: enable caching (1 year for static assets)
	return CacheHeaders{
		CacheControl: "public, max-age=31536000, immutable",
		Pragma:       "",
		Expires:      "",
	}
}

// GetLogLevel returns the recommended log level for the current mode
func GetLogLevel() string {
	if IsDevelopment() {
		return "debug"
	}
	return "info"
}

// ShouldCacheTemplates returns true if templates should be cached
func ShouldCacheTemplates() bool {
	return IsProduction()
}

// ShouldEnableAutoReload returns true if auto-reload should be enabled
func ShouldEnableAutoReload() bool {
	return IsDevelopment()
}

// ShouldEnableProfiling returns true if pprof profiling endpoints should be enabled.
// Requires --debug flag — not just development mode (PART 6).
func ShouldEnableProfiling() bool {
	return IsDebug()
}

// GetPanicRecoveryMode returns the panic recovery behavior for the current mode
// Returns "verbose" for development, "graceful" for production
func GetPanicRecoveryMode() string {
	if IsDevelopment() {
		return "verbose"
	}
	return "graceful"
}

// String returns the string representation of the Mode
func (m Mode) String() string {
	return string(m)
}

// Validate returns an error if the mode is not valid
func (m Mode) Validate() error {
	switch m {
	case Production, Development:
		return nil
	default:
		return errors.New("invalid mode")
	}
}
