package server

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	// blank import side-effect: registers pprof handlers on DefaultServeMux
	_ "net/http/pprof"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apimgr/pastebin/src/cache"
	"github.com/apimgr/pastebin/src/common/httputil"
	"github.com/apimgr/pastebin/src/common/i18n"
	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/mode"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/geoip"
	"github.com/apimgr/pastebin/src/graphql"
	"github.com/apimgr/pastebin/src/handler"
	"github.com/apimgr/pastebin/src/metrics"
	"github.com/apimgr/pastebin/src/ssl"
	"github.com/apimgr/pastebin/src/swagger"
	"github.com/apimgr/pastebin/src/tor"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	qrcode "github.com/skip2/go-qrcode"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static
var staticFS embed.FS

// requestStats tracks total and per-hour request counts using a 24-bucket ring.
type requestStats struct {
	total      atomic.Int64
	activeConn atomic.Int64
	buckets    [24]atomic.Int64
	mu         sync.Mutex
	lastHour   int
}

func (rs *requestStats) inc() {
	rs.total.Add(1)
	h := time.Now().Hour()
	rs.mu.Lock()
	if h != rs.lastHour {
		// Zero out stale buckets between lastHour and h.
		for cur := (rs.lastHour + 1) % 24; cur != (h+1)%24; cur = (cur + 1) % 24 {
			rs.buckets[cur].Store(0)
		}
		rs.lastHour = h
	}
	rs.mu.Unlock()
	rs.buckets[h].Add(1)
}

func (rs *requestStats) last24h() int64 {
	var sum int64
	for i := 0; i < 24; i++ {
		sum += rs.buckets[i].Load()
	}
	return sum
}

// ─── Health types (PART 13) ──────────────────────────────────────────────────

// HealthResponse is the canonical /server/healthz response structure (PART 13).
type HealthResponse struct {
	Project        ProjectInfo  `json:"project"`
	Status         string       `json:"status"`
	PendingRestart bool         `json:"pending_restart,omitempty"`
	RestartReason  []string     `json:"restart_reason,omitempty"`
	Version        string       `json:"version"`
	GoVersion      string       `json:"go_version"`
	Build          BuildInfo    `json:"build"`
	Uptime         string       `json:"uptime"`
	Mode           string       `json:"mode"`
	Timestamp      time.Time    `json:"timestamp"`
	Features       FeaturesInfo `json:"features"`
	Checks         ChecksInfo   `json:"checks"`
	Stats          StatsInfo    `json:"stats"`
}

// ProjectInfo holds public branding fields.
type ProjectInfo struct {
	Name        string `json:"name"`
	Tagline     string `json:"tagline"`
	Description string `json:"description"`
}

// BuildInfo holds build-time metadata.
type BuildInfo struct {
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

// FeaturesInfo reports which optional features are active.
type FeaturesInfo struct {
	Tor   TorInfo `json:"tor"`
	GeoIP bool    `json:"geoip"`
}

// TorInfo reports Tor hidden service status.
type TorInfo struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Status   string `json:"status"`
	Hostname string `json:"hostname"`
}

// ChecksInfo reports component health as "ok" or "error".
type ChecksInfo struct {
	Database  string `json:"database"`
	Cache     string `json:"cache"`
	Disk      string `json:"disk"`
	Scheduler string `json:"scheduler"`
	Tor       string `json:"tor,omitempty"`
}

// StatsInfo holds public-safe aggregate statistics.
type StatsInfo struct {
	RequestsTotal  int64 `json:"requests_total"`
	Requests24h    int64 `json:"requests_24h"`
	ActiveConns    int   `json:"active_connections"`
	PastesTotal    int64 `json:"pastes_total"`
}

// SchedulerAPI is the interface the server uses to interact with the scheduler.
// It is satisfied by *scheduler.Scheduler and can be set via SetSchedulerAPI.
type SchedulerAPI interface {
	GetTasks() []database.TaskState
	GetTask(id string) (database.TaskState, bool)
	RunNow(id string) error
	EnableTask(id string)
	DisableTask(id string)
}

// Server owns the HTTP router and all handler dependencies.
type Server struct {
	router           *chi.Mux
	db               database.DB
	cacheStore       cache.Cache
	cfg              *config.Config
	cfgMgr           *config.ConfigManager
	templates        *template.Template
	pasteHandler     *handler.PasteHandler
	compatHandler    *handler.CompatHandler
	swaggerHandler   *swagger.Handler
	graphqlHandler   *graphql.Handler
	metricsCollector *metrics.Collector
	geoipDB          *geoip.DB
	torManager       *tor.Manager
	createLimiter    *rateLimiter
	deleteLimiter    *rateLimiter
	version          string
	commitID         string
	buildDate        string
	configDir        string
	startTime        time.Time
	stats            requestStats
	// schedHealthFn is an optional callback that reports whether the scheduler
	// is running — set by main after constructing the server.
	schedHealthFn func() bool
	// schedulerAPI provides runtime access to the scheduler for the API handlers.
	schedulerAPI SchedulerAPI
	// operatorTokenHash is SHA-256(server.token), cached at construction time.
	// Constant-time compared against incoming Bearer tokens on protected routes.
	operatorTokenHash [32]byte
	// pendingRestartMu guards pendingRestartKeys.
	pendingRestartMu   sync.Mutex
	pendingRestartKeys []string
	// csrfSecret is the HMAC key for CSRF token signing, loaded from the DB at startup.
	csrfSecret []byte
}

// SetSchedulerHealthFn registers a callback that reports whether the scheduler is running.
// Call this from main after constructing the server, before calling Run.
func (s *Server) SetSchedulerHealthFn(fn func() bool) {
	s.schedHealthFn = fn
}

// SetSchedulerAPI wires the scheduler into the server so scheduler API routes can
// delegate to it. Call this from main after constructing both the server and the
// scheduler, before calling Run.
func (s *Server) SetSchedulerAPI(api SchedulerAPI) {
	s.schedulerAPI = api
}

// MarkPendingRestart records a config key that requires a restart to take effect.
// The key is surfaced in the healthz pending_restart / restart_reason fields.
func (s *Server) MarkPendingRestart(key string) {
	s.pendingRestartMu.Lock()
	defer s.pendingRestartMu.Unlock()
	for _, k := range s.pendingRestartKeys {
		if k == key {
			return
		}
	}
	s.pendingRestartKeys = append(s.pendingRestartKeys, key)
}

// liveCfg returns the most current config, applying hot-reloaded values if available.
func (s *Server) liveCfg() *config.Config {
	if s.cfgMgr != nil {
		return s.cfgMgr.Get()
	}
	return s.cfg
}

