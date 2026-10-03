//go:build netbsd

package server

import (
	"os"

	"golang.org/x/sys/unix"
)

// checkDisk returns true when at least 100 MiB of free space is available.
// NetBSD is split out from disk_unix.go because syscall.Statfs_t there is
// declared as `[0]byte` (an opaque placeholder with no fields at all), so the
// shared Statfs-based implementation cannot compile against it. NetBSD's real
// filesystem API is statvfs, exposed by x/sys/unix as Statvfs.
func (s *Server) checkDisk() bool {
	var stat unix.Statvfs_t
	if err := unix.Statvfs(os.TempDir(), &stat); err != nil {
		// assume ok if we can't check
		return true
	}
	// Frsize is the fragment size — the unit f_blocks/f_bfree are counted in.
	// Bavail is the space available to unprivileged users.
	free := int64(stat.Bavail) * int64(stat.Frsize)
	// 100 MiB
	return free > 100<<20
}
