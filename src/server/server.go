package server

import (
	"context"
	crand "crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apimgr/pastebin/src/cache"
	"github.com/apimgr/pastebin/src/common/i18n"
	"github.com/apimgr/pastebin/src/config"
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

// HealthResponse is the canonical /server/healthz response structure.
type HealthResponse struct {
	Project   ProjectInfo  `json:"project"`
	Status    string       `json:"status"`
	Version   string       `json:"version"`
	GoVersion string       `json:"go_version"`
	Build     BuildInfo    `json:"build"`
	Uptime    string       `json:"uptime"`
	Mode      string       `json:"mode"`
	Timestamp time.Time    `json:"timestamp"`
	Cluster   ClusterInfo  `json:"cluster"`
	Features  FeaturesInfo `json:"features"`
	Checks    ChecksInfo   `json:"checks"`
	Stats     StatsInfo    `json:"stats"`
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

// ClusterInfo reports cluster state (single-node for now).
type ClusterInfo struct {
	Enabled bool `json:"enabled"`
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
	Database string `json:"database"`
	Cache    string `json:"cache"`
	Disk     string `json:"disk"`
}

// StatsInfo holds public-safe aggregate statistics.
type StatsInfo struct {
	RequestsTotal  int64 `json:"requests_total"`
	Requests24h    int64 `json:"requests_24h"`
	ActiveConns    int   `json:"active_connections"`
	PastesTotal    int64 `json:"pastes_total"`
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

	s.pasteHandler = handler.NewPasteHandler(db, cfg.Server.BaseURL)
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

	s.setupRoutes()
	return s
}

// OnConfigChange is called by the ConfigManager after each successful hot-reload.
// It updates rate limiter thresholds so they take effect on the next request.
func (s *Server) OnConfigChange(next *config.Config) {
	if s.createLimiter != nil && next.RateLimit.CreatePerM > 0 {
		s.createLimiter.UpdateLimit(next.RateLimit.CreatePerM)
	}
	if s.deleteLimiter != nil && next.RateLimit.DeletePerM > 0 {
		s.deleteLimiter.UpdateLimit(next.RateLimit.DeletePerM)
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

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
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
	r.Use(s.securityHeadersMiddleware)
	r.Use(s.secFetchMiddleware)
	r.Use(s.corsMiddleware)
	r.Use(s.noTrailingSlash)
	r.Use(s.countRequests)
	r.Use(s.metricsCollector.Middleware())
	if s.geoipDB != nil {
		r.Use(s.geoipDB.Middleware())
	}

	// ── Static assets & PWA ──────────────────────────────────────────────────
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	r.Get("/manifest.json", s.handleManifest)
	r.Get("/sw.js", s.handleServiceWorker)
	r.Get("/robots.txt", s.handleRobots)
	r.Get("/security.txt", s.handleSecurity)
	r.Get("/favicon.ico", s.handleFavicon)

	// ── Metrics endpoint ─────────────────────────────────────────────────────
	if s.cfg.Server.Metrics.Enabled {
		endpoint := s.cfg.Server.Metrics.Endpoint
		if endpoint == "" {
			endpoint = "/metrics"
		}
		r.Handle(endpoint, s.metricsCollector.Handler())
	}

	// ── Server info pages ────────────────────────────────────────────────────
	r.Get("/server/about", s.handleAbout)
	r.Get("/server/help", s.handleHelp)
	r.Get("/server/privacy", s.handlePrivacy)
	r.Get("/server/terms", s.handleTerms)
	r.Get("/server/healthz", s.handleHealthz)
	r.Get("/healthz", s.handleHealthz)
	r.Get("/health", s.handleHealthz)

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
	r.Get("/u/{username}", handler.AuthStubRedirect) // user profiles → home
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
	r.Get("/api/remove", s.maybeDeleteRateLimit(s.compatHandler.LenRemove)) // some clients use GET
	r.Get("/api/list", s.compatHandler.LenList)

	// ── Versioned API (native) ───────────────────────────────────────────────
	r.Get("/api", s.handleAPIInfo)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", s.handleAPIInfo)

		// Native REST API (IDEA.md):
		//   POST   /api/v1/paste         — create
		//   GET    /api/v1/paste/{id}    — retrieve
		//   DELETE /api/v1/paste/{id}    — delete
		//   GET    /api/v1/paste/{id}/raw — raw text
		//   GET    /api/v1/pastes        — list (plural)
		r.Get("/pastes", s.pasteHandler.ListPastes)
		r.Route("/paste", func(r chi.Router) {
			r.Post("/", s.maybeRateLimit(s.pasteHandler.CreatePaste))
			r.Get("/{id}", s.pasteHandler.GetPaste)
			r.Delete("/{id}", s.maybeDeleteRateLimit(s.pasteHandler.DeletePaste))
			r.Get("/{id}/raw", s.pasteHandler.GetRawPaste)
		})

		// Legacy plural-noun aliases so existing integrations keep working.
		r.Post("/pastes", s.maybeRateLimit(s.pasteHandler.CreatePaste))
		r.Get("/pastes/{id}", s.pasteHandler.GetPaste)
		r.Delete("/pastes/{id}", s.maybeDeleteRateLimit(s.pasteHandler.DeletePaste))
		r.Get("/pastes/{id}/raw", s.pasteHandler.GetRawPaste)

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
	})

	// Unversioned aliases that MUST mount the same handler as the versioned route.
	r.Get("/api/swagger", s.swaggerHandler.ServeSpec)
	r.Get("/api/graphql", s.graphqlHandler.ServeHTTP)

	// ── Web: main pages ──────────────────────────────────────────────────────
	r.Get("/", s.handleHome)
	// POST / — lenpaste form-POST to root creates a paste and redirects to /{id}
	r.Post("/", s.maybeRateLimit(s.pasteHandler.CreatePaste))
	r.Get("/recent", s.handleRecent)
	r.Get("/list", s.handleRecent)    // microbin alias
	r.Get("/archive", s.handleRecent) // pastebin.com alias
	r.Get("/trends", s.handleRecent)  // pastebin.com alias
	r.Post("/create", s.maybeRateLimit(s.pasteHandler.CreatePaste))
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
	r.Get("/file/{id}", s.handleDownload) // microbin alias
	r.Get("/emb/{id}", s.handleEmbed)
	r.Get("/qr/{id}", s.handleQR)
	r.Get("/remove/{id}", s.handleRemovePage)
	r.Post("/remove/{id}", s.maybeDeleteRateLimit(s.handleRemoveSubmit))
	r.Post("/upload", s.maybeRateLimit(s.pasteHandler.CreatePaste)) // microbin upload
	r.Get("/upload/{id}", s.handleViewPaste)          // microbin upload alias
	r.Get("/p/{id}", s.handleViewPaste)               // microbin short URL
	r.Get("/{id}/raw", s.pasteHandler.GetRawPaste)   // pastebin.com /id/raw
	r.Get("/url/{id}", s.handleURLRedirect)           // microbin URL-paste redirect
	r.Get("/u/{id}", s.handleURLRedirect)             // microbin short URL redirect

	// GraphQL endpoint (POST for queries, GET for GraphiQL UI)
	r.Handle("/graphql", s.graphqlHandler)

	// Swagger UI (human-readable docs page)
	r.Get("/server/swagger", s.swaggerHandler.ServeUI)
	r.Get("/server/docs/swagger", s.swaggerHandler.ServeUI)
	r.Get("/server/docs/graphql", s.graphqlHandler.ServeHTTP)

	// ── Paste view — catch-all (must be last) ────────────────────────────────
	r.Get("/{id}", s.handleViewPaste)
}

// ─── Middleware ───────────────────────────────────────────────────────────────

func (s *Server) countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.stats.inc()
		s.stats.activeConn.Add(1)
		defer s.stats.activeConn.Add(-1)
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
						http.Error(w, `{"ok":false,"error":"SEC_FETCH_BLOCKED","message":"cross-site state-changing request blocked"}`, http.StatusForbidden)
						return
					}
				}
			}
		}

		// Reject API endpoints navigated to directly (Sec-Fetch-Mode: navigate on /api/*).
		if r.Header.Get("Sec-Fetch-Mode") == "navigate" && strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, `{"ok":false,"error":"SEC_FETCH_BLOCKED","message":"direct navigation to API endpoint blocked"}`, http.StatusForbidden)
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

