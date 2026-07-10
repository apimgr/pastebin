// Package metrics registers and exposes Prometheus metrics for the pastebin service.
// All metrics are prefixed with "pastebin_" per the project naming convention.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const ns = "pastebin"

// Default histogram buckets, used when the config supplies no override.
var (
	defaultDurationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	defaultSizeBuckets     = []float64{100, 1000, 10000, 100000, 1000000, 10000000}
)

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

	// Rate limiting (REQUIRED per PART 20). Labelled by endpoint_class only;
	// never by ip — per-IP labels are an unbounded-cardinality memory-DoS
	// vector. Per-IP detail belongs in structured logs, not metric labels.
	RateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "rate_limit_hits_total",
			Help:      "Rate limit triggers by endpoint class (every rate-limited request evaluated).",
		},
		[]string{"endpoint_class"},
	)
	RateLimitBlocks = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: ns,
			Name:      "rate_limit_blocked_total",
			Help:      "Requests blocked by rate limit, by endpoint class.",
		},
		[]string{"endpoint_class"},
	)

	// Rate limiting (optional detail per PART 20).
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

	// Go runtime metrics (registered only when include_runtime is true).
	GoGoroutines = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_goroutines",
		Help:      "Current number of goroutines.",
	})
	GoMemAllocBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_mem_alloc_bytes",
		Help:      "Bytes of heap memory currently allocated and in use.",
	})
	GoMemSysBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_mem_sys_bytes",
		Help:      "Total bytes obtained from the OS.",
	})
	GoGCRunsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "go_gc_runs_total",
		Help:      "Total number of completed GC cycles.",
	})
	GoGCPauseTotalSeconds = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "go_gc_pause_total_seconds",
		Help:      "Cumulative seconds spent in GC stop-the-world pauses.",
	})

	// System metrics (registered only when include_system is true).
	SystemCPUUsagePct = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_cpu_usage_percent",
		Help:      "System CPU usage percentage (0-100).",
	})
	SystemMemUsagePct = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_memory_usage_percent",
		Help:      "System memory usage percentage (0-100).",
	})
	SystemMemUsedBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_memory_used_bytes",
		Help:      "System memory in use, in bytes.",
	})
	SystemMemTotalBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_memory_total_bytes",
		Help:      "Total system memory, in bytes.",
	})
	SystemDiskUsagePct = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_disk_usage_percent",
		Help:      "Disk usage percentage (0-100) for the data path.",
	}, []string{"path"})
	SystemDiskUsedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_disk_used_bytes",
		Help:      "Disk space used, in bytes, for the data path.",
	}, []string{"path"})
	SystemDiskTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "system_disk_total_bytes",
		Help:      "Total disk space, in bytes, for the data path.",
	}, []string{"path"})

	// Tor metrics (always registered; project ships Tor per PART 31).
	TorEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "tor_enabled",
		Help:      "1 when the Tor hidden service is enabled, else 0.",
	})
	TorRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "tor_running",
		Help:      "1 when the Tor hidden service is running, else 0.",
	})
	TorCircuitEstablished = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: ns,
		Name:      "tor_circuit_established",
		Help:      "1 when a Tor circuit is established, else 0.",
	})
	TorRequestsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "tor_requests_total",
		Help:      "Total requests received over the Tor hidden service (.onion Host).",
	})
)

// Collector bundles scrape-time collection dependencies.
type Collector struct {
	startTime time.Time
	// bearer token for access control; empty = no auth
	token string
	// tracks accumulated GC pause nanoseconds between scrapes
	lastPauseTotalNs uint64

	includeSystem  bool
	includeRuntime bool
	dataDir        string

	// Per-collector HTTP histograms, built from configured buckets.
	httpReqDuration *prometheus.HistogramVec
	httpReqSize     *prometheus.HistogramVec
	httpRespSize    *prometheus.HistogramVec

	// CPU sampling state for delta-based usage percentage (system metrics).
	lastCPUTotal uint64
	lastCPUIdle  uint64

	torMu    sync.Mutex
	torState func() (enabled, running bool)
}

// Options configures a Collector. Buckets default to the package defaults
// when empty; IncludeSystem/IncludeRuntime gate optional metric families.
type Options struct {
	Version         string
	Commit          string
	BuildDate       string
	Token           string
	StartTime       time.Time
	IncludeSystem   bool
	IncludeRuntime  bool
	DurationBuckets []float64
	SizeBuckets     []float64
	DataDir         string
}

// New creates a Collector with the default metric set (runtime metrics on,
// system metrics off, no Tor provider). Retained for callers and tests that
// only need the core collector.
func New(version, commit, buildDate string, startTime time.Time, token string) *Collector {
	return NewWithOptions(Options{
		Version:        version,
		Commit:         commit,
		BuildDate:      buildDate,
		Token:          token,
		StartTime:      startTime,
		IncludeRuntime: true,
	})
}

