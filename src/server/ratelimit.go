package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/apimgr/pastebin/src/metrics"
)

// ipBucket tracks request counts for a single IP within a sliding window.
type ipBucket struct {
	mu       sync.Mutex
	requests []time.Time
}

// allow reports whether a new request is permitted under the given limit per window.
func (b *ipBucket) allow(limit int, window time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-window)

	// Drop expired entries.
	valid := b.requests[:0]
	for _, t := range b.requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	b.requests = valid

	if len(b.requests) >= limit {
		return false
	}
	b.requests = append(b.requests, time.Now())
	return true
}

// rateLimiter is a thread-safe in-memory per-IP rate limiter.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	limit   int
	window  time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*ipBucket),
		limit:   limit,
		window:  window,
	}
	go rl.gc()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{}
		rl.buckets[ip] = b
	}
	limit := rl.limit
	rl.mu.Unlock()

	return b.allow(limit, rl.window)
}

// UpdateLimit replaces the per-window request limit without restarting the limiter.
// Existing buckets are not flushed; the new limit takes effect on the next request.
func (rl *rateLimiter) UpdateLimit(newLimit int) {
	rl.mu.Lock()
	rl.limit = newLimit
	rl.mu.Unlock()
}

// gc periodically removes IP buckets that have no recent requests.
func (rl *rateLimiter) gc() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-rl.window)
		rl.mu.Lock()
		for ip, b := range rl.buckets {
			b.mu.Lock()
			active := false
			for _, t := range b.requests {
				if t.After(cutoff) {
					active = true
					break
				}
			}
			b.mu.Unlock()
			if !active {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitMiddleware returns a middleware that enforces the given rate limiter.
// On rejection it returns 429 with a Retry-After header. endpointClass labels
// the rate_limit_* metrics (e.g. "create", "delete"); per-IP detail is never
// used as a metric label (unbounded cardinality — see PART 20).
func rateLimitMiddleware(rl *rateLimiter, endpointClass string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allowlisted clients bypass rate limiting (PART 5 middleware order).
			if isAllowlisted(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}

			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}

			metrics.RateLimitHits.WithLabelValues(endpointClass).Inc()

			if !rl.allow(ip) {
				metrics.RateLimitBlocks.WithLabelValues(endpointClass).Inc()
				metrics.RateLimitRequestsTotal.WithLabelValues("per_ip", "limited").Inc()
				metrics.RateLimitBlockedTotal.WithLabelValues("per_ip").Inc()

				retryAfter := int(rl.window.Seconds())
				w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				// Canonical envelope per PART 14/15 (AI.md:15353).
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ok":          false,
					"error":       "RATE_LIMITED",
					"message":     "Too many requests",
					"retry_after": retryAfter,
				})
				return
			}
			metrics.RateLimitRequestsTotal.WithLabelValues("per_ip", "allowed").Inc()
			next.ServeHTTP(w, r)
		})
	}
}