func (s *Server) noTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			r.URL.Path = strings.TrimRight(r.URL.Path, "/")
			http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
			return
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
				Enabled:         true,
				Email:           cfg.Server.TLS.Email,
				Challenge:       ssl.ParseChallenge(cfg.Server.TLS.Challenge),
				DNSProviderType: cfg.Server.TLS.DNSProvider,
				Staging:         cfg.Server.TLS.Staging,
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
		http.Error(w, `{"ok":false,"error":"SERVER_ERROR","message":"Internal server error"}`, http.StatusInternalServerError)
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
		Database: "ok",
		Cache:    "ok",
		Disk:     "ok",
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

	status := "healthy"
	if checks.Database == "error" || checks.Disk == "error" {
		status = "unhealthy"
	}

	// Fetch total paste count for stats (best-effort — zero on error).
	var pastesTotal int64
	if n, err := s.db.CountPastes(); err == nil {
		pastesTotal = n
	}

	hr := HealthResponse{
		Project: ProjectInfo{
			Name:        s.liveCfg().Web.SiteTitle,
			Tagline:     "Simple, fast paste service",
			Description: "A self-hosted pastebin with syntax highlighting and burn-after-read support.",
		},
		Status:    status,
		Version:   s.version,
		GoVersion: runtime.Version(),
		Build: BuildInfo{
			Commit: s.commitID,
			Date:   s.buildDate,
		},
		Uptime:    formatUptime(time.Since(s.startTime)),
		Mode:      s.cfg.Server.Mode,
		Timestamp: time.Now().UTC(),
		Cluster: ClusterInfo{
			Enabled: false,
		},
		Features: FeaturesInfo{
			Tor:   s.buildTorInfo(),
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
		status = "running"
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
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		s.handleHealthzJSON(w, r)
		return
	}
	hr := s.buildHealthResponse()
	s.renderTemplate(w, r, "healthz.html", map[string]interface{}{
		"Health":    hr,
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
	})
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
				"GET    /api/v1/pastes":         "list public pastes",
				"POST   /api/v1/paste":          "create paste (JSON/multipart/raw)",
				"GET    /api/v1/paste/{id}":     "get paste JSON",
				"DELETE /api/v1/paste/{id}":     "delete paste (requires token)",
				"GET    /api/v1/paste/{id}/raw":  "get paste raw text",
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
			"curl_json": `curl -H "Content-Type: application/json" -d '{"content":"hello"}' ` + base + "/api/v1/paste",
			"pipe":      "cat file.txt | curl --data-binary @- " + base + "/create",
		},
	},
	})
}