// New constructs a Server and wires all routes.
// cfgMgr may be nil (e.g. in tests); when set, hot-reloadable settings are read live.
func New(db database.DB, cfg *config.Config, cfgMgr *config.ConfigManager, version, commitID, buildDate, configDir, dataDir string) *Server {
	s := &Server{
		router:    chi.NewRouter(),
		db:        db,
		cfg:       cfg,
		cfgMgr:    cfgMgr,
		version:   version,
		commitID:  commitID,
		buildDate: buildDate,
		configDir: configDir,
		startTime: time.Now(),
	}
	s.stats.lastHour = time.Now().Hour()

	// Cache SHA-256(server.token) early so it can be passed to handlers below.
	// If the token is empty, operatorTokenHash remains zero — protected routes return 401.
	if cfg.Server.Token != "" {
		s.operatorTokenHash = sha256.Sum256([]byte(cfg.Server.Token))
	}

	s.pasteHandler = handler.NewPasteHandler(db, cfg.Server.BaseURL, s.operatorTokenHash)
	s.compatHandler = handler.NewCompatHandler(s.pasteHandler, db, version)
	s.swaggerHandler = swagger.New(cfg.Web.SiteTitle+" API", version, cfg.Server.BaseURL)
	s.graphqlHandler = graphql.New(db, cfg.Web.SiteTitle)
	s.metricsCollector = metrics.New(version, commitID, buildDate, s.startTime, cfg.Server.Metrics.Token)

	if cfg.Server.GeoIP.Enabled {
		gcfg := geoip.Config{
			Dir:            cfg.Server.GeoIP.Dir,
			EnableASN:      cfg.Server.GeoIP.Databases.ASN,
			EnableCountry:  cfg.Server.GeoIP.Databases.Country,
			EnableCity:     cfg.Server.GeoIP.Databases.City,
			EnableWHOIS:    cfg.Server.GeoIP.Databases.WHOIS,
			DenyCountries:  cfg.Server.GeoIP.DenyCountries,
			AllowCountries: cfg.Server.GeoIP.AllowCountries,
			// Wire server-wide security allowlist into geoip so GeoIP also
			// bypasses country-blocking for explicitly allowlisted IPs.
			Allowlist: cfg.Web.Security.Allowlist,
		}
		if gdb, err := geoip.Open(gcfg); err != nil {
			log.Printf("warning: geoip init: %v", err)
		} else {
			s.geoipDB = gdb
		}
	}

	// Tor hidden service — auto-enabled when Tor binary is found.
	torCfg := tor.Config{
		Binary:                    cfg.Server.Tor.Binary,
		UseNetwork:                cfg.Server.Tor.UseNetwork,
		AllowUserPreference:       cfg.Server.Tor.AllowUserPreference,
		MaxCircuits:               cfg.Server.Tor.MaxCircuits,
		CircuitTimeout:            cfg.Server.Tor.CircuitTimeout,
		BootstrapTimeout:          cfg.Server.Tor.BootstrapTimeout,
		SafeLogging:               cfg.Server.Tor.SafeLogging,
		MaxStreamsPerCircuit:       cfg.Server.Tor.MaxStreamsPerCircuit,
		CloseCircuitOnStreamLimit: cfg.Server.Tor.CloseCircuitOnStreamLimit,
		BandwidthRate:             cfg.Server.Tor.BandwidthRate,
		BandwidthBurst:            cfg.Server.Tor.BandwidthBurst,
		MaxMonthlyBandwidth:       cfg.Server.Tor.MaxMonthlyBandwidth,
		NumIntroPoints:            cfg.Server.Tor.NumIntroPoints,
		VirtualPort:               cfg.Server.Tor.VirtualPort,
		ConfigDir:                 configDir,
		DataDir:                   dataDir,
	}
	serverPort, _ := strconv.Atoi(cfg.Server.Port)
	if serverPort == 0 {
		serverPort = 3010
	}
	s.torManager = tor.NewManager(context.Background(), serverPort, torCfg)

	// Initialize cache driver (PART 9/12). Falls back to in-process memory on error.
	cacheCfg := cache.Config{
		Type:          cfg.Server.Cache.Type,
		URL:           cfg.Server.Cache.URL,
		Host:          cfg.Server.Cache.Host,
		Port:          cfg.Server.Cache.Port,
		Username:      cfg.Server.Cache.Username,
		Password:      cfg.Server.Cache.Password,
		DB:            cfg.Server.Cache.DB,
		TLS:           cfg.Server.Cache.TLS,
		TLSSkipVerify: cfg.Server.Cache.TLSSkipVerify,
		PoolSize:      cfg.Server.Cache.PoolSize,
		MinIdle:       cfg.Server.Cache.MinIdle,
		Prefix:        cfg.Server.Cache.Prefix,
	}
	if d, err := time.ParseDuration(cfg.Server.Cache.Timeout); err == nil {
		cacheCfg.Timeout = d
	}
	if d, err := time.ParseDuration(cfg.Server.Cache.TTL); err == nil {
		cacheCfg.TTL = d
	}
	if cs, err := cache.New(cacheCfg); err != nil {
		log.Printf("warning: cache init failed (%v); falling back to memory", err)
		fallback := cache.DefaultConfig()
		fallback.Prefix = cacheCfg.Prefix
		fallback.TTL = cacheCfg.TTL
		s.cacheStore, _ = cache.New(fallback)
	} else {
		s.cacheStore = cs
	}

	// Default: 30 creates per IP per minute; configurable via rate_limit.create_per_minute.
	createLimit := cfg.RateLimit.CreatePerM
	if createLimit <= 0 {
		createLimit = 30
	}
	// Default: 10 deletes per IP per minute (stricter than creates to prevent enumeration).
	deleteLimit := cfg.RateLimit.DeletePerM
	if deleteLimit <= 0 {
		deleteLimit = 10
	}
	if cfg.RateLimit.Enabled {
		s.createLimiter = newRateLimiter(createLimit, time.Minute)
		s.deleteLimiter = newRateLimiter(deleteLimit, time.Minute)
	}

	tmpl, err := template.New("").Funcs(template.FuncMap{
		"t": func(lang, key string) string {
			return i18n.Translate(lang, key)
		},
		"i18njs": func(lang string) template.JS {
			return template.JS(i18n.JSBundle(lang))
		},
	}).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Printf("warning: could not parse templates: %v", err)
	}
	s.templates = tmpl

	// Ensure all project-level secrets exist in the DB (PART 11).
	// These are generated on first start and never returned in API responses.
	for _, secretKey := range []string{"installation_secret", "cookie_signing_key", "csrf_token_secret"} {
		if _, err := db.EnsureAppSecret(secretKey); err != nil {
			log.Printf("warning: could not initialize app secret %q: %v", secretKey, err)
		}
	}
	// Cache the CSRF signing secret for use in csrfMiddleware.
	if csrfSec, err := db.EnsureAppSecret("csrf_token_secret"); err == nil {
		s.csrfSecret = csrfSec
	}

	s.setupRoutes()
	return s
}

// OnConfigChange is called by the ConfigManager after each successful hot-reload.
// It updates rate limiter thresholds and tracks restart-required key changes.
func (s *Server) OnConfigChange(next *config.Config) {
	if s.createLimiter != nil && next.RateLimit.CreatePerM > 0 {
		s.createLimiter.UpdateLimit(next.RateLimit.CreatePerM)
	}
	if s.deleteLimiter != nil && next.RateLimit.DeletePerM > 0 {
		s.deleteLimiter.UpdateLimit(next.RateLimit.DeletePerM)
	}

	// Detect restart-required changes and record them for healthz.
	prev := s.liveCfg()
	if prev.Server.Port != next.Server.Port {
		s.MarkPendingRestart("server.port")
	}
	if prev.Server.Address != next.Server.Address {
		s.MarkPendingRestart("server.address")
	}
	if prev.Database.Type != next.Database.Type || prev.Database.Path != next.Database.Path {
		s.MarkPendingRestart("database")
	}
	if prev.Server.Tor.Binary != next.Server.Tor.Binary ||
		prev.Server.Tor.VirtualPort != next.Server.Tor.VirtualPort {
		s.MarkPendingRestart("server.tor")
	}
	if prev.Server.TLS.Enabled != next.Server.TLS.Enabled ||
		prev.Server.TLS.LetsEncrypt.Email != next.Server.TLS.LetsEncrypt.Email {
		s.MarkPendingRestart("server.ssl")
	}
}

// GeoIPEnabled returns true when the GeoIP database was successfully opened.
func (s *Server) GeoIPEnabled() bool {
	return s.geoipDB != nil
}

// UpdateGeoIP downloads fresh GeoIP databases. Safe to call from a scheduler.
func (s *Server) UpdateGeoIP() error {
	if s.geoipDB == nil {
		return nil
	}
	return s.geoipDB.Update()
}

// TorRunning returns true when the Tor hidden service is active.
func (s *Server) TorRunning() bool {
	if s.torManager == nil {
		return false
	}
	return s.torManager.Running()
}

// TorOnionAddress returns the .onion address, or empty string if not running.
func (s *Server) TorOnionAddress() string {
	if s.torManager == nil {
		return ""
	}
	return s.torManager.OnionAddress()
}

// maybeRateLimit wraps h with the create rate limiter if enabled.
func (s *Server) maybeRateLimit(h http.HandlerFunc) http.HandlerFunc {
	if s.createLimiter == nil {
		return h
	}
	mw := rateLimitMiddleware(s.createLimiter)
	return mw(h).ServeHTTP
}

// maybeDeleteRateLimit wraps h with the delete rate limiter if enabled.
func (s *Server) maybeDeleteRateLimit(h http.HandlerFunc) http.HandlerFunc {
	if s.deleteLimiter == nil {
		return h
	}
	mw := rateLimitMiddleware(s.deleteLimiter)
	return mw(h).ServeHTTP
}

