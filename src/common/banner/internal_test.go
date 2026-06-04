package banner

// Internal tests for unexported banner functions.
// These run in the same package and can call unexported helpers directly.

import (
	"testing"

	"github.com/apimgr/pastebin/src/common/terminal"
)

// TestPrintStartupBannerCompact verifies the compact banner path runs without panic.
func TestPrintStartupBannerCompact(t *testing.T) {
	cfg := BannerConfig{
		AppName: "pastebin",
		Version: "1.0.0",
		AppMode: "production",
		Debug:   false,
		URLs:    []string{"http://localhost:8080"},
	}
	printStartupBannerCompact(cfg)

	// With debug enabled
	cfg.Debug = true
	printStartupBannerCompact(cfg)
}

// TestPrintStartupBannerMinimal verifies the minimal banner path runs without panic.
func TestPrintStartupBannerMinimal(t *testing.T) {
	cfg := BannerConfig{
		AppName: "pastebin",
		Version: "1.0.0",
		AppMode: "production",
		URLs:    []string{"http://localhost:8080", "http://127.0.0.1:8080"},
	}
	printStartupBannerMinimal(cfg)
}

// TestPrintStartupBannerMicro verifies the micro banner path runs without panic.
func TestPrintStartupBannerMicro(t *testing.T) {
	cfg := BannerConfig{
		AppName: "pastebin",
		Version: "1.0.0",
	}
	printStartupBannerMicro(cfg)
}

// TestPrintStartupBannerFull_WithURLs verifies the full banner with URLs runs without panic.
func TestPrintStartupBannerFull_WithURLs(t *testing.T) {
	cfg := BannerConfig{
		AppName: "pastebin",
		Version: "1.0.0",
		AppMode: "development",
		Debug:   true,
		URLs:    []string{"http://localhost:8080", "http://127.0.0.1:8080"},
	}
	size := terminal.TerminalSize{Cols: 100, Rows: 30, Mode: terminal.SizeModeStandard}
	printStartupBannerFull(cfg, size)
}

// TestPrintStartupBannerFull_NarrowWidth verifies the full banner at width=40 (padding clamped to 0).
func TestPrintStartupBannerFull_NarrowWidth(t *testing.T) {
	cfg := BannerConfig{
		AppName: "pastebin",
		Version: "1.0.0",
		AppMode: "production",
		URLs:    []string{},
	}
	size := terminal.TerminalSize{Cols: 40, Rows: 24, Mode: terminal.SizeModeStandard}
	printStartupBannerFull(cfg, size)
}

// TestPrintStartupBannerFull_NoColor_URLArrow verifies NO_COLOR arrow rendering.
func TestPrintStartupBannerFull_NoColor_URLArrow(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cfg := BannerConfig{
		AppName: "pastebin",
		Version: "1.0.0",
		AppMode: "production",
		URLs:    []string{"http://localhost:8080"},
	}
	size := terminal.TerminalSize{Cols: 80, Rows: 24, Mode: terminal.SizeModeStandard}
	printStartupBannerFull(cfg, size)
}
