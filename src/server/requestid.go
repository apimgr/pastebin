package server

import (
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// ctxKeyRequestID carries the per-request trace ID through the request context.
const ctxKeyRequestID ctxKeyType = 100

// RequestIDFromContext returns the request ID attached by requestIDMiddleware,
// or an empty string when the request did not pass through that middleware.
func RequestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRequestID).(string)
	return v
}

// requestIDMiddleware is execution step 2 of the canonical middleware chain
// (AI.md:7178) and must run before path security and logging so every log line
// and error response carries a trace ID. An inbound ID is reused when it is a
// well-formed UUID; anything else is replaced by a freshly generated UUID v4
// and logged as a warning (AI.md:12876-12890).
func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = r.Header.Get("X-Correlation-ID")
		}
		if reqID == "" {
			reqID = r.Header.Get("X-Trace-ID")
		}

		if reqID != "" {
			if _, err := uuid.Parse(reqID); err != nil {
				log.Printf("[request] WARNING: discarding malformed inbound request ID")
				reqID = ""
			}
		}
		if reqID == "" {
			reqID = uuid.NewString()
		}

		w.Header().Set("X-Request-ID", reqID)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
