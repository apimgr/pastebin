package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const orgName = "apimgr"

// isRoot returns true when the process is running as the privileged user.
// Wrapped to keep the Windows build (where Getuid is unsupported by some toolchains) happy.
func isRoot() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	return os.Geteuid() == 0
}

// GetConfigDir returns the platform-correct config directory for appName.
// See AI.md PART 4 (OS-Specific Paths) and PART 26 (Docker container paths).
func GetConfigDir(appName string) string {
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		return dir
	}

	// Container: spec PART 26 says /config/{project_name}/
	if isContainer() {
		return filepath.Join("/config", appName)
	}

	if isRoot() {
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

// GetDataDir returns the platform-correct data directory for appName.
func GetDataDir(appName string) string {
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}

	// Container: spec PART 26 says /data/{project_name}/
	if isContainer() {
		return filepath.Join("/data", appName)
	}

	if isRoot() {
		return filepath.Join("/var/lib", orgName, appName)
	}

	switch runtime.GOOS {
	case "darwin":
		// PART 4 macOS user: ~/Library/Application Support/{project_org}/{internal_name}/
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", orgName, appName)
	case "windows":
		// PART 4 Windows user: %LocalAppData%\{project_org}\{internal_name}\
		return filepath.Join(os.Getenv("LOCALAPPDATA"), orgName, appName)
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, orgName, appName)
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", orgName, appName)
	}
}

// GetBackupDir returns the platform-correct backup directory for appName.
// PART 4 / PART 5: prefer system-wide backup path when privileged, user-local otherwise.
func GetBackupDir(appName string) string {
	if dir := os.Getenv("BACKUP_DIR"); dir != "" {
		return dir
	}

	// Container: PART 26 says /data/backups/{project_name}/
	if isContainer() {
		return filepath.Join("/data", "backups", appName)
	}

	if isRoot() {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join("/Library", "Backups", orgName, appName)
		case "windows":
			return filepath.Join(os.Getenv("ProgramData"), "Backups", orgName, appName)
		case "freebsd", "openbsd", "netbsd":
			return filepath.Join("/var", "backups", orgName, appName)
		default:
			return filepath.Join("/mnt", "Backups", orgName, appName)
		}
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Backups", orgName, appName)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Backups", orgName, appName)
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "share", "Backups", orgName, appName)
	}
}

// GetPIDFile returns the platform-correct PID file path for appName.
// PART 4 / PART 8 — privileged installs use a system run dir; user installs
// keep the PID inside the data dir.
func GetPIDFile(appName string) string {
	if p := os.Getenv("PID_FILE"); p != "" {
		return p
	}

	if isContainer() {
		return filepath.Join(GetDataDir(appName), appName+".pid")
	}

	if isRoot() {
		switch runtime.GOOS {
		case "windows":
			return filepath.Join(os.Getenv("ProgramData"), orgName, appName, appName+".pid")
		default:
			return filepath.Join("/var", "run", orgName, appName+".pid")
		}
	}

	return filepath.Join(GetDataDir(appName), appName+".pid")
}

// GetLogsDir returns the platform-correct logs directory for appName.
func GetLogsDir(appName string) string {
	if dir := os.Getenv("LOGS_DIR"); dir != "" {
		return dir
	}

	// Container: spec PART 26 says /data/log/{project_name}/
	if isContainer() {
		return filepath.Join("/data", "log", appName)
	}

	if isRoot() {
		return filepath.Join("/var/log", orgName, appName)
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Logs", orgName, appName)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), orgName, appName, "logs")
	default:
		// PART 4 (Linux user): ~/.local/log/{project_org}/{internal_name}/
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "log", orgName, appName)
	}
}

// GetCacheDir returns the platform-correct cache directory for appName.
func GetCacheDir(appName string) string {
	if dir := os.Getenv("CACHE_DIR"); dir != "" {
		return dir
	}

	// Container: spec PART 26 says /data/{project_name}/cache/
	if isContainer() {
		return filepath.Join("/data", appName, "cache")
	}

	if isRoot() {
		return filepath.Join("/var/cache", orgName, appName)
	}

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Caches", orgName, appName)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), orgName, appName, "cache")
	default:
		if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			return filepath.Join(xdg, orgName, appName)
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".cache", orgName, appName)
	}
}

// EnsureDir creates path with sensible default permissions for the running user.
func EnsureDir(path string) error {
	perm := os.FileMode(0o700)
	if isRoot() {
		perm = 0o755
	}
	return os.MkdirAll(path, perm)
}

// IsContainer reports whether the process is running inside a Linux container.
func IsContainer() bool {
	return isContainer()
}

// isContainer reports whether the process is running inside a Linux container.
func isContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "containerd") {
			return true
		}
	}

	return false
}
