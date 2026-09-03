//go:build e2e

// Package e2e contains browser/end-to-end tests for pastebin (AI.md PART 28,
// "Browser E2E Testing"). This file is compiled ONLY under the `e2e` build
// tag, so `make test` (the pre-commit gate) never runs it — it is invoked
// on-demand via tests/e2e.sh against a running instance (E2E_BASE_URL).
//
// Three Mandatory E2E Tiers per PART 28:
//   Tier 1 — SSR:          plain net/http request/response inspection
//   Tier 2 — No-JS browser: chromedp with script execution disabled
//   Tier 3 — Full browser:  chromedp with JS on, zero console errors
//
// NOTE: Tier 2 and Tier 3 require github.com/chromedp/chromedp, which is not
// yet a dependency of this module. Adding it means editing the shared
// go.mod/go.sum while many other automated passes are concurrently working
// in this repository, and it cannot be verified here (no Go toolchain on the
// host, per AI.md/testing-rules.md). That addition is deliberately deferred
// to its own isolated, verifiable commit rather than risked here. Only Tier 1
// is implemented in this file today; TestTier2NoJS and TestTier3FullBrowser
// are stubbed out with t.Skip so the suite's shape matches the spec and the
// tags/names are ready for the chromedp implementation to fill in.
package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// baseURL returns the E2E_BASE_URL the running stack is reachable at,
// set by tests/e2e.sh before invoking `go test -tags e2e`.
func baseURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("E2E_BASE_URL")
	if u == "" {
		t.Skip("E2E_BASE_URL not set — run via tests/e2e.sh against a live stack")
	}
	return strings.TrimRight(u, "/")
}

func client() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// TestTier1Healthz verifies /server/healthz responds and is not redirected.
func TestTier1Healthz(t *testing.T) {
	base := baseURL(t)
	resp, err := client().Get(base + "/server/healthz")
	if err != nil {
		t.Fatalf("GET /server/healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestTier1HomeNoJS verifies the home page renders usable server-side HTML
// (create form present) without requiring JavaScript to execute — this is
// the SSR baseline that Tier 2 (No-JS chromedp) will later exercise inside
// an actual browser engine.
func TestTier1HomeNoJS(t *testing.T) {
	base := baseURL(t)
	resp, err := client().Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	html := string(body)
	if !strings.Contains(html, "<form") {
		t.Error("home page has no <form> — create form must work without JS")
	}
	if strings.Contains(html, "<script") && !strings.Contains(html, "static/js/app.js") {
		t.Error("home page references JS other than the single app.js bundle")
	}
}

// TestTier1PasteLifecycle exercises create -> view -> raw -> delete via the
// native JSON API end to end (AI.md IDEA.md API Endpoints table).
func TestTier1PasteLifecycle(t *testing.T) {
	base := baseURL(t)
	c := client()

	createBody := strings.NewReader(`{"content":"e2e tier1 content","title":"e2e","language":"text","expiry":"1h"}`)
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/pastes", createBody)
	if err != nil {
		t.Fatalf("build create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/pastes: %v", err)
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read create response: %v", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 200/201, body=%s", resp.StatusCode, rawBody)
	}

	var created struct {
		OK   bool `json:"ok"`
		Data struct {
			ID         string `json:"id"`
			OwnerToken string `json:"owner_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &created); err != nil {
		t.Fatalf("decode create response: %v (body=%s)", err, rawBody)
	}
	if created.Data.ID == "" {
		t.Fatalf("create response missing id: %s", rawBody)
	}
	id := created.Data.ID

	// View metadata + content.
	viewResp, err := c.Get(base + "/api/v1/pastes/" + id)
	if err != nil {
		t.Fatalf("GET /api/v1/pastes/%s: %v", id, err)
	}
	defer viewResp.Body.Close()
	if viewResp.StatusCode != http.StatusOK {
		t.Errorf("view status = %d, want 200", viewResp.StatusCode)
	}

	// Raw content never redirects.
	rawResp, err := c.Get(base + "/" + id + "/raw")
	if err != nil {
		t.Fatalf("GET /%s/raw: %v", id, err)
	}
	defer rawResp.Body.Close()
	if rawResp.StatusCode != http.StatusOK {
		t.Errorf("raw status = %d, want 200", rawResp.StatusCode)
	}
	rawText, _ := io.ReadAll(rawResp.Body)
	if !strings.Contains(string(rawText), "e2e tier1 content") {
		t.Errorf("raw content mismatch: %s", rawText)
	}

	// Owner-token delete, if the API returned one.
	if created.Data.OwnerToken != "" {
		delReq, err := http.NewRequest(http.MethodDelete, base+"/api/v1/pastes/"+id, nil)
		if err != nil {
			t.Fatalf("build delete request: %v", err)
		}
		delReq.Header.Set("Authorization", "Bearer "+created.Data.OwnerToken)
		delResp, err := c.Do(delReq)
		if err != nil {
			t.Fatalf("DELETE /api/v1/pastes/%s: %v", id, err)
		}
		defer delResp.Body.Close()
		if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
			t.Errorf("delete status = %d, want 200/204", delResp.StatusCode)
		}
	}
}

// TestTier1UnknownID verifies an unknown paste ID returns 404 via both the
// web route and the API route, never a 500 or a redirect to a login page
// that doesn't exist in this project.
func TestTier1UnknownID(t *testing.T) {
	base := baseURL(t)
	c := client()

	const bogus = "0000000000000000000000000000zz"

	webResp, err := c.Get(base + "/" + bogus)
	if err != nil {
		t.Fatalf("GET /%s: %v", bogus, err)
	}
	defer webResp.Body.Close()
	if webResp.StatusCode != http.StatusNotFound {
		t.Errorf("web unknown-id status = %d, want 404", webResp.StatusCode)
	}

	apiResp, err := c.Get(base + "/api/v1/pastes/" + bogus)
	if err != nil {
		t.Fatalf("GET /api/v1/pastes/%s: %v", bogus, err)
	}
	defer apiResp.Body.Close()
	if apiResp.StatusCode != http.StatusNotFound {
		t.Errorf("api unknown-id status = %d, want 404", apiResp.StatusCode)
	}
}

// TestTier2NoJS is a placeholder for the chromedp-based No-JS tier described
// in AI.md PART 28. Implement once github.com/chromedp/chromedp is added to
// go.mod in its own isolated commit.
func TestTier2NoJS(t *testing.T) {
	t.Skip("Tier 2 (No-JS chromedp) pending github.com/chromedp/chromedp dependency addition")
}

// TestTier3FullBrowser is a placeholder for the chromedp-based full-browser
// tier (JS on, zero console errors) described in AI.md PART 28. Implement
// once github.com/chromedp/chromedp is added to go.mod in its own isolated
// commit.
func TestTier3FullBrowser(t *testing.T) {
	t.Skip("Tier 3 (full browser chromedp) pending github.com/chromedp/chromedp dependency addition")
}