// ─── Web page handlers ────────────────────────────────────────────────────────

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	pastes, _, _ := s.db.GetPublicPastes(1, 5)
	s.renderTemplate(w, r, "home.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"BaseURL":   s.baseURL(r),
		"Recent":    pastes,
	})
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "create.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
	})
}

func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	page := 1
	pastes, total, _ := s.db.GetPublicPastes(page, 20)
	s.renderTemplate(w, r, "recent.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"Pastes":    pastes,
		"Total":     total,
	})
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
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if paste == nil {
		http.NotFound(w, r)
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
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	if token == "" {
		s.renderTemplate(w, r, "remove.html", map[string]interface{}{
			"SiteTitle": s.liveCfg().Web.SiteTitle,
			"Theme":     s.liveCfg().Web.Theme,
			"ID":        id,
			"Error":     "delete token is required",
			"Success":   false,
		})
		return
	}

	if err := s.db.DeletePasteByToken(id, handler.HashToken(token)); err != nil {
		s.renderTemplate(w, r, "remove.html", map[string]interface{}{
			"SiteTitle": s.liveCfg().Web.SiteTitle,
			"Theme":     s.liveCfg().Web.Theme,
			"ID":        id,
			"Error":     "paste not found or invalid token",
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
	s.renderTemplate(w, r, "about.html", s.pageData())
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "help.html", map[string]interface{}{
		"SiteTitle": s.liveCfg().Web.SiteTitle,
		"Theme":     s.liveCfg().Web.Theme,
		"BaseURL":   s.baseURL(r),
	})
}

func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "privacy.html", s.pageData())
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
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
		"background_color": "#0d1117",
		"theme_color":      "#238636",
		"icons": []map[string]interface{}{
			{"src": "/static/icons/icon-192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/static/icons/icon-512.png", "sizes": "512x512", "type": "image/png"},
		},
	})
}

func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(`const CACHE='paste-v1';
const FILES=['/','/ create','/static/css/main.css','/static/js/main.js'];
self.addEventListener('install',e=>e.waitUntil(caches.open(CACHE).then(c=>c.addAll(FILES))));
self.addEventListener('fetch',e=>e.respondWith(caches.match(e.request).then(r=>r||fetch(e.request))));`))
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

// ─── Template helpers ─────────────────────────────────────────────────────────

func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {
	if s.templates == nil {
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = make(map[string]interface{})
	}
	// Inject language for i18n — templates access it as .Lang
	data["Lang"] = i18n.LangFromRequest(r)
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s error: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
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
