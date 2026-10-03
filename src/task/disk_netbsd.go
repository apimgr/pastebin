//go:build netbsd

package task

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// diskStats returns the free and total bytes of the filesystem containing
// path. NetBSD is split out from disk_unix.go because syscall.Statfs_t there is
// declared as `[0]byte` (an opaque placeholder with no fields at all), so the
// shared Statfs-based implementation cannot compile against it. NetBSD's real
// filesystem API is statvfs, exposed by x/sys/unix as Statvfs.
func diskStats(path string) (free, total uint64, err error) {
	var st unix.Statvfs_t
	if err := unix.Statvfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	// Frsize is the fragment size — the unit f_blocks/f_bfree are counted in.
	// Bavail is the space available to unprivileged users — the honest number
	// for "can this backup fit".
	return st.Bavail * st.Frsize, st.Blocks * st.Frsize, nil
}
