package banner

import (
	"fmt"
	"os"
	"strings"

	"github.com/apimgr/pastebin/src/common/terminal"
)

// BannerConfig holds configuration for the startup banner
type BannerConfig struct {
	AppName string
	Version string
	AppMode string   // production/development
	Debug   bool
	URLs    []string
}

// PrintStartupBanner prints a responsive startup banner based on terminal size.
// Respects NO_COLOR environment variable.
func PrintStartupBanner(cfg BannerConfig) {
	size := terminal.GetTerminalSize()

	switch {
	case size.Mode >= terminal.SizeModeStandard:
		printStartupBannerFull(cfg, size)
	case size.Mode >= terminal.SizeModeCompact:
		printStartupBannerCompact(cfg)
	case size.Mode >= terminal.SizeModeMinimal:
		printStartupBannerMinimal(cfg)
	default:
		printStartupBannerMicro(cfg)
	}
}

// colorEnabled returns true if ANSI color output is allowed
func colorEnabled() bool {
	return os.Getenv("NO_COLOR") == ""
}

// colorize wraps text in an ANSI color code if color is enabled
func colorize(text, ansiCode string) string {
	if !colorEnabled() {
		return text
	}
	return "\033[" + ansiCode + "m" + text + "\033[0m"
}

// printStartupBannerFull prints the full banner with ASCII art (80+ cols)
func printStartupBannerFull(cfg BannerConfig, size terminal.TerminalSize) {
	width := size.Cols
	if width > 120 {
		width = 120
	}

	line := strings.Repeat("─", width-2)
	fmt.Println(colorize("┌"+line+"┐", "36"))

	// App name and version centered
	title := fmt.Sprintf(" %s v%s ", cfg.AppName, cfg.Version)
	padding := (width - 2 - len(title)) / 2
	if padding < 0 {
		padding = 0
	}
	left := strings.Repeat(" ", padding)
	right := strings.Repeat(" ", width-2-padding-len(title))
	fmt.Println(colorize("│", "36") + colorize(left+title+right, "1;36") + colorize("│", "36"))

	// Mode line
	modeStr := cfg.AppMode
	if cfg.Debug {
		modeStr += " [debug]"
	}
	modeLine := fmt.Sprintf(" Mode: %s ", modeStr)
	padding = (width - 2 - len(modeLine)) / 2
	if padding < 0 {
		padding = 0
	}
	left = strings.Repeat(" ", padding)
	right = strings.Repeat(" ", width-2-padding-len(modeLine))
	fmt.Println(colorize("│", "36") + left + colorize(modeLine, "33") + right + colorize("│", "36"))

	// Separator
	fmt.Println(colorize("├"+line+"┤", "36"))

	// URLs
	for _, url := range cfg.URLs {
		urlLine := fmt.Sprintf(" ➜  %s", url)
		padding := width - 2 - len(urlLine)
		if padding < 0 {
			padding = 0
		}
		fmt.Println(colorize("│", "36") + colorize(urlLine, "32") + strings.Repeat(" ", padding) + colorize("│", "36"))
	}

	fmt.Println(colorize("└"+line+"┘", "36"))
}

// printStartupBannerCompact prints a compact banner (60-79 cols)
func printStartupBannerCompact(cfg BannerConfig) {
	name := colorize(cfg.AppName, "1;36")
	ver := colorize("v"+cfg.Version, "36")
	mode := colorize(cfg.AppMode, "33")
	fmt.Printf("%s %s [%s]\n", name, ver, mode)
	if cfg.Debug {
		fmt.Println(colorize("  debug mode enabled", "33"))
	}
	for _, url := range cfg.URLs {
		fmt.Printf("  %s\n", colorize(url, "32"))
	}
}

// printStartupBannerMinimal prints a minimal single-line banner (40-59 cols)
func printStartupBannerMinimal(cfg BannerConfig) {
	name := colorize(cfg.AppName, "1;36")
	ver := colorize("v"+cfg.Version, "36")
	fmt.Printf("%s %s\n", name, ver)
	for _, url := range cfg.URLs {
		fmt.Println(colorize("  "+url, "32"))
	}
}

// printStartupBannerMicro prints a minimal one-liner for very small terminals
func printStartupBannerMicro(cfg BannerConfig) {
	fmt.Printf("%s v%s\n", cfg.AppName, cfg.Version)
}
