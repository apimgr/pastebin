//go:build !windows

package server

import (
	"os"
	"syscall"
)

// checkDisk returns true when at least 100 MiB of free space is available.
// Uses int64 arithmetic throughout to avoid type-mismatch across platforms:
//   - Linux:   Bavail uint64, Bsize int64
//   - FreeBSD: Bavail int64,  Bsize int64
//   - Darwin:  Bavail uint32, Bsize int32
func (s *Server) checkDisk() bool {
	var stat syscall.Statfs_t
	dir := os.TempDir()
	if err := syscall.Statfs(dir, &stat); err != nil {
		// assume ok if we can't check
		return true
	}
	free := int64(stat.Bavail) * int64(stat.Bsize)
	// 100 MiB
	return free > 100<<20
}
