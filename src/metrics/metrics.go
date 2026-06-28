// Package metrics registers and exposes Prometheus metrics for the pastebin service.
// All metrics are prefixed with "pastebin_" per the project naming convention.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const ns = "pastebin"

// Registered metric variables — exported so server/middleware can observe them.
var (
	// Application info
	AppInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "app_info",
			Help:      "Application information; value is always 1, labels carry build info.",
		},
		[]string{"version", "commit", "build_date", "go_version"},
	)
	AppUptime = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "app_uptime_seconds",
		Help:      "Seconds since the application started.",
	})
	AppStartTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "app_start_timestamp",
		Help:      "Unix timestamp when the application started.",
	})

	// HTTP metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests processed.",
		},
		[]string{"method", "path", "status"},
	)
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency distribution.",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)
	HTTPRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_request_size_bytes",
			Help:      "HTTP request body size distribution.",
			Buckets:   []float64{100, 1000, 10000, 100000, 1000000, 10000000},
		},
		[]string{"method", "path"},
	)
	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_response_size_bytes",
			Help:      "HTTP response body size distribution.",
			Buckets:   []float64{100, 1000, 10000, 100000, 1000000, 10000000},
		},
		[]string{"method", "path"},
	)
	HTTPActiveRequests = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "http_active_requests",
		Help:      "Number of HTTP requests currently being processed.",
	})

	// Database metrics
	DBQueriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "db_queries_total",
			Help:      "Total number of database queries executed.",
		},
		[]string{"operation", "table"},
	)
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "db_query_duration_seconds",
			Help:      "Database query latency distribution.",
			Buckets:   []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1},
		},
		[]string{"operation", "table"},
	)
	DBConnectionsOpen = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "db_connections_open",
		Help:      "Number of open database connections in the pool.",
	})
	DBConnectionsInUse = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "db_connections_in_use",
		Help:      "Number of database connections currently in use.",
	})
	DBErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "db_errors_total",
			Help:      "Total number of database errors.",
		},
		[]string{"operation", "error_type"},
	)

	// Authentication metrics (REQUIRED per PART 20).
	AuthAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "auth_attempts_total",
			Help:      "Total authentication attempts.",
		},
		[]string{"method", "status"},
	)
	AuthSessionsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "auth_sessions_active",
		Help:      "Number of active authenticated sessions.",
	})
	APITokensActive = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "api_tokens_active",
		Help:      "Number of active (non-revoked, non-expired) API tokens.",
	})

	// Cache metrics (PART 20; project uses a cache subsystem).
	CacheHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "cache_hits_total",
			Help:      "Total cache hits.",
		},
		[]string{"cache"},
	)
	CacheMissesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "cache_misses_total",
			Help:      "Total cache misses.",
		},
		[]string{"cache"},
	)
	CacheEvictionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "cache_evictions_total",
			Help:      "Total cache evictions.",
		},
		[]string{"cache"},
	)
	CacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "cache_size",
			Help:      "Current number of items in the cache.",
		},
		[]string{"cache"},
	)
	CacheBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "cache_bytes",
			Help:      "Estimated bytes used by the cache backend.",
		},
		[]string{"cache"},
	)

	// Scheduler metrics (PART 20; project uses the built-in scheduler, PART 18).
	SchedulerTasksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "scheduler_tasks_total",
			Help:      "Total scheduled task executions.",
		},
		[]string{"task", "status"},
	)
	SchedulerTaskDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "scheduler_task_duration_seconds",
			Help:      "Scheduled task execution duration distribution.",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"task"},
	)
	SchedulerTasksRunning = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "scheduler_tasks_running",
			Help:      "Number of scheduled tasks currently running, by task name.",
		},
		[]string{"task"},
	)
	SchedulerLastRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: ns,
			Name:      "scheduler_last_run_timestamp",
			Help:      "Unix timestamp of the last run for each scheduled task.",
		},
		[]string{"task"},
	)

	// Rate limiting
	RateLimitRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "ratelimit_requests_total",
			Help:      "Total rate-limited requests.",
		},
		[]string{"limit", "status"},
	)
	RateLimitBlockedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "ratelimit_blocked_total",
			Help:      "Requests blocked by the rate limiter.",
		},
		[]string{"limit"},
	)

	// Project-specific: paste metrics
	PastesCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "pastes_created_total",
		Help:      "Total number of pastes created.",
	})
	PastesDeletedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "pastes_deleted_total",
		Help:      "Total number of pastes deleted.",
	})
	PastesViewedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "pastes_viewed_total",
		Help:      "Total number of paste view events.",
	})

	// Go runtime (collected on scrape)
	GoGoroutines = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_goroutines",
		Help:      "Current number of goroutines.",
	})
	GoMemAllocBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_mem_alloc_bytes",
		Help:      "Bytes of heap memory currently allocated and in use.",
	})
	GoMemSysBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_mem_sys_bytes",
		Help:      "Total bytes obtained from the OS.",
	})
	GoGCRunsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "go_gc_runs_total",
		Help:      "Total number of completed GC cycles.",
	})
	GoGCPauseTotalSeconds = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "go_gc_pause_total_seconds",
		Help:      "Cumulative seconds spent in GC stop-the-world pauses.",
	})
)

