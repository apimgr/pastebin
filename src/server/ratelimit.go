package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
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
	rl.mu.Unlock()

	return b.allow(rl.limit, rl.window)
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
// On rejection it returns 429 with a Retry-After header.
func rateLimitMiddleware(rl *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}

			if !rl.allow(ip) {
				retryAfter := int(rl.window.Seconds())
				w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{
					"error":       "rate limit exceeded",
					"retry_after": fmt.Sprintf("%ds", retryAfter),
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
