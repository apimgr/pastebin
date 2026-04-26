package server

import (
	"context"
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/admin"
	"github.com/apimgr/pastebin/src/auth"
	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/database"
	"github.com/apimgr/pastebin/src/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Server struct {
	router       *chi.Mux
	db           database.DB
	config       *config.Config
	templates    *template.Template
	authService  *auth.AuthService
	pasteHandler *handlers.PasteHandler
	authHandler  *handlers.AuthHandler
	adminHandler *admin.Handler
	version      string
}

func New(db database.DB, cfg *config.Config, version string) *Server {
	s := &Server{
		router:  chi.NewRouter(),
		db:      db,
		config:  cfg,
		version: version,
	}

	// Initialize auth service
	s.authService = auth.NewAuthService(db, cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)

	// Initialize handlers
	s.pasteHandler = handlers.NewPasteHandler(db, "")
	s.authHandler = handlers.NewAuthHandler(db, s.authService)

	// Initialize admin handler
	s.adminHandler = admin.NewHandler(
		cfg.Server.Admin.Username,
		cfg.Server.Admin.Password,
		cfg.Server.Admin.APIToken,
		cfg.Server.Session.Timeout,
		false, // SSL enabled - would check TLS config
		version,
		"",    // commit
		"",    // buildDate
	)

	// Parse templates
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		log.Printf("Warning: could not parse templates: %v", err)
	}
	s.templates = tmpl

	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	r := s.router

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(s.corsMiddleware)

	// Register admin routes
	s.adminHandler.RegisterRoutes(r)

	// Health check
	r.Get("/healthz", s.handleHealthz)
	r.Get("/health", s.handleHealthz)

	// Static files
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// PWA support
	r.Get("/manifest.json", s.handleManifest)
	r.Get("/sw.js", s.handleServiceWorker)
	r.Get("/robots.txt", s.handleRobots)
	r.Get("/security.txt", s.handleSecurity)

	// API info
	r.Get("/api", s.handleAPIInfo)
	r.Get("/api/v1", s.handleAPIInfo)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Auth routes
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", s.authHandler.Register)
			r.Post("/login", s.authHandler.Login)

			// Protected routes
			r.Group(func(r chi.Router) {
				r.Use(s.authService.Middleware(true))
				r.Get("/me", s.authHandler.GetMe)
				r.Get("/tokens", s.authHandler.ListTokens)
				r.Post("/tokens", s.authHandler.CreateToken)
				r.Delete("/tokens/{tokenId}", s.authHandler.DeleteToken)
				r.Get("/pastes", s.authHandler.GetMyPastes)
			})
		})

		// Paste routes
		r.Get("/create", s.pasteHandler.ListPastes)
		r.Get("/pastes", s.pasteHandler.ListPastes)

		r.Group(func(r chi.Router) {
			r.Use(s.authService.Middleware(false)) // Optional auth
			r.Post("/create", s.pasteHandler.CreatePaste)
			r.Post("/pastes", s.pasteHandler.CreatePaste)
		})
	})

	// Web routes
	r.Get("/", s.handleHome)
	r.Get("/create", s.handleCreatePage)
	r.Get("/recent", s.handleRecentPage)

	// Raw paste routes
	r.Get("/raw/{id}", s.pasteHandler.GetRawPaste)
	r.Get("/r/{id}", s.pasteHandler.GetRawPaste)

	// Download route
	r.Get("/download/{id}", s.handleDownload)

	// Create paste (unified endpoint for web/curl)
	r.Group(func(r chi.Router) {
		r.Use(s.authService.Middleware(false))
		r.Post("/create", s.pasteHandler.CreatePaste)
	})

	// View paste (must be last - catch-all for paste IDs)
	r.Get("/{id}", s.handleViewPaste)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.config.WebSecurity.CORS)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) Run(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	errChan := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"database":  s.db.Type(),
		"version":   s.version,
	})
}

