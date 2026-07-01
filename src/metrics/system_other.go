//go:build !linux

package metrics

// systemStats holds a single sample of system resource usage.
type systemStats struct {
	memUsed   uint64
	memTotal  uint64
	cpuIdle   uint64
	cpuTotal  uint64
	diskUsed  uint64
	diskTotal uint64
}

// systemStatsSupported reports whether system metrics can be collected.
// System metrics require /proc and are only implemented on Linux.
func systemStatsSupported() bool { return false }

// readSystemStats is unavailable on non-Linux platforms.
func readSystemStats(dataDir string) (systemStats, bool) {
	return systemStats{}, false
}