func (s *Server) setupRoutes() {
	r := s.router

	// Middleware execution order per PART 5:
	// RealIP (chi) — extract real client IP from trusted X-Forwarded-For headers
	// Recoverer (chi) — panic recovery
	// 1. URLNormalize — trailing slash redirect (file-extension paths exempt)
	// 2. PathSecurity — block path traversal, normalize double slashes
	// 3. SecurityHeaders + SecFetch + CORS — add response headers
	// 4. Allowlist — flag IPs that bypass blocklist/rate-limit/geoip
	// 5. Blocklist — reject blocked IPs (unless allowlisted)
	// 6. GeoIP — country blocking (honours allowlist flag)
	// 7. Logging + metrics + compression (request recording)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
	r.Use(s.noTrailingSlash)
	r.Use(s.pathSecurityMiddleware)
	r.Use(s.securityHeadersMiddleware)
	r.Use(s.secFetchMiddleware)
	r.Use(s.csrfMiddleware)
	r.Use(s.corsMiddleware)
	r.Use(s.allowlistMiddleware)
	r.Use(s.blocklistMiddleware)
	if s.geoipDB != nil {
		r.Use(s.geoipDB.Middleware())
	}
	r.Use(middleware.Logger)
	r.Use(s.countRequests)
	r.Use(s.metricsCollector.Middleware())
	// .txt extension middleware for API routes (PART 14): strips ".txt" suffix from /api/
	// paths so the router matches the canonical route, while preserving the original
	// URL in the request so GetAPIResponseFormat can detect the text-format intent.
	r.Use(s.txtExtensionMiddleware)

	// Response compression (PART 12) — compresses text/html, text/css, text/javascript,
	// application/json, and application/xml at level 5.
	r.Use(middleware.Compress(5,
		"text/html",
		"text/css",
		"text/javascript",
		"application/json",
		"application/xml",
		"text/plain",
	))

	// ── Static assets & PWA ──────────────────────────────────────────────────
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	r.Get("/manifest.json", s.handleManifest)
	r.Get("/sw.js", s.handleServiceWorker)
	r.Get("/robots.txt", s.handleRobots)
	r.Get("/static/icons/icon-180.png", s.handlePWAIcon180)
	r.Get("/static/icons/icon-192.png", s.handlePWAIcon192)
	r.Get("/static/icons/icon-512.png", s.handlePWAIcon512)
	r.Get("/security.txt", s.handleSecurity)
	r.Get("/favicon.ico", s.handleFavicon)

	// ── Metrics endpoint ─────────────────────────────────────────────────────
	if s.cfg.Server.Metrics.Enabled {
		endpoint := s.cfg.Server.Metrics.Endpoint
		if endpoint == "" {
			endpoint = "/metrics"
		}
		r.With(s.metricsIPAllowlistMiddleware).Handle(endpoint, s.metricsCollector.Handler())
	}

	// ── Server info pages ────────────────────────────────────────────────────
	r.Get("/server/about", s.handleAbout)
	r.Get("/server/help", s.handleHelp)
	r.Get("/server/privacy", s.handlePrivacy)
	r.Get("/server/terms", s.handleTerms)
	r.Get("/server/contact", s.handleContact)
	r.Get("/server/healthz", s.handleHealthz)
	// /healthz root alias — only when server.healthz.root.enabled: true (PART 13).
	if s.liveCfg().Web.Healthz.Root.Enabled {
		r.Get("/healthz", s.handleHealthz)
	}
	// PWA offline fallback page — referenced by service worker cache
	r.Get("/offline", s.handleOffline)

	// ── Debug endpoints (only when --debug flag is active) (PART 6) ──────────
	if mode.ShouldShowDebugEndpoints() {
		r.Mount("/debug/pprof", http.DefaultServeMux)
		r.Get("/debug/vars", expvar.Handler().ServeHTTP)
		log.Printf("debug: /debug/pprof and /debug/vars endpoints enabled")
	}

	// ── Auth stubs (no user accounts — redirect to home) ─────────────────────
	for _, path := range []string{
		"/login", "/register", "/logout", "/settings",
		"/server/auth/login", "/server/auth/register",
		"/server/auth/logout", "/server/auth/settings",
	} {
		r.Get(path, handler.AuthStubRedirect)
		r.Post(path, handler.AuthStubRedirect)
	}
	// microbin auth-gate redirects
	// user profiles → home
	r.Get("/u/{username}", handler.AuthStubRedirect)
	r.Get("/auth/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/"+chi.URLParam(r, "id"), http.StatusFound)
	})
	r.Get("/auth_raw/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/raw/"+chi.URLParam(r, "id"), http.StatusFound)
	})
	r.Get("/auth_remove_private/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/remove/"+chi.URLParam(r, "id"), http.StatusFound)
	})

	// ── pastebin.com API compatibility ───────────────────────────────────────
	r.Post("/api/api_post.php", s.maybeRateLimit(s.compatHandler.PastebinPost))
	r.Get("/api/api_raw.php", s.compatHandler.PastebinRaw)
	r.Post("/api/api_login.php", s.compatHandler.PastebinLogin)

	// ── lenpaste API compatibility ───────────────────────────────────────────
	r.Post("/api/new", s.maybeRateLimit(s.compatHandler.LenCreate))
	r.Get("/api/get", s.compatHandler.LenGet)
	r.Delete("/api/remove", s.maybeDeleteRateLimit(s.compatHandler.LenRemove))
	// some clients use GET
	r.Get("/api/remove", s.maybeDeleteRateLimit(s.compatHandler.LenRemove))
	r.Get("/api/list", s.compatHandler.LenList)

	// ── Versioned API (native) ───────────────────────────────────────────────
	r.Get("/api", s.handleAPIInfo)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", s.handleAPIInfo)

		// Native REST API (PART 14): plural resource routes only; no singular forms.
		r.Route("/pastes", func(r chi.Router) {
			r.Get("/", s.pasteHandler.ListPastes)
			r.Post("/", s.maybeRateLimit(s.pasteHandler.CreatePaste))
			r.Get("/{id}", s.pasteHandler.GetPaste)
			r.Delete("/{id}", s.maybeDeleteRateLimit(s.pasteHandler.DeletePaste))
			r.Get("/{id}/raw", s.pasteHandler.GetRawPaste)
		})

		// microbin-style /pasta alias
		r.Get("/pasta", s.compatHandler.MicrobinList)
		r.Post("/pasta", s.compatHandler.MicrobinCreate)
		r.Get("/pasta/{id}", s.compatHandler.MicrobinGet)
		r.Delete("/pasta/{id}", s.maybeDeleteRateLimit(s.compatHandler.MicrobinDelete))

		// lenpaste v1 versioned aliases
		r.Post("/new", s.compatHandler.LenCreate)
		r.Get("/get", s.compatHandler.LenGet)
		r.Get("/getServerInfo", s.compatHandler.LenServerInfo)

		// Server info
		r.Get("/server/healthz", s.handleHealthzJSON)
		r.Get("/server/version", s.handleVersion)
		r.Get("/server/swagger", s.swaggerHandler.ServeSpec)
		// /api/v1/server/graphql — versioned GraphQL endpoint (PART 14)
		r.Handle("/server/graphql", s.graphqlHandler)

		// Scheduler API (PART 18)
		// Read-only status routes are public. Mutating routes require server.token.
		r.Route("/scheduler", func(r chi.Router) {
			r.Get("/", s.handleSchedulerList)
			r.Get("/{id}", s.handleSchedulerShow)
			r.Get("/{id}/history", s.handleSchedulerHistory)
			r.With(s.requireOperatorToken).Post("/{id}/run", s.handleSchedulerRun)
			r.With(s.requireOperatorToken).Post("/{id}/enable", s.handleSchedulerEnable)
			r.With(s.requireOperatorToken).Post("/{id}/disable", s.handleSchedulerDisable)
		})
	})

	// Unversioned aliases — same handler as versioned, served directly (no redirect) per PART 14.
	r.Get("/api/swagger", s.swaggerHandler.ServeSpec)
	r.Handle("/api/graphql", s.graphqlHandler)
	r.Get("/api/healthz", s.handleHealthzJSON)
	// autodiscover is non-versioned by design (PART 32/14): clients use it before knowing the version.
	r.Get("/api/autodiscover", s.handleAutodiscover)

	// ── Web: main pages ──────────────────────────────────────────────────────
	r.Get("/", s.handleHome)
	// POST / — lenpaste form-POST to root creates a paste and redirects to /{id}
	r.Post("/", s.maybeRateLimit(s.pasteHandler.CreatePaste))
	r.Get("/recent", s.handleRecent)
	// microbin alias
	r.Get("/list", s.handleRecent)
	// pastebin.com alias
	r.Get("/archive", s.handleRecent)
	// pastebin.com alias
	r.Get("/trends", s.handleRecent)
	r.Post("/create", s.maybeRateLimit(s.handleWebCreate))
	r.Get("/create", s.handleCreatePage)

	// ── Redirect aliases ────────────────────────────────────────────────────
	r.Get("/guide", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/help", http.StatusFound)
	})
	r.Get("/emb_help", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/help", http.StatusFound)
	})
	r.Get("/about", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/about", http.StatusFound)
	})

	// ── Web: paste views ─────────────────────────────────────────────────────
	r.Get("/raw/{id}", s.pasteHandler.GetRawPaste)
	r.Get("/r/{id}", s.pasteHandler.GetRawPaste)
	r.Get("/dl/{id}", s.handleDownload)
	r.Get("/download/{id}", s.handleDownload)
	// microbin alias
	r.Get("/file/{id}", s.handleDownload)
	r.Get("/emb/{id}", s.handleEmbed)
	r.Get("/qr/{id}", s.handleQR)
	r.Get("/qr/{id}/image", s.handleQRImage)
	r.Get("/remove/{id}", s.handleRemovePage)
	r.Post("/remove/{id}", s.maybeDeleteRateLimit(s.handleRemoveSubmit))
	// microbin upload
	r.Post("/upload", s.maybeRateLimit(s.pasteHandler.CreatePaste))
	// microbin upload alias
	r.Get("/upload/{id}", s.handleViewPaste)
	// microbin short URL
	r.Get("/p/{id}", s.handleViewPaste)
	// pastebin.com /id/raw
	r.Get("/{id}/raw", s.pasteHandler.GetRawPaste)
	// microbin URL-paste redirect
	r.Get("/url/{id}", s.handleURLRedirect)
	// microbin short URL redirect
	r.Get("/u/{id}", s.handleURLRedirect)

	// Swagger UI (human-readable docs page)
	r.Get("/server/swagger", s.swaggerHandler.ServeUI)
	r.Get("/server/docs/swagger", s.swaggerHandler.ServeUI)
	r.Get("/server/docs/graphql", s.graphqlHandler.ServeHTTP)

	// ── Paste view — catch-all (must be last) ────────────────────────────────
	r.Get("/{id}", s.handleViewPaste)
}

// ─── Middleware ───────────────────────────────────────────────────────────────

// requireOperatorToken is middleware that enforces server.token authentication.
// It extracts "Authorization: Bearer <token>", SHA-256 hashes it, and compares
// against the cached hash using constant-time comparison (PART 11).
// Returns 401 on missing/invalid credentials — always with the same generic message
// to prevent user enumeration.
func (s *Server) requireOperatorToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(authHeader) <= len(prefix) || authHeader[:len(prefix)] != prefix {
			metrics.AuthAttemptsTotal.WithLabelValues("bearer", "failure").Inc()
			w.Header().Set("WWW-Authenticate", `Bearer realm="pastebin"`)
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"ok": false, "error": "UNAUTHORIZED", "message": "operator token required",
			})
			return
		}
		incoming := authHeader[len(prefix):]
		incomingHash := sha256.Sum256([]byte(incoming))
		var zeroHash [32]byte
		if s.operatorTokenHash == zeroHash {
			metrics.AuthAttemptsTotal.WithLabelValues("bearer", "failure").Inc()
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"ok": false, "error": "SERVER_ERROR", "message": "server.token not configured",
			})
			return
		}
		if subtle.ConstantTimeCompare(incomingHash[:], s.operatorTokenHash[:]) != 1 {
			metrics.AuthAttemptsTotal.WithLabelValues("bearer", "failure").Inc()
			w.Header().Set("WWW-Authenticate", `Bearer realm="pastebin"`)
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"ok": false, "error": "UNAUTHORIZED", "message": "operator token required",
			})
			return
		}
		metrics.AuthAttemptsTotal.WithLabelValues("bearer", "success").Inc()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.stats.inc()
		s.stats.activeConn.Add(1)
		defer s.stats.activeConn.Add(-1)
		next.ServeHTTP(w, r)
	})
}

