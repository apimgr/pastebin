package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/handler"
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
	router        *chi.Mux
	db            database.DB
	cfg           *config.Config
	templates     *template.Template
	pasteHandler  *handler.PasteHandler
	compatHandler *handler.CompatHandler
	createLimiter *rateLimiter
	version       string
	commitID      string
	buildDate     string
	startTime     time.Time
	stats         requestStats
}

// New constructs a Server and wires all routes.
func New(db database.DB, cfg *config.Config, version, commitID, buildDate string) *Server {
	s := &Server{
		router:    chi.NewRouter(),
		db:        db,
		cfg:       cfg,
		version:   version,
		commitID:  commitID,
		buildDate: buildDate,
		startTime: time.Now(),
	}
	s.stats.lastHour = time.Now().Hour()

	s.pasteHandler = handler.NewPasteHandler(db, cfg.Server.BaseURL)
	s.compatHandler = handler.NewCompatHandler(s.pasteHandler, db)

	// Default: 30 creates per IP per minute; configurable via rate_limit.create_per_minute.
	createLimit := cfg.RateLimit.CreatePerM
	if createLimit <= 0 {
		createLimit = 30
	}
	if cfg.RateLimit.Enabled {
		s.createLimiter = newRateLimiter(createLimit, time.Minute)
	}

	tmpl, err := template.New("").ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Printf("warning: could not parse templates: %v", err)
	}
	s.templates = tmpl

	s.setupRoutes()
	return s
}

// maybeRateLimit wraps h with the create rate limiter if enabled.
func (s *Server) maybeRateLimit(h http.HandlerFunc) http.HandlerFunc {
	if s.createLimiter == nil {
		return h
	}
	mw := rateLimitMiddleware(s.createLimiter)
	return mw(h).ServeHTTP
}

func (s *Server) setupRoutes() {
	r := s.router

	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
	r.Use(s.corsMiddleware)
	r.Use(s.noTrailingSlash)
	r.Use(s.countRequests)

	// ── Static assets & PWA ──────────────────────────────────────────────────
	staticSub, _ := fs.Sub(staticFS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	r.Get("/manifest.json", s.handleManifest)
	r.Get("/sw.js", s.handleServiceWorker)
	r.Get("/robots.txt", s.handleRobots)
	r.Get("/security.txt", s.handleSecurity)
	r.Get("/favicon.ico", s.handleFavicon)

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
	r.Delete("/api/remove", s.compatHandler.LenRemove)
	r.Get("/api/remove", s.compatHandler.LenRemove) // some clients use GET
	r.Get("/api/list", s.compatHandler.LenList)

	// ── Versioned API (native) ───────────────────────────────────────────────
	r.Get("/api", s.handleAPIInfo)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/", s.handleAPIInfo)

		// Pastes
		r.Get("/pastes", s.pasteHandler.ListPastes)
		r.Post("/pastes", s.maybeRateLimit(s.pasteHandler.CreatePaste))
		r.Get("/pastes/{id}", s.pasteHandler.GetPaste)
		r.Delete("/pastes/{id}", s.pasteHandler.DeletePaste)

		// microbin-style /pasta alias
		r.Get("/pasta", s.compatHandler.MicrobinList)
		r.Post("/pasta", s.compatHandler.MicrobinCreate)
		r.Get("/pasta/{id}", s.compatHandler.MicrobinGet)
		r.Delete("/pasta/{id}", s.compatHandler.MicrobinDelete)

		// lenpaste v1 versioned aliases
		r.Post("/new", s.compatHandler.LenCreate)
		r.Get("/get", s.compatHandler.LenGet)
		r.Get("/getServerInfo", s.compatHandler.LenServerInfo)

		// Server info
		r.Get("/server/healthz", s.handleHealthzJSON)
		r.Get("/server/version", s.handleVersion)
		r.Get("/server/swagger", s.handleSwagger)
	})

	// ── Web: main pages ──────────────────────────────────────────────────────
	r.Get("/", s.handleHome)
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
	r.Post("/remove/{id}", s.handleRemoveSubmit)
	r.Post("/upload", s.maybeRateLimit(s.pasteHandler.CreatePaste)) // microbin upload
	r.Get("/upload/{id}", s.handleViewPaste)          // microbin upload alias
	r.Get("/p/{id}", s.handleViewPaste)               // microbin short URL
	r.Get("/{id}/raw", s.pasteHandler.GetRawPaste)   // pastebin.com /id/raw
	r.Get("/url/{id}", s.handleURLRedirect)           // microbin URL-paste redirect
	r.Get("/u/{id}", s.handleURLRedirect)             // microbin short URL redirect

	// GraphQL
	r.Handle("/graphql", s.compatHandler.GraphQLHandler())

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
		cors := s.cfg.Web.Security.CORS
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
	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
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

	if !s.checkDisk() {
		checks.Disk = "error"
	}

	status := "healthy"
	if checks.Database == "error" || checks.Disk == "error" {
		status = "unhealthy"
	}

	hr := HealthResponse{
		Project: ProjectInfo{
			Name:        s.cfg.Web.SiteTitle,
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
			Tor:   TorInfo{Enabled: false, Running: false, Status: "disabled", Hostname: ""},
			GeoIP: false,
		},
		Checks: checks,
		Stats: StatsInfo{
			RequestsTotal: s.stats.total.Load(),
			Requests24h:   s.stats.last24h(),
			ActiveConns:   int(s.stats.activeConn.Load()),
		},
	}
	return hr
}