// Collector bundles scrape-time collection dependencies.
type Collector struct {
	startTime        time.Time
	token            string // bearer token for access control; empty = no auth
	lastPauseTotalNs uint64 // tracks accumulated GC pause nanoseconds between scrapes
}

// New creates a Collector and seeds the static app_info gauge.
func New(version, commit, buildDate string, startTime time.Time, token string) *Collector {
	AppInfo.WithLabelValues(version, commit, buildDate, runtime.Version()).Set(1)
	AppStartTimestamp.Set(float64(startTime.Unix()))
	return &Collector{startTime: startTime, token: token}
}

// Handler returns an http.Handler that serves Prometheus metrics.
// If a bearer token is configured, the Authorization header is validated.
func (c *Collector) Handler() http.Handler {
	inner := promhttp.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Enforce bearer token auth if configured.
		if c.token != "" {
			auth := r.Header.Get("Authorization")
			// Constant-time comparison to prevent token timing attacks.
			expected := "Bearer " + c.token
			if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// Collect runtime metrics just before scrape.
		c.collectRuntime()

		inner.ServeHTTP(w, r)
	})
}

// collectRuntime updates runtime-derived gauges immediately before each scrape.
func (c *Collector) collectRuntime() {
	AppUptime.Set(time.Since(c.startTime).Seconds())

	GoGoroutines.Set(float64(runtime.NumGoroutine()))

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	GoMemAllocBytes.Set(float64(ms.Alloc))
	GoMemSysBytes.Set(float64(ms.Sys))

	// Accumulate delta GC pause time since last scrape.
	if ms.PauseTotalNs > c.lastPauseTotalNs {
		delta := ms.PauseTotalNs - c.lastPauseTotalNs
		GoGCPauseTotalSeconds.Add(float64(delta) / 1e9)
		c.lastPauseTotalNs = ms.PauseTotalNs
	}
}

// Middleware returns a chi-compatible middleware that records HTTP metrics.
func (c *Collector) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip the metrics endpoint itself to avoid self-referential noise.
			if r.URL.Path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			HTTPActiveRequests.Inc()
			defer HTTPActiveRequests.Dec()

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			path := normalizePath(r.URL.Path)
			dur := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.statusCode)

			HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(dur)
			if r.ContentLength > 0 {
				HTTPRequestSize.WithLabelValues(r.Method, path).Observe(float64(r.ContentLength))
			}
			HTTPResponseSize.WithLabelValues(r.Method, path).Observe(float64(wrapped.written))
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.written += n
	return n, err
}

// normalizePath replaces dynamic path segments (IDs, UUIDs, hashes) with ":id"
// to prevent label cardinality explosion.
func normalizePath(path string) string {
	if len(path) == 0 {
		return "/"
	}
	// Keep well-known static prefixes intact.
	staticPrefixes := []string{
		"/api/", "/server/", "/graphql", "/metrics",
		"/static/", "/.well-known/",
	}
	for _, p := range staticPrefixes {
		if len(path) >= len(p) && path[:len(p)] == p {
			// For /api/ routes keep more detail but still replace IDs.
			break
		}
	}
	// Replace segments that look like IDs: 8+ hex/alphanumeric chars.
	out := make([]byte, 0, len(path))
	i := 0
	for i < len(path) {
		if path[i] == '/' {
			out = append(out, '/')
			i++
			j := i
			for j < len(path) && path[j] != '/' {
				j++
			}
			seg := path[i:j]
			if isIDSegment(seg) {
				out = append(out, ':')
				out = append(out, 'i', 'd')
			} else {
				out = append(out, seg...)
			}
			i = j
		} else {
			i++
		}
	}
	if len(out) == 0 {
		return "/"
	}
	return string(out)
}

// isIDSegment returns true for segments that look like database IDs or UUIDs.
func isIDSegment(s string) bool {
	if len(s) < 6 {
		return false
	}
	hex := 0
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-' {
			hex++
		}
	}
	// If 90%+ hex/UUID chars and 8+ long — treat as ID.
	return float64(hex)/float64(len(s)) > 0.9 && len(s) >= 8
}