// txtExtensionMiddleware strips ".txt" from API route paths so chi can match
// the canonical route (e.g., /api/v1/pastes.txt → /api/v1/pastes). It sets
// the httputil txt-extension flag in context so GetAPIResponseFormat detects
// that text output was requested even after the suffix is removed.
func (s *Server) txtExtensionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && strings.HasSuffix(r.URL.Path, ".txt") {
			// Strip .txt before routing so chi matches the canonical route path.
			stripped := *r.URL
			stripped.Path = strings.TrimSuffix(r.URL.Path, ".txt")
			r2 := httputil.WithTxtExtension(r)
			r2.URL = &stripped
			next.ServeHTTP(w, r2)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cors := s.liveCfg().Web.Security.CORS
		if cors == "" {
			cors = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", cors)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Delete-Token, X-Title, X-Language, X-Expires-In")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// permissionsPolicy is the default Permissions-Policy header value built from
// the PART 11 spec defaults at package init time.
var permissionsPolicy = strings.Join([]string{
	"accelerometer=()", "ambient-light-sensor=()", "battery=()", "camera=()",
	"display-capture=()", "geolocation=()", "gyroscope=()", "hid=()",
	"idle-detection=()", "magnetometer=()", "microphone=()", "midi=()",
	"screen-wake-lock=()", "serial=()", "usb=()", "xr-spatial-tracking=()",
	"attribution-reporting=()", "browsing-topics=()", "interest-cohort=()",
	"autoplay=(self)", "encrypted-media=(self)", "fullscreen=(self)",
	"payment=(self)", "picture-in-picture=(self)",
	"publickey-credentials-get=(self)", "storage-access=(self)", "web-share=(self)",
}, ", ")

// buildCSP returns the Content-Security-Policy header value for a request,
// using the server config's CSP settings.  When TLS is off, upgrade-insecure-requests
// is omitted.  In development mode, report-only mode is used.
func (s *Server) buildCSP(r *http.Request) (header string, reportOnly bool) {
	cfg := s.liveCfg()
	if !cfg.Web.CSP.Enabled {
		return "", false
	}

	fqdn := cfg.Server.FQDN
	apiVer := "v1"
	reportURI := "/api/" + apiVer + "/server/reports/csp"

	csp := cfg.Web.CSP
	scriptSrc := "'self' 'unsafe-inline'"
	if csp.ScriptSrcOverride != "" {
		scriptSrc = csp.ScriptSrcOverride
	} else if csp.ScriptSrcExtra != "" {
		scriptSrc += " " + csp.ScriptSrcExtra
	}
	styleSrc := "'self' 'unsafe-inline'"
	if csp.StyleSrcExtra != "" {
		styleSrc += " " + csp.StyleSrcExtra
	}
	imgSrc := "'self' data: blob: https:"
	if csp.ImgSrcExtra != "" {
		imgSrc += " " + csp.ImgSrcExtra
	}
	fontSrc := "'self' https:"
	if csp.FontSrcExtra != "" {
		fontSrc += " " + csp.FontSrcExtra
	}
	connectSrc := "'self'"
	if fqdn != "" && fqdn != "localhost" {
		connectSrc += " https://" + fqdn
	}
	if csp.ConnectSrcExtra != "" {
		connectSrc += " " + csp.ConnectSrcExtra
	}
	frameSrc := "'self'"
	if csp.FrameSrcExtra != "" {
		frameSrc += " " + csp.FrameSrcExtra
	}
	formAction := "'self'"
	if csp.FormActionExtra != "" {
		formAction += " " + csp.FormActionExtra
	}

	directives := []string{
		"default-src 'self'",
		"script-src " + scriptSrc,
		"style-src " + styleSrc,
		"img-src " + imgSrc,
		"font-src " + fontSrc,
		"connect-src " + connectSrc,
		"media-src 'self' blob:",
		"worker-src 'self' blob:",
		"manifest-src 'self'",
		"frame-src " + frameSrc,
		"frame-ancestors 'self'",
		"base-uri 'self'",
		"form-action " + formAction,
		"object-src 'none'",
	}

	// Only include upgrade-insecure-requests when TLS is enabled.
	if cfg.Server.TLS.Enabled {
		directives = append(directives, "upgrade-insecure-requests")
	}

	directives = append(directives,
		"report-to default",
		"report-uri "+reportURI,
	)

	policy := strings.Join(directives, "; ")
	isReportOnly := csp.Mode == "report-only" || cfg.Server.Mode == "development"
	return policy, isReportOnly
}

// buildReportingHeaders returns the Reporting-Endpoints, Report-To, and NEL header values.
func (s *Server) buildReportingHeaders() (endpoints, reportTo, nel string) {
	cfg := s.liveCfg()
	fqdn := cfg.Server.FQDN
	if fqdn == "" || fqdn == "localhost" || !cfg.Server.TLS.Enabled {
		return "", "", ""
	}
	apiVer := "v1"
	base := "https://" + fqdn + "/api/" + apiVer + "/server/reports"
	endpoints = `default="` + base + `/default"`
	reportTo = `{"group":"default","max_age":10886400,"endpoints":[{"url":"` + base + `/default"}]}`
	nel = `{"report_to":"default","max_age":2592000,"include_subdomains":true}`
	return endpoints, reportTo, nel
}

// securityHeadersMiddleware sets all mandatory security response headers per PART 11.
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := s.liveCfg()
		h := w.Header()

		// Mandatory legacy security headers (PART 11).
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		h.Set("Origin-Agent-Cluster", "?1")

		// Cross-origin isolation headers — defaults keep broad compatibility (PART 11).
		h.Set("Cross-Origin-Opener-Policy", "unsafe-none")
		h.Set("Cross-Origin-Embedder-Policy", "unsafe-none")
		h.Set("Cross-Origin-Resource-Policy", "cross-origin")

		// Content-Security-Policy.
		if policy, reportOnly := s.buildCSP(r); policy != "" {
			if reportOnly {
				h.Set("Content-Security-Policy-Report-Only", policy)
			} else {
				h.Set("Content-Security-Policy", policy)
			}
		}

		// Permissions-Policy.
		h.Set("Permissions-Policy", permissionsPolicy)

		// Strict-Transport-Security — only when TLS is active (RFC 6797).
		if cfg.Server.TLS.Enabled {
			hsts := cfg.Web.HSTS
			if hsts.Enabled {
				hstsVal := fmt.Sprintf("max-age=%d", hsts.MaxAgeSeconds)
				if hsts.IncludeSubdomains {
					hstsVal += "; includeSubDomains"
				}
				if hsts.Preload {
					hstsVal += "; preload"
				}
				h.Set("Strict-Transport-Security", hstsVal)
			}
		}

		// Reporting API (modern + legacy NEL) — only when TLS is enabled.
		if endpoints, reportTo, nel := s.buildReportingHeaders(); endpoints != "" {
			h.Set("Reporting-Endpoints", endpoints)
			h.Set("Report-To", reportTo)
			h.Set("NEL", nel)
		}

		// Per-request ID — use existing if forwarded, otherwise generate.
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = newRequestID()
		}
		h.Set("X-Request-ID", reqID)

		next.ServeHTTP(w, r)
	})
}

// newRequestID generates a compact hex request ID from 8 random bytes.
func newRequestID() string {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return "00000000"
	}
	return fmt.Sprintf("%x", b)
}

// metricsIPAllowlistMiddleware restricts /metrics to loopback addresses plus any
// IPs or CIDRs listed in cfg.Server.Metrics.AllowedIPs (PART 20).
// Loopback (127.0.0.1, ::1) is always permitted regardless of the configured list.
// When AllowedIPs is empty the endpoint is loopback-only; add CIDRs to permit
// additional internal networks (e.g. "10.0.0.0/8").
func (s *Server) metricsIPAllowlistMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		// Always allow loopback so monitoring on the same host works without config.
		if ip != nil && ip.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}
		cfg := s.liveCfg()
		allowed := cfg.Server.Metrics.AllowedIPs
		if len(allowed) > 0 {
			al := newAllowlistSet(allowed)
			if ip != nil && al.contains(ip) {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"ok":      false,
			"error":   "FORBIDDEN",
			"message": "metrics access denied",
		})
	})
}

// secFetchMiddleware rejects cross-site state-changing requests per PART 11.
// Validation rules (when sec_fetch_validation=true):
//   - Reject POST/PUT/PATCH/DELETE where Sec-Fetch-Site: cross-site AND no Bearer/API token.
//   - Reject GET/HEAD to /api/* where Sec-Fetch-Mode: navigate (unintended top-level nav).
//   - Absence of Sec-Fetch-* is treated as pass-through (legacy-browser compat).
func (s *Server) secFetchMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := s.liveCfg()
		if !cfg.Web.Headers.SecFetchValidation {
			next.ServeHTTP(w, r)
			return
		}

		// Check Sec-Fetch-Site: cross-site on state-changing methods.
		fetchSite := r.Header.Get("Sec-Fetch-Site")
		if fetchSite == "cross-site" {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				// Allow if Bearer or API-token auth is present — Bearer is not auto-attached
				// by browsers, so no CSRF risk.
				if r.Header.Get("Authorization") == "" && r.Header.Get("X-API-Token") == "" {
					// Check CSRF exempt paths.
					if !isCSRFExempt(r.URL.Path, cfg.Web.CSRF.ExemptPaths) {
						writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "SEC_FETCH_BLOCKED", "message": "cross-site state-changing request blocked"})
						return
					}
				}
			}
		}

		// Reject API endpoints navigated to directly (Sec-Fetch-Mode: navigate on /api/*).
		if r.Header.Get("Sec-Fetch-Mode") == "navigate" && strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusForbidden, map[string]interface{}{"ok": false, "error": "SEC_FETCH_BLOCKED", "message": "direct navigation to API endpoint blocked"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// reservedSlugs is the set of names that must not resolve to paste IDs (PART 16).
// These are system routes, common paths, and technical endpoints.
var reservedSlugs = map[string]struct{}{
	"api":           {},
	"server":        {},
	"static":        {},
	"assets":        {},
	"healthz":       {},
	"metrics":       {},
	"webhook":       {},
	"webhooks":      {},
	"search":        {},
	"explore":       {},
	"discover":      {},
	"trending":      {},
	"help":          {},
	"support":       {},
	"docs":          {},
	"documentation": {},
	"about":         {},
	"contact":       {},
	"terms":         {},
	"privacy":       {},
	"legal":         {},
	"security":      {},
	"graphql":       {},
	"swagger":       {},
	"rest":          {},
	"rpc":           {},
	"ws":            {},
	"websocket":     {},
	"cdn":           {},
	"media":         {},
	"uploads":       {},
	"files":         {},
	"images":        {},
	"robots.txt":    {},
	"sitemap.xml":   {},
	"favicon.ico":   {},
	".well-known":   {},
	"raw":           {},
	"dl":            {},
	"download":      {},
	"file":          {},
	"r":             {},
	"emb":           {},
	"qr":            {},
	"remove":        {},
	"upload":        {},
	"p":             {},
	"u":             {},
	"url":           {},
	"auth":          {},
	"recent":        {},
}

// isReservedSlug reports whether id is a reserved system name and must not
// be treated as a paste identifier.
func isReservedSlug(id string) bool {
	_, ok := reservedSlugs[strings.ToLower(id)]
	return ok
}

// csrfTokenKey is the context key under which the generated CSRF token string is stored.
type csrfTokenKeyType struct{}

var csrfTokenKey csrfTokenKeyType

// generateCSRFToken creates a new HMAC-SHA256 signed CSRF token using the server's csrfSecret.
// Format: base64(32-random-bytes) + "." + base64(HMAC of those bytes).
func (s *Server) generateCSRFToken() (string, error) {
	nonce := make([]byte, 32)
	if _, err := crand.Read(nonce); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.csrfSecret)
	mac.Write(nonce)
	sig := mac.Sum(nil)
	token := base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(sig)
	return token, nil
}