// checkDisk returns true when at least 100 MiB of free space is available.
func (s *Server) checkDisk() bool {
	var stat syscall.Statfs_t
	dir := os.TempDir()
	if err := syscall.Statfs(dir, &stat); err != nil {
		return true // assume ok if we can't check
	}
	free := stat.Bavail * uint64(stat.Bsize)
	return free > 100<<20 // 100 MiB
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
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
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
				"GET  /api/v1/pastes":       "list public pastes",
				"POST /api/v1/pastes":       "create paste (JSON/multipart/raw)",
				"GET  /api/v1/pastes/{id}":  "get paste JSON",
				"DELETE /api/v1/pastes/{id}": "delete paste (requires token)",
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

// ─── Web page handlers ────────────────────────────────────────────────────────

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	pastes, _, _ := s.db.GetPublicPastes(1, 5)
	s.renderTemplate(w, r, "home.html", map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
		"BaseURL":   s.baseURL(r),
		"Recent":    pastes,
	})
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "create.html", map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
	})
}

func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	page := 1
	pastes, total, _ := s.db.GetPublicPastes(page, 20)
	s.renderTemplate(w, r, "recent.html", map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
		"Pastes":    pastes,
		"Total":     total,
	})
}

func (s *Server) handleViewPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

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
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
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
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
		"ID":        id,
		"Link":      link,
	})
}

func (s *Server) handleRemovePage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.renderTemplate(w, r, "remove.html", map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
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
			"SiteTitle": s.cfg.Web.SiteTitle,
			"Theme":     s.cfg.Web.Theme,
			"ID":        id,
			"Error":     "delete token is required",
			"Success":   false,
		})
		return
	}

	if err := s.db.DeletePasteByToken(id, handler.HashToken(token)); err != nil {
		s.renderTemplate(w, r, "remove.html", map[string]interface{}{
			"SiteTitle": s.cfg.Web.SiteTitle,
			"Theme":     s.cfg.Web.Theme,
			"ID":        id,
			"Error":     "paste not found or invalid token",
			"Success":   false,
		})
		return
	}

	s.renderTemplate(w, r, "remove.html", map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
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
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
		"Paste":     paste,
		"ID":        id,
		"Content":   handler.HighlightedContent(paste),
	})
}

// handleSwagger serves a minimal OpenAPI JSON description.
func (s *Server) handleSwagger(w http.ResponseWriter, r *http.Request) {
	base := s.baseURL(r)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"openapi": "3.0.3",
		"info": map[string]interface{}{
			"title":   s.cfg.Web.SiteTitle + " API",
			"version": s.version,
		},
		"servers": []map[string]interface{}{{"url": base}},
		"paths": map[string]interface{}{
			"/api/v1/pastes": map[string]interface{}{
				"get":  map[string]interface{}{"summary": "List public pastes", "tags": []string{"pastes"}},
				"post": map[string]interface{}{"summary": "Create paste", "tags": []string{"pastes"}},
			},
			"/api/v1/pastes/{id}": map[string]interface{}{
				"get":    map[string]interface{}{"summary": "Get paste by ID", "tags": []string{"pastes"}},
				"delete": map[string]interface{}{"summary": "Delete paste", "tags": []string{"pastes"}},
			},
		},
	})
}

// ─── Server info pages ────────────────────────────────────────────────────────

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "about.html", s.pageData())
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "help.html", map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
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
		"name":             s.cfg.Web.SiteTitle,
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
	for _, p := range s.cfg.Web.Robots.Allow {
		b.WriteString("Allow: " + p + "\n")
	}
	for _, p := range s.cfg.Web.Robots.Deny {
		b.WriteString("Disallow: " + p + "\n")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(b.String()))
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte("Contact: " + s.cfg.Web.Security.Contact + "\nPreferred-Languages: en\n"))
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
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s error: %v", name, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) pageData() map[string]interface{} {
	return map[string]interface{}{
		"SiteTitle": s.cfg.Web.SiteTitle,
		"Theme":     s.cfg.Web.Theme,
	}
}

func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.Server.BaseURL != "" {
		return s.cfg.Server.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