// NewWithOptions creates a Collector, seeds app_info, builds the HTTP
// histograms from the configured buckets, and conditionally registers the
// optional runtime and system metric families.
func NewWithOptions(o Options) *Collector {
	AppInfo.WithLabelValues(o.Version, o.Commit, o.BuildDate, runtime.Version()).Set(1)
	AppStartTimestamp.Set(float64(o.StartTime.Unix()))

	dur := o.DurationBuckets
	if len(dur) == 0 {
		dur = defaultDurationBuckets
	}
	sz := o.SizeBuckets
	if len(sz) == 0 {
		sz = defaultSizeBuckets
	}

	c := &Collector{
		startTime:      o.StartTime,
		token:          o.Token,
		includeSystem:  o.IncludeSystem && systemStatsSupported(),
		includeRuntime: o.IncludeRuntime,
		dataDir:        o.DataDir,
	}

	c.httpReqDuration = registerHistogramVec(prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency distribution.",
			Buckets:   dur,
		},
		[]string{"method", "path"},
	))
	c.httpReqSize = registerHistogramVec(prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_request_size_bytes",
			Help:      "HTTP request body size distribution.",
			Buckets:   sz,
		},
		[]string{"method", "path"},
	))
	c.httpRespSize = registerHistogramVec(prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: ns,
			Name:      "http_response_size_bytes",
			Help:      "HTTP response body size distribution.",
			Buckets:   sz,
		},
		[]string{"method", "path"},
	))

	if c.includeRuntime {
		register(GoGoroutines, GoMemAllocBytes, GoMemSysBytes, GoGCRunsTotal, GoGCPauseTotalSeconds)
	}
	if c.includeSystem {
		register(SystemCPUUsagePct, SystemMemUsagePct, SystemMemUsedBytes, SystemMemTotalBytes,
			SystemDiskUsagePct, SystemDiskUsedBytes, SystemDiskTotalBytes)
	}

	return c
}

// SetTorProvider wires a callback that reports Tor hidden-service state at
// scrape time. Set after the Tor manager is constructed.
func (c *Collector) SetTorProvider(fn func() (enabled, running bool)) {
	c.torMu.Lock()
	c.torState = fn
	c.torMu.Unlock()
}

// register registers collectors, ignoring AlreadyRegisteredError so that
// multiple Collector instances (e.g. across tests) do not panic.
func register(cs ...prometheus.Collector) {
	for _, col := range cs {
		if err := prometheus.Register(col); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				panic(err)
			}
		}
	}
}

// registerHistogramVec registers h and returns it, or returns the already
// registered collector when one exists (so observations reach the scraped
// instance rather than a dangling duplicate).
func registerHistogramVec(h *prometheus.HistogramVec) *prometheus.HistogramVec {
	if err := prometheus.Register(h); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			return are.ExistingCollector.(*prometheus.HistogramVec)
		}
		panic(err)
	}
	return h
}

// setBool sets a gauge to 1 when b is true, else 0.
func setBool(g prometheus.Gauge, b bool) {
	if b {
		g.Set(1)
		return
	}
	g.Set(0)
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

	if c.includeRuntime {
		GoGoroutines.Set(float64(runtime.NumGoroutine()))

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		GoMemAllocBytes.Set(float64(ms.Alloc))
		GoMemSysBytes.Set(float64(ms.Sys))
		GoGCRunsTotal.Set(float64(ms.NumGC))

		// Accumulate delta GC pause time since last scrape.
		if ms.PauseTotalNs > c.lastPauseTotalNs {
			delta := ms.PauseTotalNs - c.lastPauseTotalNs
			GoGCPauseTotalSeconds.Add(float64(delta) / 1e9)
			c.lastPauseTotalNs = ms.PauseTotalNs
		}
	}

	if c.includeSystem {
		c.collectSystem()
	}

	c.torMu.Lock()
	fn := c.torState
	c.torMu.Unlock()
	if fn != nil {
		enabled, running := fn()
		setBool(TorEnabled, enabled)
		setBool(TorRunning, running)
		// Running implies a bootstrapped circuit (Tor manager blocks on
		// bootstrap before reporting running).
		setBool(TorCircuitEstablished, running)
	}
}

// collectSystem samples CPU, memory, and disk usage into the system_* gauges.
func (c *Collector) collectSystem() {
	st, ok := readSystemStats(c.dataDir)
	if !ok {
		return
	}
	if st.memTotal > 0 {
		SystemMemTotalBytes.Set(float64(st.memTotal))
		SystemMemUsedBytes.Set(float64(st.memUsed))
		SystemMemUsagePct.Set(float64(st.memUsed) / float64(st.memTotal) * 100)
	}
	// CPU percentage is derived from the delta between consecutive scrapes.
	if c.lastCPUTotal != 0 && st.cpuTotal > c.lastCPUTotal {
		totalDelta := st.cpuTotal - c.lastCPUTotal
		idleDelta := st.cpuIdle - c.lastCPUIdle
		if totalDelta > 0 {
			SystemCPUUsagePct.Set(float64(totalDelta-idleDelta) / float64(totalDelta) * 100)
		}
	}
	c.lastCPUTotal = st.cpuTotal
	c.lastCPUIdle = st.cpuIdle
	if st.diskTotal > 0 && c.dataDir != "" {
		SystemDiskTotalBytes.WithLabelValues(c.dataDir).Set(float64(st.diskTotal))
		SystemDiskUsedBytes.WithLabelValues(c.dataDir).Set(float64(st.diskUsed))
		SystemDiskUsagePct.WithLabelValues(c.dataDir).Set(float64(st.diskUsed) / float64(st.diskTotal) * 100)
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

			// Count requests arriving over the Tor hidden service.
			if strings.Contains(strings.ToLower(r.Host), ".onion") {
				TorRequestsTotal.Inc()
			}

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			path := normalizePath(r.URL.Path)
			dur := time.Since(start).Seconds()
			status := strconv.Itoa(wrapped.statusCode)

			HTTPRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			c.httpReqDuration.WithLabelValues(r.Method, path).Observe(dur)
			if r.ContentLength > 0 {
				c.httpReqSize.WithLabelValues(r.Method, path).Observe(float64(r.ContentLength))
			}
			c.httpRespSize.WithLabelValues(r.Method, path).Observe(float64(wrapped.written))
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