// validateCSRFToken reports whether token is a valid HMAC-signed CSRF token (constant-time).
func (s *Server) validateCSRFToken(token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(nonce) != 32 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, s.csrfSecret)
	mac.Write(nonce)
	expected := mac.Sum(nil)
	return subtle.ConstantTimeCompare(sig, expected) == 1
}

// csrfMiddleware implements the double-submit CSRF protection pattern (PART 11,
// AI.md "CSRF Protection"). The token cookie is stable across requests (reused
// while valid, re-minted only when absent or invalid) so tokens embedded in
// already-rendered forms keep working. Validation runs ONLY when the request is
// state-mutating AND from a cross-site/unknown origin; Bearer/API-token,
// read-only, WebSocket-upgrade, same-origin, and exempt-path requests are
// bypassed. A cross-site mutating request that arrives without the cookie is
// the signature of an attack (SameSite=Strict strips the cookie cross-site),
// so it is rejected with 403, never bypassed.
func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := s.liveCfg()
		if !cfg.Web.CSRF.Enabled || len(s.csrfSecret) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		// Inspect the request's existing CSRF cookie before issuing a new one.
		reqCookie, cookieErr := r.Cookie(cfg.Web.CSRF.CookieName)
		hasCookie := cookieErr == nil && reqCookie.Value != ""

		// Reuse a valid existing token; mint a fresh one only when absent/invalid.
		token := ""
		if hasCookie && s.validateCSRFToken(reqCookie.Value) {
			token = reqCookie.Value
		}
		if token == "" {
			t, err := s.generateCSRFToken()
			if err != nil {
				// Fail open — log and continue without setting the cookie.
				log.Printf("csrf: token generation failed: %v", err)
				next.ServeHTTP(w, r)
				return
			}
			token = t
		}

		// Validate the token if and only if ALL hold (AI.md "When CSRF
		// Validation Runs"): mutating method, cookie-authenticated, cross-site.
		isMutating := r.Method == http.MethodPost || r.Method == http.MethodPut ||
			r.Method == http.MethodPatch || r.Method == http.MethodDelete

		// Bypass conditions (any one is sufficient).
		hasBearer := r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Token") != ""
		bypass := !isMutating || hasBearer ||
			isWebSocketUpgrade(r) ||
			isCSRFExempt(r.URL.Path, cfg.Web.CSRF.ExemptPaths) ||
			isSameOrigin(r)

		if !bypass {
			// Read the submitted token from the header, falling back to the form field.
			submitted := r.Header.Get(cfg.Web.CSRF.HeaderName)
			if submitted == "" {
				_ = r.ParseForm()
				submitted = r.FormValue("csrf_token")
			}
			reason := ""
			switch {
			case !hasCookie:
				reason = "cookie absent"
			case submitted == "":
				reason = "token absent"
			case !s.validateCSRFToken(submitted):
				reason = "token signature invalid"
			case subtle.ConstantTimeCompare([]byte(submitted), []byte(reqCookie.Value)) != 1:
				reason = "token mismatch"
			}
			if reason != "" {
				clientHost, _, splitErr := net.SplitHostPort(r.RemoteAddr)
				if splitErr != nil {
					clientHost = r.RemoteAddr
				}
				log.Printf("security.csrf_failure ip=%s endpoint=%s reason=%q", clientHost, r.URL.Path, reason)
				writeJSON(w, http.StatusForbidden, map[string]interface{}{
					"ok":      false,
					"error":   "CSRF_FAILED",
					"message": "CSRF token validation failed",
				})
				return
			}
		}

		secure := r.TLS != nil
		if cfg.Web.CSRF.Secure == "true" {
			secure = true
		} else if cfg.Web.CSRF.Secure == "false" {
			secure = false
		}

		// Double-submit cookie: HttpOnly=false so progressive-enhancement JS can
		// echo the token into the X-CSRF-Token header; SameSite=Strict is the
		// primary defense (blocks cross-site cookie attachment entirely).
		http.SetCookie(w, &http.Cookie{
			Name:     cfg.Web.CSRF.CookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: false,
			Secure:   secure,
			SameSite: http.SameSiteStrictMode,
		})

		// Store token in context so renderTemplate can inject it into page data.
		ctx := context.WithValue(r.Context(), csrfTokenKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isWebSocketUpgrade reports whether the request is a WebSocket upgrade handshake.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// isSameOrigin reports whether the request originates from the app's own host.
// It compares the Origin header (or Referer when Origin is absent) host against
// r.Host. A missing/unparseable source is treated as cross-site (not same-origin)
// so it falls through to token validation per the CSRF spec.
func isSameOrigin(r *http.Request) bool {
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
	}
	if src == "" {
		return false
	}
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// isCSRFExempt reports whether path matches any of the exempt glob patterns.
// Only simple prefix and wildcard-suffix patterns are supported (e.g., /foo/*, /foo/bar).
func isCSRFExempt(path string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasSuffix(p, "/*") {
			if strings.HasPrefix(path, strings.TrimSuffix(p, "/*")+"/") || path == strings.TrimSuffix(p, "/*") {
				return true
			}
		} else if p == path {
			return true
		}
	}
	return false
}

