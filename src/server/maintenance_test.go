package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apimgr/pastebin/src/config"
	"github.com/apimgr/pastebin/src/health"
)

// okHandler is a sentinel next-handler that records whether it ran.
func okHandler(ran *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*ran = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestMaintenanceMiddlewarePassThroughWhenNormal(t *testing.T) {
	s := &Server{maintenance: health.New(health.Config{})}
	ran := false
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pastes", nil)
	rr := httptest.NewRecorder()
	s.maintenanceMiddleware(okHandler(&ran)).ServeHTTP(rr, req)
	if !ran {
		t.Fatal("expected next handler to run when not in maintenance")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestMaintenanceMiddlewareBlocksWrites(t *testing.T) {
	m := health.New(health.Config{SelfHealingEnabled: true})
	m.Enter(health.ReasonDatabaseConnection, "Database connection failed")
	s := &Server{maintenance: m}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		ran := false
		req := httptest.NewRequest(method, "/api/v1/pastes", nil)
		rr := httptest.NewRecorder()
		s.maintenanceMiddleware(okHandler(&ran)).ServeHTTP(rr, req)

		if ran {
			t.Fatalf("%s: next handler should not run in maintenance", method)
		}
		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", method, rr.Code)
		}
		if rr.Header().Get("X-Maintenance-Mode") != "true" {
			t.Fatalf("%s: missing X-Maintenance-Mode header", method)
		}
		if rr.Header().Get("X-Maintenance-Reason") != health.ReasonDatabaseConnection {
			t.Fatalf("%s: wrong reason header: %q", method, rr.Header().Get("X-Maintenance-Reason"))
		}
		if rr.Header().Get("Retry-After") == "" {
			t.Fatalf("%s: missing Retry-After header", method)
		}

		var body struct {
			OK      bool   `json:"ok"`
			Error   string `json:"error"`
			Message string `json:"message"`
			Details struct {
				Reason      string `json:"reason"`
				SelfHealing bool   `json:"self_healing"`
			} `json:"details"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: bad json: %v", method, err)
		}
		if body.OK || body.Error != "MAINTENANCE" {
			t.Fatalf("%s: wrong envelope: %+v", method, body)
		}
		if body.Details.Reason != health.ReasonDatabaseConnection || !body.Details.SelfHealing {
			t.Fatalf("%s: wrong details: %+v", method, body.Details)
		}
	}
}

func TestCriticalCheck(t *testing.T) {
	t.Run("db failure reports database_connection", func(t *testing.T) {
		s := &Server{db: &stubDB{pingErr: errors.New("down")}}
		ok, reason, msg := s.criticalCheck()
		if ok || reason != health.ReasonDatabaseConnection || msg == "" {
			t.Fatalf("expected db failure, got ok=%v reason=%q msg=%q", ok, reason, msg)
		}
	})
	t.Run("healthy db passes", func(t *testing.T) {
		s := &Server{db: &stubDB{}}
		ok, reason, _ := s.criticalCheck()
		// Disk is expected healthy on the test host, so this should pass.
		if !ok || reason != "" {
			t.Fatalf("expected healthy, got ok=%v reason=%q", ok, reason)
		}
	})
}

func TestMaintenanceCleanupTrimsBackupsAndLogs(t *testing.T) {
	dir := t.TempDir()
	backupDir := filepath.Join(dir, "backup")
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	base := time.Now()
	for i := 0; i < 4; i++ {
		p := filepath.Join(backupDir, "pastebin_backup_"+time.Duration(i).String()+".tar.gz")
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Stagger modtimes so ordering is deterministic.
		mt := base.Add(time.Duration(i) * time.Hour)
		_ = os.Chtimes(p, mt, mt)
	}

	oldLog := filepath.Join(logDir, "old.log")
	newLog := filepath.Join(logDir, "new.log")
	if err := os.WriteFile(oldLog, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newLog, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := base.Add(-100 * 24 * time.Hour)
	_ = os.Chtimes(oldLog, old, old)

	cfg := &config.Config{}
	cfg.Server.Maintenance.Cleanup.BackupKeepCount = 2
	cfg.Server.Maintenance.Cleanup.LogRetentionDays = 7
	s := &Server{cfg: cfg, dataDir: dir}
	s.maintenanceCleanup()

	entries, _ := os.ReadDir(backupDir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 backups kept, got %d", len(entries))
	}
	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatal("expected old log removed")
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Fatal("expected new log kept")
	}
}

func TestMaintenanceCleanupNoDataDir(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	// Must not panic when dataDir is empty.
	s.maintenanceCleanup()
}

func TestMaintenanceNotifyNoopWhenEmailDisabled(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	// Email disabled by default → must be a silent no-op (no panic).
	s.maintenanceNotify("enter", health.Snapshot{Reason: health.ReasonDatabaseConnection, Message: "db down"})
	s.maintenanceNotify("exit", health.Snapshot{Reason: health.ReasonDatabaseConnection})
}

func TestMaintenanceMiddlewareAllowsReads(t *testing.T) {
	m := health.New(health.Config{})
	m.Enter(health.ReasonDatabaseConnection, "db down")
	s := &Server{maintenance: m}

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		ran := false
		req := httptest.NewRequest(method, "/api/v1/pastes/abc", nil)
		rr := httptest.NewRecorder()
		s.maintenanceMiddleware(okHandler(&ran)).ServeHTTP(rr, req)
		if !ran {
			t.Fatalf("%s: read should pass through during maintenance", method)
		}
	}
}
