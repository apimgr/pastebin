package paths

import (
	"os"
	"path/filepath"
	"runtime"
)

const orgName = "apimgr"

func GetConfigDir(appName string) string {
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		return dir
	}

	if isContainer() {
		return "/config"
	}

	if os.Getuid() == 0 {
		return filepath.Join("/etc", orgName, appName)
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", orgName, appName)
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), orgName, appName)
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, orgName, appName)
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", orgName, appName)
	}
}

func GetDataDir(appName string) string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}

	if isContainer() {
		return "/data"
	}

	if os.Getuid() == 0 {
		return filepath.Join("/var/lib", orgName, appName)
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", orgName, appName, "data")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), orgName, appName, "data")
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, orgName, appName)
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", orgName, appName)
	}
}

func GetLogsDir(appName string) string {
	if dir := os.Getenv("LOGS_DIR"); dir != "" {
		return dir
	}

	if isContainer() {
		return "/logs"
	}

	if os.Getuid() == 0 {
		return filepath.Join("/var/log", orgName, appName)
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", orgName, appName)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), orgName, appName, "logs")
	default:
		if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
			return filepath.Join(xdg, orgName, appName, "logs")
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "state", orgName, appName, "logs")
	}
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func isContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if len(content) > 0 && (contains(content, "docker") || contains(content, "kubepods") || contains(content, "containerd")) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