// noTrailingSlash redirects paths with trailing slashes to the canonical
// form (no trailing slash). Root "/" is left unchanged. Paths whose last
// segment contains a "." (explicit file requests, e.g. /static/app.js/) are
// also left unchanged per PART 16 URL-normalization rules.
func (s *Server) noTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p != "/" && strings.HasSuffix(p, "/") {
			// Skip redirect for explicit file requests (last segment has a ".").
			lastSeg := p[strings.LastIndex(p, "/"):]
			if !strings.Contains(lastSeg, ".") {
				r.URL.Path = strings.TrimRight(p, "/")
				http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ─── Run ─────────────────────────────────────────────────────────────────────

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context, addr string) error {
	if s.geoipDB != nil {
		defer s.geoipDB.Close()
	}
	if s.cacheStore != nil {
		defer s.cacheStore.Close()
	}

	// Start Tor hidden service (non-fatal if Tor binary not found).
	if s.torManager != nil {
		if err := s.torManager.Start(); err != nil {
			log.Printf("tor: start failed: %v", err)
		} else if s.torManager.Running() {
			go s.torManager.Monitor()
		}
		defer s.torManager.Close()
	}

	cfg := s.liveCfg()

	// Parse HTTP server timeouts from config, falling back to safe defaults.
	readTimeout := 30 * time.Second
	if d, err := time.ParseDuration(cfg.Server.Limits.ReadTimeout); err == nil && d > 0 {
		readTimeout = d
	}
	writeTimeout := 30 * time.Second
	if d, err := time.ParseDuration(cfg.Server.Limits.WriteTimeout); err == nil && d > 0 {
		writeTimeout = d
	}
	idleTimeout := 120 * time.Second
	if d, err := time.ParseDuration(cfg.Server.Limits.IdleTimeout); err == nil && d > 0 {
		idleTimeout = d
	}

	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	errCh := make(chan error, 1)

	// When TLS is configured, set up SSL manager and serve HTTPS.
	if cfg.Server.TLS.Enabled {
		fqdn := cfg.Server.FQDN
		sslMgr := ssl.NewManager(ssl.Config{
			Enabled: true,
			CertDir: s.configDir + "/ssl",
			FQDN:    fqdn,
			LetsEncrypt: ssl.LetsEncryptConfig{
				Enabled:         cfg.Server.TLS.LetsEncrypt.Enabled,
				Email:           cfg.Server.TLS.LetsEncrypt.Email,
				Challenge:       ssl.ParseChallenge(cfg.Server.TLS.LetsEncrypt.Challenge),
				DNSProviderType: cfg.Server.TLS.DNSProvider,
				Staging:         cfg.Server.TLS.LetsEncrypt.Staging,
			},
		})

		domains := []string{fqdn}
		tlsCfg, err := sslMgr.GetTLSConfig(domains)
		if err != nil {
			log.Printf("ssl: TLS setup failed: %v — falling back to HTTP", err)
			// Fall through to plain HTTP so the server still starts.
		} else if tlsCfg != nil {
			srv.TLSConfig = tlsCfg
			// Wrap the router so autocert HTTP-01 challenges are handled on port 80.
			srv.Handler = sslMgr.GetHTTPHandler(s.router)
			go func() {
				if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			select {
			case <-ctx.Done():
				shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				return srv.Shutdown(shut)
			case err := <-errCh:
				return err
			}
		}
	}

	// Plain HTTP (no TLS, or TLS setup failed).
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shut)
	case err := <-errCh:
		return err
	}
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

// writeJSON marshals v with 2-space indent and writes it with a trailing newline.
func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"ok":false,"error":"SERVER_ERROR","message":"Internal server error"}` + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
	w.Write([]byte("\n"))
}

// ─── Health & info handlers ───────────────────────────────────────────────────

func (s *Server) buildHealthResponse() HealthResponse {
	checks := ChecksInfo{
		Database:  "ok",
		Cache:     "ok",
		Disk:      "ok",
		Scheduler: "ok",
	}

	if err := s.db.Ping(); err != nil {
		checks.Database = "error"
	}

	// Ping the cache driver (non-fatal for health status but surfaced in checks).
	if s.cacheStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := s.cacheStore.Ping(ctx); err != nil {
			checks.Cache = "error"
		}
		cancel()
	}

	if !s.checkDisk() {
		checks.Disk = "error"
	}

	// Scheduler health check via optional callback registered by main.
	if s.schedHealthFn != nil && !s.schedHealthFn() {
		checks.Scheduler = "error"
	}

	// Tor health check — report only when Tor is enabled.
	torInfo := s.buildTorInfo()
	if torInfo.Enabled {
		if torInfo.Running {
			checks.Tor = "ok"
		} else {
			checks.Tor = "error"
		}
	}

	status := "healthy"
	if checks.Database == "error" || checks.Disk == "error" {
		status = "unhealthy"
	} else if checks.Cache == "error" || checks.Scheduler == "error" || checks.Tor == "error" {
		status = "degraded"
	}

	// Fetch total paste count for stats (best-effort — zero on error).
	var pastesTotal int64
	if n, err := s.db.CountPastes(); err == nil {
		pastesTotal = n
	}

	// Collect pending-restart keys under the lock.
	s.pendingRestartMu.Lock()
	pendingKeys := make([]string, len(s.pendingRestartKeys))
	copy(pendingKeys, s.pendingRestartKeys)
	s.pendingRestartMu.Unlock()

	hr := HealthResponse{
		Project: ProjectInfo{
			Name:        s.liveCfg().Web.SiteTitle,
			Tagline:     "Simple, fast paste service",
			Description: "A self-hosted pastebin with syntax highlighting and burn-after-read support.",
		},
		Status:         status,
		PendingRestart: len(pendingKeys) > 0,
		RestartReason:  pendingKeys,
		Version:        s.version,
		GoVersion:      runtime.Version(),
		Build: BuildInfo{
			Commit: s.commitID,
			Date:   s.buildDate,
		},
		Uptime:    formatUptime(time.Since(s.startTime)),
		Mode:      s.cfg.Server.Mode,
		Timestamp: time.Now().UTC(),
		Features: FeaturesInfo{
			Tor:   torInfo,
			GeoIP: s.geoipDB != nil,
		},
		Checks: checks,
		Stats: StatsInfo{
			RequestsTotal: s.stats.total.Load(),
			Requests24h:   s.stats.last24h(),
			ActiveConns:   int(s.stats.activeConn.Load()),
			PastesTotal:   pastesTotal,
		},
	}
	return hr
}


// buildTorInfo returns the TorInfo block for health responses.
func (s *Server) buildTorInfo() TorInfo {
	if s.torManager == nil {
		return TorInfo{Enabled: false, Running: false, Status: "disabled", Hostname: ""}
	}
	running := s.torManager.Running()
	onion := s.torManager.OnionAddress()
	status := "starting"
	if running {
		status = "healthy"
	}
	return TorInfo{
		Enabled:  true,
		Running:  running,
		Status:   status,
		Hostname: onion,
	}
}

// formatUptime converts a duration to a human-readable string like "2d 5h 30m".
func formatUptime(d time.Duration) string {
	d = d.Round(time.Minute)
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	switch detectClientType(r) {
	case "json":
		s.handleHealthzJSON(w, r)
		return
	case "text":
		hr := s.buildHealthResponse()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writeHealthText(w, hr)
		return
	}
	hr := s.buildHealthResponse()
	s.renderTemplate(w, r, "healthz.html", map[string]interface{}{
		"Health":    hr,
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
	})
}

// writeHealthText emits the /server/healthz response in the canonical
// flattened dot-notation plain-text format defined in PART 13.
func writeHealthText(w http.ResponseWriter, hr HealthResponse) {
	fmt.Fprintf(w, "project.name: %s\n", hr.Project.Name)
	fmt.Fprintf(w, "project.tagline: %s\n", hr.Project.Tagline)
	fmt.Fprintf(w, "project.description: %s\n", hr.Project.Description)
	fmt.Fprintf(w, "status: %s\n", hr.Status)
	fmt.Fprintf(w, "version: %s\n", hr.Version)
	fmt.Fprintf(w, "go_version: %s\n", hr.GoVersion)
	fmt.Fprintf(w, "build.commit: %s\n", hr.Build.Commit)
	fmt.Fprintf(w, "build.date: %s\n", hr.Build.Date)
	fmt.Fprintf(w, "uptime: %s\n", hr.Uptime)
	fmt.Fprintf(w, "mode: %s\n", hr.Mode)
	fmt.Fprintf(w, "timestamp: %s\n", hr.Timestamp.UTC().Format(time.RFC3339))
	fmt.Fprintf(w, "features.tor.enabled: %t\n", hr.Features.Tor.Enabled)
	fmt.Fprintf(w, "features.tor.running: %t\n", hr.Features.Tor.Running)
	if hr.Features.Tor.Status != "" {
		fmt.Fprintf(w, "features.tor.status: %s\n", hr.Features.Tor.Status)
	}
	if hr.Features.Tor.Hostname != "" {
		fmt.Fprintf(w, "features.tor.hostname: %s\n", hr.Features.Tor.Hostname)
	}
	fmt.Fprintf(w, "features.geoip: %t\n", hr.Features.GeoIP)
	fmt.Fprintf(w, "checks.database: %s\n", hr.Checks.Database)
	fmt.Fprintf(w, "checks.cache: %s\n", hr.Checks.Cache)
	fmt.Fprintf(w, "checks.disk: %s\n", hr.Checks.Disk)
	fmt.Fprintf(w, "checks.scheduler: %s\n", hr.Checks.Scheduler)
	if hr.Checks.Tor != "" {
		fmt.Fprintf(w, "checks.tor: %s\n", hr.Checks.Tor)
	}
	fmt.Fprintf(w, "stats.requests_total: %d\n", hr.Stats.RequestsTotal)
	fmt.Fprintf(w, "stats.requests_24h: %d\n", hr.Stats.Requests24h)
	fmt.Fprintf(w, "stats.active_connections: %d\n", hr.Stats.ActiveConns)
}

func (s *Server) handleHealthzJSON(w http.ResponseWriter, r *http.Request) {
	hr := s.buildHealthResponse()
	writeJSON(w, http.StatusOK, hr)
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"version": s.version}})
}

func (s *Server) handleAPIInfo(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	base := scheme + "://" + r.Host

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
		"name":    "Pastebin API",
		"version": s.version,
		"endpoints": map[string]interface{}{
			"native": map[string]string{
				"GET    /api/v1/pastes":          "list public pastes",
				"POST   /api/v1/pastes":          "create paste (JSON/multipart/raw)",
				"GET    /api/v1/pastes/{id}":     "get paste JSON",
				"DELETE /api/v1/pastes/{id}":     "delete paste (requires token)",
				"GET    /api/v1/pastes/{id}/raw": "get paste raw text",
			},
			"web": map[string]string{
				"GET  /":           "home",
				"GET  /create":     "create form",
				"POST /create":     "create paste (form/raw)",
				"GET  /{id}":       "view paste",
				"GET  /raw/{id}":   "raw content",
				"GET  /dl/{id}":    "download",
				"GET  /emb/{id}":   "embed view",
				"GET  /remove/{id}": "delete form",
			},
			"compat_pastebin": map[string]string{
				"POST /api/api_post.php":  "create paste",
				"GET  /api/api_raw.php":   "get raw paste (?i=ID)",
				"POST /api/api_login.php": "always returns ANONYMOUS",
			},
			"compat_lenpaste": map[string]string{
				"POST /api/new":    "create paste",
				"GET  /api/get":    "get paste (?id=ID)",
				"DELETE /api/remove": "delete paste (?id=ID&deleteToken=TOKEN)",
				"GET  /api/list":   "list pastes",
			},
		},
		"examples": map[string]string{
			"curl_raw":  "curl --data-binary @file.txt " + base + "/create",
			"curl_file": "curl -F 'files=@code.py' " + base + "/create",
			"curl_json": `curl -H "Content-Type: application/json" -d '{"content":"hello"}' ` + base + "/api/v1/pastes",
			"pipe":      "cat file.txt | curl --data-binary @- " + base + "/create",
		},
	},
	})
}

// handleAutodiscover serves /api/autodiscover — returns server info and CLI
// update metadata (PART 32 / PART 14). Non-versioned by design: clients use it
// before knowing which API version the server supports.
func (s *Server) handleAutodiscover(w http.ResponseWriter, r *http.Request) {
	cfg := s.liveCfg()
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	base := scheme + "://" + r.Host
	if cfg.Server.BaseURL != "" {
		base = strings.TrimRight(cfg.Server.BaseURL, "/")
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			// Server identity
			"server":      "pastebin",
			"version":     s.version,
			"api_version": "v1",
			"base_url":    base,

			// CLI update metadata (PART 32).
			// cli_versions maps os-arch → {version, sha256} for each available binary.
			// Empty map = no CLI binaries hosted by this server; clients stay on their
			// installed version. Operators can populate via the release workflow.
			"cli_versions":    map[string]interface{}{},
			"cli_min_version": "0.0.0",

			// Feature flags visible to clients
			"features": map[string]interface{}{
				"tor":     s.torManager != nil && s.torManager.Running(),
				"metrics": cfg.Server.Metrics.Enabled,
			},
		},
	})
}

// ─── Web page handlers ────────────────────────────────────────────────────────

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	pastes, _, _ := s.db.GetPublicPastes(1, 5)
	data := map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"BaseURL":   s.baseURL(r),
		"Recent":    pastes,
	}

	// Content negotiation: HTTP tools get the full template rendered as plain text.
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "home.html", data)
		if err != nil {
			// Fallback when templates are unavailable: minimal plain text.
			fmt.Fprintf(w, "%s\nPOST %s/api/v1/pastes to create a paste.\n", s.liveCfg().Web.SiteTitle, s.baseURL(r))
			for _, p := range pastes {
				fmt.Fprintf(w, "%s/%s\t%s\n", s.baseURL(r), p.ID, p.Title)
			}
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}

	s.renderTemplate(w, r, "home.html", data)
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "create.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
	})
}

// handleWebCreate handles browser form submissions to POST /create. The form
// submits as a standard urlencoded POST (no JavaScript required, PART 16); the
// result — including the one-time owner token — is rendered server-side back
// into create.html. Non-browser callers (JSON/multipart/raw) are delegated to
// the content-negotiating API handler.
func (s *Server) handleWebCreate(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		s.pasteHandler.CreatePaste(w, r)
		return
	}

	data := map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
	}

	resp, status, err := s.pasteHandler.CreateFromForm(r)
	if err != nil {
		if status == 0 {
			status = http.StatusBadRequest
		}
		data["Error"] = err.Error()
		w.WriteHeader(status)
		s.renderTemplate(w, r, "create.html", data)
		return
	}

	data["Created"] = resp
	s.renderTemplate(w, r, "create.html", data)
}

func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	page := 1
	pastes, total, _ := s.db.GetPublicPastes(page, 20)
	data := map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"BaseURL":   s.baseURL(r),
		"Pastes":    pastes,
		"Total":     total,
	}

	// Content negotiation: HTTP tools get the full template rendered as plain text.
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "recent.html", data)
		if err != nil {
			// Fallback when templates are unavailable: minimal plain text.
			fmt.Fprintf(w, "# Recent pastes (%d total)\n", total)
			for _, p := range pastes {
				fmt.Fprintf(w, "%s/%s\t%s\t%s\n", s.baseURL(r), p.ID, p.Language, p.Title)
			}
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}

	s.renderTemplate(w, r, "recent.html", data)
}

// detectClientType returns "html", "json", or "text" based on User-Agent and
// Accept header per PART 14 content negotiation rules.
//
// Priority order:
//  1. Explicit Accept header overrides UA detection.
//  2. Our CLI client (pastebin-cli/) → "json" (INTERACTIVE, renders its own TUI).
//  3. Text browsers (lynx, w3m, etc.) → "html" (INTERACTIVE, no JavaScript).
//  4. HTTP tools (curl, wget, empty UA) → "text" (NON-INTERACTIVE, dump output).
//  5. Anything else (regular browsers) → "html".
func detectClientType(r *http.Request) string {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		return "json"
	}
	if strings.Contains(accept, "text/plain") {
		return "text"
	}
	if strings.Contains(accept, "text/html") {
		return "html"
	}

	// Our client is INTERACTIVE — receives JSON, renders own TUI/GUI.
	if httputil.IsOurCliClient(r) {
		return "json"
	}

	// Text browsers are INTERACTIVE but have no JavaScript support.
	// They receive the standard HTML templates (forms use POST, no JS required).
	if httputil.IsTextBrowser(r) {
		return "html"
	}

	// HTTP tools are NON-INTERACTIVE — send pre-formatted plain text.
	if httputil.IsHttpTool(r) {
		return "text"
	}

	// Default: regular browser.
	return "html"
}

func (s *Server) handleViewPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Reject reserved system slugs — these are never valid paste IDs.
	if isReservedSlug(id) {
		http.NotFound(w, r)
		return
	}

	paste, err := s.pasteHandler.GetPasteForWeb(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SERVER_ERROR", "message": "internal server error"})
		return
	}
	if paste == nil {
		http.NotFound(w, r)
		return
	}

	// Content negotiation per PART 16: CLI tools get raw text, browsers get HTML.
	switch detectClientType(r) {
	case "text":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(paste.Content))
		return
	case "json":
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":   true,
			"data": paste,
		})
		return
	}

	s.renderTemplate(w, r, "paste.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"Paste":     paste,
		"ID":        id,
		"Content":   handler.HighlightedContent(paste),
	})
}

func (s *Server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	paste, err := s.pasteHandler.GetPasteForWeb(id)
	if err != nil || paste == nil {
		http.NotFound(w, r)
		return
	}

	s.renderTemplate(w, r, "emb.html", map[string]interface{}{
		"Paste":   paste,
		"Content": handler.HighlightedContent(paste),
	})
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	link := s.baseURL(r) + "/" + id
	// Render a simple page that generates a QR code client-side (or redirect to QR API).
	s.renderTemplate(w, r, "qr.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"ID":        id,
		"Link":      link,
	})
}

// handleQRImage generates a QR code PNG server-side and streams it directly to
// the client. This avoids any external API dependency per PART 16.
func (s *Server) handleQRImage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	link := s.baseURL(r) + "/" + id
	png, err := qrcode.Encode(link, qrcode.Medium, 300)
	if err != nil {
		http.Error(w, "qr generation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(png) //nolint:errcheck
}

func (s *Server) handleRemovePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.renderTemplate(w, r, "remove.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"ID":        id,
		"Error":     "",
		"Success":   false,
	})
}

func (s *Server) handleRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "BAD_REQUEST", "message": "bad request"})
		return
	}
	token := r.FormValue("token")
	if token == "" {
		s.renderTemplate(w, r, "remove.html", map[string]interface{}{
			"SiteTitle": s.liveCfg().Web.SiteTitle,
			"Theme":     s.liveCfg().Web.Theme,
			"ID":        id,
			"Error":     "owner token is required",
			"Success":   false,
		})
		return
	}

	// Two-tier auth (PART 11): operator token bypasses api_tokens lookup.
	incomingHash := sha256.Sum256([]byte(token))
	var zeroHash [32]byte
	if s.operatorTokenHash != zeroHash &&
		subtle.ConstantTimeCompare(incomingHash[:], s.operatorTokenHash[:]) == 1 {
		if err := s.db.DeletePaste(id); err != nil {
			s.renderTemplate(w, r, "remove.html", map[string]interface{}{
				"SiteTitle": s.liveCfg().Web.SiteTitle,
				"Theme":     s.liveCfg().Web.Theme,
				"ID":        id,
				"Error":     "paste not found",
				"Success":   false,
			})
			return
		}
	} else if err := s.db.VerifyAPIToken(incomingHash, "paste", id); err != nil {
		s.renderTemplate(w, r, "remove.html", map[string]interface{}{
			"SiteTitle": s.liveCfg().Web.SiteTitle,
			"Theme":     s.liveCfg().Web.Theme,
			"ID":        id,
			"Error":     "paste not found or invalid token",
			"Success":   false,
		})
		return
	} else if err := s.db.DeletePaste(id); err != nil {
		s.renderTemplate(w, r, "remove.html", map[string]interface{}{
			"SiteTitle": s.liveCfg().Web.SiteTitle,
			"Theme":     s.liveCfg().Web.Theme,
			"ID":        id,
			"Error":     "paste not found",
			"Success":   false,
		})
		return
	}

	s.renderTemplate(w, r, "remove.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"ID":        id,
		"Error":     "",
		"Success":   true,
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	paste, err := s.pasteHandler.GetPasteForWeb(id)
	if err != nil || paste == nil {
		http.NotFound(w, r)
		return
	}

	filename := paste.Title
	if filename == "" || filename == "Untitled" {
		filename = id
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write([]byte(paste.Content))
}

// handleURLRedirect redirects to the paste content if it is a URL; otherwise
// shows the paste view. Used for microbin /url/{id} and /u/{id}.
func (s *Server) handleURLRedirect(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	paste, err := s.pasteHandler.GetPasteForWeb(id)
	if err != nil || paste == nil {
		http.NotFound(w, r)
		return
	}
	content := strings.TrimSpace(paste.Content)
	if strings.HasPrefix(content, "http://") || strings.HasPrefix(content, "https://") {
		http.Redirect(w, r, content, http.StatusFound)
		return
	}
	// Fall back to normal paste view.
	s.renderTemplate(w, r, "paste.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"Paste":     paste,
		"ID":        id,
		"Content":   handler.HighlightedContent(paste),
	})
}


// ─── Server info pages ────────────────────────────────────────────────────────

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "about.html", s.pageData())
		if err != nil {
			fmt.Fprintf(w, "%s\n%s/server/about\n", s.liveCfg().Web.SiteTitle, s.baseURL(r))
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}
	s.renderTemplate(w, r, "about.html", s.pageData())
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"BaseURL":   s.baseURL(r),
	}
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "help.html", data)
		if err != nil {
			fmt.Fprintf(w, "Usage: curl %s/create\nAPI docs: %s/server/docs/swagger\n", s.baseURL(r), s.baseURL(r))
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}
	s.renderTemplate(w, r, "help.html", data)
}

func (s *Server) handleContact(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"Contact":   s.liveCfg().Web.Security.Contact,
	}
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "contact.html", data)
		if err != nil {
			fmt.Fprintf(w, "Contact: %s\n", s.liveCfg().Web.Security.Contact)
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}
	s.renderTemplate(w, r, "contact.html", data)
}

func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "privacy.html", s.pageData())
		if err != nil {
			fmt.Fprintf(w, "Privacy Policy — %s/server/privacy\n", s.baseURL(r))
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}
	s.renderTemplate(w, r, "privacy.html", s.pageData())
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	if detectClientType(r) == "text" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		html, err := s.renderTemplateToString(r, "terms.html", s.pageData())
		if err != nil {
			fmt.Fprintf(w, "Terms of Service — %s/server/terms\n", s.baseURL(r))
			return
		}
		fmt.Fprint(w, httputil.HTML2TextConverter(html, 80))
		return
	}
	s.renderTemplate(w, r, "terms.html", s.pageData())
}

// ─── Misc handlers ────────────────────────────────────────────────────────────

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":             s.liveCfg().Web.SiteTitle,
		"short_name":       "Paste",
		"description":      "A fast, public pastebin service",
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#1e1e2e",
		"theme_color":      "#89b4fa",
		"icons": []map[string]interface{}{
			{"src": "/static/icons/icon-192.png", "sizes": "192x192", "type": "image/png", "purpose": "any maskable"},
			{"src": "/static/icons/icon-512.png", "sizes": "512x512", "type": "image/png", "purpose": "any maskable"},
		},
	})
}

func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	// Inject server version so cache names are tied to the release.
	version := s.version
	if version == "" {
		version = "dev"
	}
	sw := fmt.Sprintf(`// Pastebin Service Worker — version %s
