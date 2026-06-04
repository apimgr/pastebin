package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "/"},
		{"root", "/", "/"},
		{"short word", "/pastes", "/pastes"},
		{"api path no id", "/api/v1/paste", "/api/v1/paste"},
		{"single id segment", "/abc12345", "/:id"},
		{"api path with id", "/api/v1/paste/abc12345def0", "/api/v1/paste/:id"},
		{"static asset", "/static/app.css", "/static/app.css"},
		{"uuid path", "/api/v1/user/a1b2c3d4-e5f6-7890-abcd-ef1234567890", "/api/v1/user/:id"},
		{"nested no id", "/api/v1/health", "/api/v1/health"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePath(tc.input)
			if got != tc.want {
				t.Errorf("normalizePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestIsIDSegment(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"too short", "abc", false},
		{"word pastes", "pastes", false},
		{"8 hex chars", "abc12345", true},
		{"12 hex chars", "abc12345def0", true},
		{"uuid", "a1b2c3d4-e5f6-7890-abcd-ef1234567890", true},
		{"word users", "users", false},
		{"mostly non-hex", "helloworld", false},
		{"exactly 6 non-hex", "getall", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isIDSegment(tc.input)
			if got != tc.want {
				t.Errorf("isIDSegment(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestNew_ReturnsCollector(t *testing.T) {
	c := New("1.0.0", "abc", "2024-01-01", time.Now(), "")
	if c == nil {
		t.Fatal("New returned nil")
	}
}

func TestCollector_Handler_NoAuth(t *testing.T) {
	c := New("1.0.0", "abc", "2024-01-01", time.Now(), "")
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestCollector_Handler_WithAuth_Unauthorized(t *testing.T) {
	c := New("1.0.0", "abc", "2024-01-01", time.Now(), "secret")
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestCollector_Handler_WithAuth_Authorized(t *testing.T) {
	c := New("1.0.0", "abc", "2024-01-01", time.Now(), "secret")
	srv := httptest.NewServer(c.Handler())
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/metrics", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer secret")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestCollector_Middleware_SkipsMetricsPath(t *testing.T) {
	c := New("1.0.0", "abc", "2024-01-01", time.Now(), "")

	innerCalled := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		innerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	handler := c.Middleware()(inner)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !innerCalled {
		t.Error("inner handler was not called for /metrics path")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestCollector_Middleware_RecordsRequest(t *testing.T) {
	c := New("1.0.0", "abc", "2024-01-01", time.Now(), "")

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := c.Middleware()(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	// Verify no panic and correct response propagated.
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
