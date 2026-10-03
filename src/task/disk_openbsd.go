//go:build openbsd

package task

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// diskStats returns the free and total bytes of the filesystem containing
// path. OpenBSD is split out from disk_unix.go because it has no statvfs at
// all — only statfs, whose struct fields are all `F_`-prefixed (F_bsize,
// F_bavail, ...), so the shared Statfs-based implementation cannot compile
// against it. F_bavail is signed (it can go negative when the filesystem is
// overcommitted), so it is clamped at zero rather than wrapped into a huge
// uint64.
func diskStats(path string) (free, total uint64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %s: %w", path, err)
	}
	var bavail uint64
	if st.F_bavail > 0 {
		bavail = uint64(st.F_bavail)
	}
	return bavail * uint64(st.F_bsize), st.F_blocks * uint64(st.F_bsize), nil
}