const CACHE_VERSION = %q;
const CACHE_NAME = 'pastebin-cache-' + CACHE_VERSION;
const PRECACHE_ASSETS = [
  '/',
  '/create',
  '/recent',
  '/offline',
  '/static/css/main.css',
  '/static/js/main.js',
  '/static/icons/icon-192.png',
  '/static/icons/icon-512.png'
];

// INSTALL — pre-cache assets and activate immediately
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(PRECACHE_ASSETS))
      .then(() => self.skipWaiting())
  );
});

// ACTIVATE — purge stale caches and claim all clients
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(
        keys
          .filter(k => k.startsWith('pastebin-cache-') && k !== CACHE_NAME)
          .map(k => caches.delete(k))
      ))
      .then(() => self.clients.claim())
  );
});

// MESSAGE — allow clients to trigger skipWaiting for instant updates
self.addEventListener('message', event => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});

// FETCH — tiered caching strategy
self.addEventListener('fetch', event => {
  const { request } = event;
  const url = new URL(request.url);

  // Only intercept same-origin GET requests
  if (request.method !== 'GET' || url.origin !== self.location.origin) return;

  // API calls: network-only (never cache)
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/graphql')) return;

  // Static assets: cache-first, update cache on network hit
  if (url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request).then(cached => {
        if (cached) return cached;
        return fetch(request).then(response => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
          return response;
        });
      })
    );
    return;
  }

  // HTML pages: network-first, fall back to cache then offline page
  if (request.headers.get('accept') && request.headers.get('accept').includes('text/html')) {
    event.respondWith(
      fetch(request)
        .then(response => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
          return response;
        })
        .catch(() => caches.match(request)
          .then(cached => cached || caches.match('/offline'))
        )
    );
    return;
  }

  // Default: network-first with cache fallback
  event.respondWith(
    fetch(request).catch(() => caches.match(request))
  );
});
`, version, version)
	w.Write([]byte(sw))
}

// pwaIconSVG returns an SVG icon for the PWA manifest at the given size.
// The icon is a rounded-rect with the accent colour and a clipboard emoji.
func pwaIconSVG(size int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">
  <rect width="%d" height="%d" rx="%d" fill="#6366f1"/>
  <text x="50%%" y="55%%" dominant-baseline="middle" text-anchor="middle" font-size="%d" font-family="serif">📋</text>
</svg>`, size, size, size, size, size, size, size/6, size*2/3)
}

