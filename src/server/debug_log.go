// Per-request debug diagnostics (AI.md src/server/debug_log.go): one line per
// request written to debug.log when debug mode is active, giving operators
// method/path/status/duration/size visibility beyond the access log.
package server

import (
	"net/http"
	"strconv"
	"time"
)

// debugLog writes one diagnostic line for a completed request. Gating on
// debug mode and on server.logs.debug.enabled happens inside the manager, so
// calls are free no-ops in production.
func (s *Server) debugLog(r *http.Request, status int, duration time.Duration, size int) {
	s.logManager.Debug("request completed",
		"method", r.Method,
		"path", r.URL.Path,
		"status", strconv.Itoa(status),
		"duration", duration.String(),
		"size", strconv.Itoa(size),
		"ip", clientIP(r),
		"ua", r.UserAgent())
}
