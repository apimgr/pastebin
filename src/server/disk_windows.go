//go:build windows

package server

// checkDisk returns true when free disk space is available.
// On Windows we skip the syscall check and always report ok;
// the binary is cross-compiled and not expected to run on Windows.
func (s *Server) checkDisk() bool {
	return true
}