func (s *Server) handleAPIInfo(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	baseURL := scheme + "://" + r.Host

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"name":        "Pastebin API",
		"version":     "1.0.0",
		"description": "A pastebin service with multi-database support",
		"endpoints": map[string]interface{}{
			"unified": map[string]string{
				"GET /create":       "List all public pastes (web form)",
				"POST /create":      "Create paste (curl, form, or JSON)",
				"GET /api/v1/pastes": "List all public pastes (JSON)",
				"POST /api/v1/pastes": "Create paste (JSON)",
			},
			"auth": map[string]string{
				"register": "POST /api/v1/auth/register",
				"login":    "POST /api/v1/auth/login",
				"tokens":   "GET/POST/DELETE /api/v1/auth/tokens",
				"me":       "GET /api/v1/auth/me",
			},
			"web": map[string]string{
				"home":     "GET /",
				"create":   "GET /create (form), POST /create (submit)",
				"paste":    "GET /:id",
				"raw":      "GET /raw/:id or /r/:id",
				"download": "GET /download/:id",
			},
		},
		"examples": map[string]string{
			"curl_text": "curl -X POST --data-binary @file.txt " + baseURL + "/create",
			"curl_file": "curl -X POST -F \"files=@file.txt\" " + baseURL + "/create",
			"curl_json": "curl -X POST -H \"Content-Type: application/json\" -d '{\"content\":\"hello world\"}' " + baseURL + "/api/v1/create",
		},
	})
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if s.templates == nil {
		http.Redirect(w, r, "/create", http.StatusTemporaryRedirect)
		return
	}

	data := map[string]interface{}{
		"SiteTitle": s.config.WebUI.SiteTitle,
		"Theme":     s.config.WebUI.Theme,
		"Version":   s.version,
	}

	if err := s.templates.ExecuteTemplate(w, "home.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Redirect(w, r, "/create", http.StatusTemporaryRedirect)
	}
}

func (s *Server) handleCreatePage(w http.ResponseWriter, r *http.Request) {
	if s.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SiteTitle": s.config.WebUI.SiteTitle,
		"Theme":     s.config.WebUI.Theme,
	}

	if err := s.templates.ExecuteTemplate(w, "create.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleRecentPage(w http.ResponseWriter, r *http.Request) {
	if s.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"SiteTitle": s.config.WebUI.SiteTitle,
		"Theme":     s.config.WebUI.Theme,
	}

	if err := s.templates.ExecuteTemplate(w, "recent.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) handleViewPaste(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	paste, err := s.db.GetPasteByID(id)
	if err != nil || paste == nil {
		http.Redirect(w, r, "/create", http.StatusTemporaryRedirect)
		return
	}

	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Paste has expired", http.StatusGone)
		return
	}

	if !paste.IsPublic {
		http.Error(w, "This paste is private", http.StatusForbidden)
		return
	}

	s.db.IncrementPasteViews(id)
	paste.Views++

	if s.templates == nil {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(paste.Content))
		return
	}

	data := map[string]interface{}{
		"SiteTitle": s.config.WebUI.SiteTitle,
		"Theme":     s.config.WebUI.Theme,
		"Paste":     paste,
		"ID":        id,
	}

	if err := s.templates.ExecuteTemplate(w, "paste.html", data); err != nil {
		log.Printf("Template error: %v", err)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(paste.Content))
	}
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	paste, err := s.db.GetPasteByID(id)
	if err != nil || paste == nil {
		http.Error(w, "Paste not found", http.StatusNotFound)
		return
	}

	if paste.ExpiresAt != nil && paste.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Paste has expired", http.StatusGone)
		return
	}

	if !paste.IsPublic {
		http.Error(w, "This paste is private", http.StatusForbidden)
		return
	}

	filename := paste.Title
	if filename == "Untitled" {
		filename = id
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Write([]byte(paste.Content))
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	manifest := map[string]interface{}{
		"name":             s.config.WebUI.SiteTitle,
		"short_name":       "Pastebin",
		"description":      "A pastebin service",
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#0d1117",
		"theme_color":      "#238636",
		"icons": []map[string]interface{}{
			{"src": "/static/icons/icon-192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/static/icons/icon-512.png", "sizes": "512x512", "type": "image/png"},
		},
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	json.NewEncoder(w).Encode(manifest)
}

func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	sw := `const CACHE_NAME = 'pastebin-v1';
const urlsToCache = ['/', '/create', '/static/css/main.css', '/static/js/main.js'];

self.addEventListener('install', event => {
  event.waitUntil(caches.open(CACHE_NAME).then(cache => cache.addAll(urlsToCache)));
});

self.addEventListener('fetch', event => {
  event.respondWith(
    caches.match(event.request).then(response => response || fetch(event.request))
  );
});`
	w.Header().Set("Content-Type", "application/javascript")
	w.Write([]byte(sw))
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	var builder strings.Builder
	builder.WriteString("User-agent: *\n")
	for _, path := range s.config.WebRobots.Allow {
		builder.WriteString("Allow: " + path + "\n")
	}
	for _, path := range s.config.WebRobots.Deny {
		builder.WriteString("Disallow: " + path + "\n")
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(builder.String()))
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	security := "Contact: mailto:" + s.config.WebSecurity.Admin + "\nPreferred-Languages: en\n"
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(security))
}