func (s *Server) handlePWAIcon180(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(pwaIconSVG(180)))
}

func (s *Server) handlePWAIcon192(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(pwaIconSVG(192)))
}

func (s *Server) handlePWAIcon512(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(pwaIconSVG(512)))
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	for _, p := range s.liveCfg().Web.Robots.Allow {
		b.WriteString("Allow: " + p + "\n")
	}
	for _, p := range s.liveCfg().Web.Robots.Deny {
		b.WriteString("Disallow: " + p + "\n")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(b.String()))
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("Contact: " + s.liveCfg().Web.Security.Contact + "\nPreferred-Languages: en\n"))
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/static/favicon.ico", http.StatusFound)
}

// handleOffline serves the PWA offline fallback page.
// The service worker caches this page and serves it when the user is offline
// and no cached version of the requested page is available.
func (s *Server) handleOffline(w http.ResponseWriter, r *http.Request) {
	d := s.pageData()
	s.renderTemplate(w, r, "offline.html", d)
}

// ─── Scheduler API handlers (PART 18) ────────────────────────────────────────

// handleSchedulerList returns all registered tasks.
func (s *Server) handleSchedulerList(w http.ResponseWriter, r *http.Request) {
	if s.schedulerAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SERVICE_UNAVAILABLE", "message": "scheduler not available"})
		return
	}
	tasks := s.schedulerAPI.GetTasks()
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": tasks})
}

// handleSchedulerShow returns the state for a single task.
func (s *Server) handleSchedulerShow(w http.ResponseWriter, r *http.Request) {
	if s.schedulerAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SERVICE_UNAVAILABLE", "message": "scheduler not available"})
		return
	}
	id := chi.URLParam(r, "id")
	t, ok := s.schedulerAPI.GetTask(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "task not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": t})
}

// handleSchedulerRun triggers a task to run immediately.
func (s *Server) handleSchedulerRun(w http.ResponseWriter, r *http.Request) {
	if s.schedulerAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SERVICE_UNAVAILABLE", "message": "scheduler not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.schedulerAPI.RunNow(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"ok": false, "error": "BAD_REQUEST", "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "triggered", "task": id}})
}

// handleSchedulerEnable enables a task.
func (s *Server) handleSchedulerEnable(w http.ResponseWriter, r *http.Request) {
	if s.schedulerAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SERVICE_UNAVAILABLE", "message": "scheduler not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.schedulerAPI.GetTask(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "task not found"})
		return
	}
	s.schedulerAPI.EnableTask(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "enabled", "task": id}})
}

// handleSchedulerDisable disables a task.
func (s *Server) handleSchedulerDisable(w http.ResponseWriter, r *http.Request) {
	if s.schedulerAPI == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{"ok": false, "error": "SERVICE_UNAVAILABLE", "message": "scheduler not available"})
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := s.schedulerAPI.GetTask(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]interface{}{"ok": false, "error": "NOT_FOUND", "message": "task not found"})
		return
	}
	s.schedulerAPI.DisableTask(id)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "disabled", "task": id}})
}

// handleSchedulerHistory returns recent execution history for a task.
func (s *Server) handleSchedulerHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	history, err := s.db.ListTaskHistory(id, 20)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SERVER_ERROR", "message": "could not retrieve history"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": history})
}

// ─── Template helpers ─────────────────────────────────────────────────────────

func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {
	if s.templates == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SERVER_ERROR", "message": "templates not loaded"})
		return
	}
	if data == nil {
		data = make(map[string]interface{})
	}
	// Inject language for i18n — templates access it as .Lang
	lang := i18n.LangFromRequest(r)
	data["Lang"] = lang
	// Inject text direction for RTL languages (Arabic) — templates access it as .Dir
	data["Dir"] = i18n.Direction(lang)
	// Inject CSRF token for forms — templates access it as .CSRFToken
	if tok, ok := r.Context().Value(csrfTokenKey).(string); ok && tok != "" {
		data["CSRFToken"] = tok
	} else {
		data["CSRFToken"] = ""
	}
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s error: %v", name, err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"ok": false, "error": "SERVER_ERROR", "message": "internal server error"})
	}
}

// renderTemplateToString renders a named template to a string instead of writing to a ResponseWriter.
// Used by handlers that need to apply HTML2TextConverter for plain-text clients.
func (s *Server) renderTemplateToString(r *http.Request, name string, data map[string]interface{}) (string, error) {
	if s.templates == nil {
		return "", fmt.Errorf("templates not loaded")
	}
	if data == nil {
		data = make(map[string]interface{})
	}
	lang := i18n.LangFromRequest(r)
	data["Lang"] = lang
	data["Dir"] = i18n.Direction(lang)
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (s *Server) pageData() map[string]interface{} {
	return map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
	}
}

// baseURL constructs the base URL for the current request (PART 12).
// Resolution order (highest priority first):
//  1. Config/CLI: server.base_url
//  2. X-Forwarded-Prefix header (from trusted reverse proxy)
//  3. X-Forwarded-Path header (alternative prefix header)
//  4. X-Script-Name header (WSGI-style)
//  5. Default: scheme://host derived from TLS state and X-Forwarded-Proto
//
// X-Forwarded-* headers are only honored when the immediate peer is a trusted
// proxy (loopback, private ranges, or server.trusted_proxies.additional).
func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.Server.BaseURL != "" {
		return s.cfg.Server.BaseURL
	}

	trusted := s.isTrustedPeer(r)

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	} else if trusted && r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}

	host := r.Host
	if trusted {
		if fh := r.Header.Get("X-Forwarded-Host"); fh != "" {
			host = fh
		}
	}

	base := scheme + "://" + host

	if trusted {
		// Append reverse-proxy path prefix when present (trailing slash stripped).
		for _, hdr := range []string{"X-Forwarded-Prefix", "X-Forwarded-Path", "X-Script-Name"} {
			if prefix := r.Header.Get(hdr); prefix != "" {
				prefix = strings.TrimRight(prefix, "/")
				if prefix != "" {
					base += prefix
				}
				break
			}
		}
	}
	return base
}

// privateNets contains the CIDR ranges that are always treated as trusted proxies
// (loopback, RFC 1918 IPv4 private, RFC 4193 IPv6 unique-local, link-local).
var privateNets = func() []*net.IPNet {
	cidrs := []string{
		"127.0.0.0/8",
		"::1/128",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
		"169.254.0.0/16",
		"fe80::/10",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}()

// isTrustedPeer returns true when the immediate peer's IP is a loopback,
// private-range address, or in server.trusted_proxies.additional (PART 12).
func (s *Server) isTrustedPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	// Check server.trusted_proxies.additional entries (IPs and CIDRs; DNS not resolved here).
	for _, entry := range s.cfg.Server.TrustedProxies.Additional {
		if strings.Contains(entry, "/") {
			if _, ipNet, err := net.ParseCIDR(entry); err == nil && ipNet.Contains(ip) {
				return true
			}
		} else if net.ParseIP(entry) != nil && net.ParseIP(entry).Equal(ip) {
			return true
		}
	}
	return false
}
