//go:build e2e

// Package e2e contains browser/end-to-end tests for pastebin (AI.md PART 28).
// This file is compiled only with the e2e build tag and targets the live test
// stack started by tests/e2e.sh.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"golang.org/x/net/html"
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

func getHTML(c *http.Client, endpoint string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	return c.Do(req)
}

// TestTier1Healthz verifies /server/healthz responds and is not redirected.
func TestTier1Healthz(t *testing.T) {
	base := baseURL(t)
	resp, err := getHTML(client(), base+"/server/healthz")
	if err != nil {
		t.Fatalf("GET /server/healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

// TestTier1HomeSSR verifies the homepage renders its server-side create link
// and metadata without requiring JavaScript.
func TestTier1HomeSSR(t *testing.T) {
	base := baseURL(t)
	resp, err := getHTML(client(), base+"/")
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
	if !strings.Contains(html, `href="/create"`) {
		t.Error("home page has no server-rendered create link")
	}
	for _, marker := range []string{"<title>", `name="description"`, `name="viewport"`, `lang="en"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("home page is missing required SSR metadata %q", marker)
		}
	}
	if strings.Contains(html, "<script") && !strings.Contains(html, "static/js/app.js") {
		t.Error("home page references JS other than the single app.js bundle")
	}
}

// TestTier1InternalLinkCrawl follows same-origin links and rejects broken
// routes, server errors, and blank HTML pages.
func TestTier1InternalLinkCrawl(t *testing.T) {
	base := baseURL(t)
	baseURL, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse base URL: %v", err)
	}
	queue := []string{"/"}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if visited[path] {
			continue
		}
		visited[path] = true
		if len(visited) > 200 {
			t.Fatal("internal link crawl exceeded 200 pages")
		}
		resp, err := getHTML(client(), base+path)
		if err != nil {
			t.Errorf("GET %s: %v", path, err)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Errorf("read %s: %v", path, readErr)
			continue
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			t.Errorf("GET %s returned %d", path, resp.StatusCode)
			continue
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			continue
		}
		if len(strings.TrimSpace(string(body))) == 0 {
			t.Errorf("GET %s returned blank HTML", path)
			continue
		}
		doc, err := html.Parse(strings.NewReader(string(body)))
		if err != nil {
			t.Errorf("parse HTML at %s: %v", path, err)
			continue
		}
		var visit func(*html.Node)
		visit = func(node *html.Node) {
			if node.Type == html.ElementNode && node.Data == "a" {
				for _, attr := range node.Attr {
					if attr.Key != "href" || attr.Val == "" || strings.HasPrefix(attr.Val, "#") {
						continue
					}
					ref, parseErr := url.Parse(attr.Val)
					if parseErr != nil {
						continue
					}
					dest := baseURL.ResolveReference(ref)
					if dest.Scheme == baseURL.Scheme && dest.Host == baseURL.Host {
						queue = append(queue, dest.RequestURI())
					}
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(doc)
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

func browserContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	endpoint := os.Getenv("E2E_BROWSER_URL")
	if endpoint == "" {
		t.Skip("E2E_BROWSER_URL not set — run via tests/e2e.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, endpoint)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	cleanup := func() {
		browserCancel()
		allocCancel()
		cancel()
	}
	if err := chromedp.Run(browserCtx); err != nil {
		cleanup()
		t.Fatalf("connect to headless browser: %v", err)
	}
	return browserCtx, cleanup
}

// TestTier2NoJS verifies that the server-rendered paste creation form is usable
// in a real browser with script execution disabled.
func TestTier2NoJS(t *testing.T) {
	base := baseURL(t)
	ctx, cancel := browserContext(t)
	defer cancel()
	var html string
	err := chromedp.Run(ctx,
		emulation.SetScriptExecutionDisabled(true),
		chromedp.Navigate(base+"/create"),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		t.Fatalf("load create page without JavaScript: %v", err)
	}
	if !strings.Contains(html, `id="paste-form"`) || !strings.Contains(html, `name="content"`) {
		t.Fatalf("no-JS page is missing the server-rendered paste form")
	}
	content := "e2e browser no javascript paste"
	var viewLink string
	err = chromedp.Run(ctx,
		chromedp.SendKeys(`#content`, content),
		chromedp.Click(`#paste-form button[type="submit"]`),
		chromedp.WaitVisible(`.result a.long-string`, chromedp.ByQuery),
		chromedp.AttributeValue(`.result a.long-string`, "href", &viewLink, nil, chromedp.ByQuery),
		chromedp.Navigate(viewLink),
		chromedp.WaitVisible(`#paste-code`, chromedp.ByQuery),
		chromedp.OuterHTML(`#paste-code`, &html, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("create and view paste without JavaScript: %v", err)
	}
	if !strings.Contains(html, content) {
		t.Fatalf("no-JS paste view is missing created content: %s", html)
	}
}

// TestTier3FullBrowser checks the home page and all resources it loads for
// browser console errors and failed network requests.
func TestTier3FullBrowser(t *testing.T) {
	base := baseURL(t)
	ctx, cancel := browserContext(t)
	defer cancel()
	var failures []string
	var failuresMu sync.Mutex
	chromedp.ListenTarget(ctx, func(event any) {
		var failure string
		switch e := event.(type) {
		case *runtime.EventExceptionThrown:
			failure = e.ExceptionDetails.Text
		case *runtime.EventConsoleAPICalled:
			if e.Type == runtime.APITypeError {
				for _, arg := range e.Args {
					failure += " " + arg.Value.String()
				}
			}
		case *network.EventLoadingFailed:
			failure = e.ErrorText
		}
		if failure != "" {
			failuresMu.Lock()
			failures = append(failures, failure)
			failuresMu.Unlock()
		}
	})
	var html string
	err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(base+"/"),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		t.Fatalf("load home page in browser: %v", err)
	}
	if !strings.Contains(html, `href="/create"`) {
		t.Fatal("home page is missing the server-rendered create link")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/create"),
		chromedp.WaitVisible(`#paste-form`, chromedp.ByQuery),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("load create page in browser: %v", err)
	}
	if !strings.Contains(html, `name="content"`) {
		t.Fatal("create page is missing the server-rendered paste content field")
	}
	for _, page := range []string{"/recent", "/server/about", "/server/help", "/server/privacy", "/server/terms", "/server/docs/swagger", "/server/docs/graphql"} {
		if err := chromedp.Run(ctx,
			chromedp.Navigate(base+page),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Evaluate(`document.title`, &html),
		); err != nil {
			t.Errorf("load %s in browser: %v", page, err)
		} else if strings.TrimSpace(html) == "" {
			t.Errorf("%s rendered without a page title", page)
		}
	}
	var hasOverflow bool
	var overflowDetails string
	if err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(375, 812, 1, false),
		chromedp.Navigate(base+"/"),
		chromedp.Evaluate(`document.documentElement.scrollWidth > window.innerWidth`, &hasOverflow),
		chromedp.Evaluate(`JSON.stringify(Array.from(document.querySelectorAll('body *')).map(el => { const r = el.getBoundingClientRect(); return {tag: el.tagName, id: el.id, className: typeof el.className === 'string' ? el.className : '', left: Math.round(r.left), right: Math.round(r.right), width: Math.round(r.width), scrollWidth: el.scrollWidth, clientWidth: el.clientWidth}; }).filter(el => el.right > window.innerWidth || el.left < 0).slice(0, 20))`, &overflowDetails),
	); err != nil {
		t.Errorf("check mobile layout: %v", err)
	} else if hasOverflow {
		t.Errorf("home page has horizontal overflow at 375px viewport: %s", overflowDetails)
	}
	failuresMu.Lock()
	defer failuresMu.Unlock()
	if len(failures) > 0 {
		t.Errorf("browser reported console or network errors: %s", strings.Join(failures, "; "))
	}
}
