//go:build !windows

package task

import (
	"fmt"
	"syscall"
)

// diskStats returns the free and total bytes of the filesystem containing
// path. Fields are cast explicitly because Statfs_t member types vary between
// linux, darwin, and freebsd.
func diskStats(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	bsize := uint64(st.Bsize)
	// Bavail is the space available to unprivileged users — the honest number
	// for "can this backup fit".
	return uint64(st.Bavail) * bsize, uint64(st.Blocks) * bsize, nil
}
