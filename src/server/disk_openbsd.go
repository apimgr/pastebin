//go:build openbsd

package server

import (
	"os"

	"golang.org/x/sys/unix"
)

// checkDisk returns true when at least 100 MiB of free space is available.
// OpenBSD is split out from disk_unix.go because it has no statvfs at all —
// only statfs, whose struct fields are all `F_`-prefixed (F_bsize, F_bavail,
// ...), so the shared Statfs-based implementation cannot compile against it.
func (s *Server) checkDisk() bool {
	var stat unix.Statfs_t
	if err := unix.Statfs(os.TempDir(), &stat); err != nil {
		// assume ok if we can't check
		return true
	}
	// F_bavail is int64 and F_bsize is uint32 (32-bit arches) or uint32 on
	// amd64 as well, so the int64 arithmetic below is safe on every arch.
	free := int64(stat.F_bavail) * int64(stat.F_bsize)
	// 100 MiB
	return free > 100<<20
}
