package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── AuthStubRedirect ─────────────────────────────────────────────────────────

func TestAuthStubRedirect(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rr := httptest.NewRecorder()

	AuthStubRedirect(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("AuthStubRedirect: got status %d, want 302", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("AuthStubRedirect: Location = %q; want /", loc)
	}
}
