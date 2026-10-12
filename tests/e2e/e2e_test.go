//go:build e2e

// Package e2e contains browser/end-to-end tests for pastebin (AI.md PART 28).
// This file is compiled only with the e2e build tag and targets the live test
// stack started by tests/e2e.sh.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
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

func csrfClient(t *testing.T, base string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Timeout: 15 * time.Second, Jar: jar}
	resp, err := getHTML(c, base+"/create")
	if err != nil {
		t.Fatalf("GET /create for CSRF token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /create for CSRF token returned %d", resp.StatusCode)
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		t.Fatalf("parse create form: %v", err)
	}
	var token string
	var find func(*html.Node)
	find = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "input" {
			name, value := "", ""
			for _, attr := range node.Attr {
				if attr.Key == "name" {
					name = attr.Val
				}
				if attr.Key == "value" {
					value = attr.Val
				}
			}
			if name == "csrf_token" {
				token = value
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			find(child)
		}
	}
	find(doc)
	if token == "" {
		t.Fatal("create form omitted CSRF token")
	}
	return c, token
}

func addCSRFToken(req *http.Request, token string) {
	req.Header.Set("X-CSRF-Token", token)
}

func getHTML(c *http.Client, endpoint string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html")
	return c.Do(req)
}

func createPaste(t *testing.T, base string, payload map[string]interface{}) (string, string) {
	t.Helper()
	c, csrfToken := csrfClient(t, base)
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/pastes", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 pastebin-e2e")
	addCSRFToken(req, csrfToken)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("create paste: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("create paste status = %d, body=%s", resp.StatusCode, body)
	}
	var result struct {
		Data struct {
			ID         string `json:"id"`
			OwnerToken string `json:"owner_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Data.ID == "" {
		t.Fatalf("create response lacks paste id: %s (%v)", body, err)
	}
	return result.Data.ID, result.Data.OwnerToken
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

func TestTier1BrowserCacheRefresh(t *testing.T) {
	base := baseURL(t)
	request, _ := http.NewRequest(http.MethodGet, base+"/", nil)
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Cookie", "pastebin_build=stale-build")
	resp, err := client().Do(request)
	if err != nil {
		t.Fatalf("GET / with stale build cookie: %v", err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Clear-Site-Data"); got != `"cache", "storage"` {
		t.Errorf("stale build Clear-Site-Data = %q", got)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("HTML Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "pastebin_build=") {
		t.Error("HTML response did not refresh the build stamp cookie")
	}

	assetResp, err := client().Get(base + "/static/js/app.js")
	if err != nil {
		t.Fatalf("GET unversioned static asset: %v", err)
	}
	assetResp.Body.Close()
	stamp := strings.Trim(assetResp.Header.Get("ETag"), `"`)
	if assetResp.StatusCode != http.StatusOK || stamp == "" || assetResp.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("unversioned asset response = %d, cache=%q, etag=%q", assetResp.StatusCode, assetResp.Header.Get("Cache-Control"), assetResp.Header.Get("ETag"))
	}
	versionedResp, err := client().Get(base + "/static/js/app.js?v=" + url.QueryEscape(stamp))
	if err != nil {
		t.Fatalf("GET versioned static asset: %v", err)
	}
	versionedResp.Body.Close()
	if versionedResp.StatusCode != http.StatusOK || versionedResp.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("versioned asset response = %d, cache=%q", versionedResp.StatusCode, versionedResp.Header.Get("Cache-Control"))
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
	assets := make(map[string]bool)
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
		if err := validatePageMetadata(doc); err != nil {
			t.Errorf("GET %s: %v", path, err)
		}
		var visit func(*html.Node)
		visit = func(node *html.Node) {
			if node.Type == html.ElementNode {
				for _, attr := range node.Attr {
					isLink := node.Data == "a" && attr.Key == "href"
					isAsset := (node.Data == "script" && attr.Key == "src") || (node.Data == "img" && attr.Key == "src") || (node.Data == "link" && attr.Key == "href")
					if (!isLink && !isAsset) || attr.Val == "" || strings.HasPrefix(attr.Val, "#") {
						continue
					}
					ref, parseErr := url.Parse(attr.Val)
					if parseErr != nil {
						continue
					}
					dest := baseURL.ResolveReference(ref)
					if dest.Scheme == baseURL.Scheme && dest.Host == baseURL.Host {
						if isAsset {
							assetPath := dest.RequestURI()
							if !assets[assetPath] {
								assets[assetPath] = true
								assetResp, assetErr := client().Get(base + assetPath)
								if assetErr != nil {
									t.Errorf("load referenced asset %s: %v", assetPath, assetErr)
								} else {
									assetResp.Body.Close()
									if assetResp.StatusCode != http.StatusOK {
										t.Errorf("referenced asset %s returned %d", assetPath, assetResp.StatusCode)
									}
								}
							}
						} else {
							queue = append(queue, dest.RequestURI())
						}
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

func TestTier1RegisteredGETRoutes(t *testing.T) {
	base := baseURL(t)
	resp, err := client().Get(base + "/debug/routes")
	if err != nil {
		t.Fatalf("GET debug route inventory: %v", err)
	}
	var inventory struct {
		Data struct {
			Routes []struct {
				Method string `json:"method"`
				Route  string `json:"route"`
			} `json:"routes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inventory); err != nil {
		resp.Body.Close()
		t.Fatalf("decode debug route inventory: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(inventory.Data.Routes) == 0 {
		t.Fatalf("debug route inventory status/count = %d/%d", resp.StatusCode, len(inventory.Data.Routes))
	}
	seen := make(map[string]bool)
	for _, route := range inventory.Data.Routes {
		if route.Method != http.MethodGet || strings.Contains(route.Route, "*") {
			continue
		}
		path := route.Route
		for {
			start := strings.IndexByte(path, '{')
			if start < 0 {
				break
			}
			end := strings.IndexByte(path[start:], '}')
			if end < 0 {
				t.Fatalf("malformed route pattern %q", route.Route)
			}
			path = path[:start] + "e2e-unknown" + path[start+end+1:]
		}
		if seen[path] || path == "/__e2e__/panic" {
			continue
		}
		seen[path] = true
		response, err := client().Get(base + path)
		if err != nil {
			t.Errorf("GET registered route %s: %v", path, err)
			continue
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode >= http.StatusInternalServerError {
			t.Errorf("GET registered route %s returned %d", path, response.StatusCode)
		}
	}
}

func TestTier1ThemeAndLocaleRendering(t *testing.T) {
	base := baseURL(t)
	for _, tc := range []struct {
		cookie string
		lang   string
		class  string
	}{
		{"theme=dark", "en", "theme-dark"},
		{"theme=light", "es", "theme-light"},
		{"theme=auto", "fr", "theme-auto"},
	} {
		req, _ := http.NewRequest(http.MethodGet, base+"/?lang="+tc.lang, nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Cookie", tc.cookie)
		resp, err := client().Do(req)
		if err != nil {
			t.Errorf("GET home for %s/%s: %v", tc.cookie, tc.lang, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), tc.class) || !strings.Contains(string(body), `lang="`+tc.lang+`"`) {
			t.Errorf("home page did not render %s and lang %s", tc.class, tc.lang)
		}
	}
}

func validatePageMetadata(doc *html.Node) error {
	var title, language, charset, viewport string
	var inspect func(*html.Node)
	inspect = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "html":
				for _, attr := range node.Attr {
					if attr.Key == "lang" {
						language = strings.TrimSpace(attr.Val)
					}
				}
			case "title":
				if node.FirstChild != nil {
					title = strings.TrimSpace(node.FirstChild.Data)
				}
			case "meta":
				name, content := "", ""
				for _, attr := range node.Attr {
					switch attr.Key {
					case "name":
						name = strings.ToLower(attr.Val)
					case "content":
						content = attr.Val
					case "charset":
						charset = strings.ToLower(attr.Val)
					}
				}
				if name == "viewport" {
					viewport = content
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			inspect(child)
		}
	}
	inspect(doc)
	var missing []string
	if title == "" {
		missing = append(missing, "non-empty <title>")
	}
	if language == "" {
		missing = append(missing, "<html lang>")
	}
	if charset != "utf-8" && charset != "utf8" {
		missing = append(missing, "UTF-8 charset meta")
	}
	if viewport == "" {
		missing = append(missing, "viewport meta")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return nil
}

// TestTier1PasteLifecycle exercises create -> view -> raw -> delete via the
// native JSON API end to end (AI.md IDEA.md API Endpoints table).
func TestTier1PasteLifecycle(t *testing.T) {
	base := baseURL(t)
	c, csrfToken := csrfClient(t, base)

	createBody := strings.NewReader(`{"content":"e2e tier1 content","title":"e2e","language":"text","expiry":"1h"}`)
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/pastes", createBody)
	if err != nil {
		t.Fatalf("build create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 pastebin-e2e")
	addCSRFToken(req, csrfToken)
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

func TestTier1PasteVisibilitySyntaxDownloadEmbedAndQR(t *testing.T) {
	base := baseURL(t)
	id, ownerToken := createPaste(t, base, map[string]interface{}{
		"content": "package main\nfunc main() {}", "title": "e2e-go.go",
		"language": "go", "visibility": "unlisted", "expires_in": "never",
	})
	if ownerToken == "" {
		t.Fatal("create response omitted owner_token")
	}
	c := client()
	listResp, err := c.Get(base + "/api/v1/pastes?limit=250")
	if err != nil {
		t.Fatal(err)
	}
	listBody, _ := io.ReadAll(listResp.Body)
	listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK || strings.Contains(string(listBody), id) {
		t.Fatalf("unlisted paste appeared in recent listing (status %d): %s", listResp.StatusCode, listBody)
	}
	viewResp, err := getHTML(c, base+"/"+id)
	if err != nil {
		t.Fatal(err)
	}
	viewBody, _ := io.ReadAll(viewResp.Body)
	viewResp.Body.Close()
	if viewResp.StatusCode != http.StatusOK || !strings.Contains(string(viewBody), "e2e-go.go") || !strings.Contains(string(viewBody), "<span") {
		t.Fatalf("paste view did not render title and server syntax highlighting (status %d)", viewResp.StatusCode)
	}
	for _, endpoint := range []struct {
		path string
		want string
	}{{"/dl/" + id, "application/octet-stream"}, {"/emb/" + id, "text/html"}, {"/qr/" + id + "/image", "image/png"}} {
		resp, err := c.Get(base + endpoint.path)
		if err != nil {
			t.Errorf("GET %s: %v", endpoint.path, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), endpoint.want) {
			t.Errorf("GET %s status/content-type = %d/%q; want 200/%q", endpoint.path, resp.StatusCode, resp.Header.Get("Content-Type"), endpoint.want)
		}
	}
	delReq, _ := http.NewRequest(http.MethodDelete, base+"/api/v1/pastes/"+id, nil)
	delReq.Header.Set("Authorization", "Bearer "+ownerToken)
	delResp, err := c.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
		t.Errorf("owner-token delete status = %d", delResp.StatusCode)
	}
}

func TestTier1BurnAfterReadDeletesAtThreshold(t *testing.T) {
	base := baseURL(t)
	id, _ := createPaste(t, base, map[string]interface{}{"content": "burn once", "burn_after": 1, "expires_in": "never"})
	first, err := client().Get(base + "/raw/" + id)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first read status = %d, want 200", first.StatusCode)
	}
	second, err := client().Get(base + "/raw/" + id)
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if second.StatusCode != http.StatusNotFound && second.StatusCode != http.StatusGone {
		t.Fatalf("second read status = %d, want 404/410", second.StatusCode)
	}
}

func TestTier1ExpiredPasteReturnsNotFound(t *testing.T) {
	base := baseURL(t)
	id, _ := createPaste(t, base, map[string]interface{}{"content": "expire soon", "expires_in": "1"})
	time.Sleep(2 * time.Second)
	resp, err := client().Get(base + "/raw/" + id)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusGone {
		t.Fatalf("expired paste status = %d, want 404/410", resp.StatusCode)
	}
}

func TestTier1CSRFAndThemedErrorPage(t *testing.T) {
	base := baseURL(t)
	req, _ := http.NewRequest(http.MethodPost, base+"/create", strings.NewReader("content=csrf+probe"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://attacker.invalid")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site form submission status = %d, want 403", resp.StatusCode)
	}
	errorResp, err := getHTML(client(), base+"/missing-e2e-route")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(errorResp.Body)
	errorResp.Body.Close()
	if errorResp.StatusCode != http.StatusNotFound || !strings.Contains(string(body), "theme-") || strings.Contains(strings.ToLower(string(body)), "goroutine ") {
		t.Errorf("404 page did not render as a themed error page (status %d)", errorResp.StatusCode)
	}
}

func TestTier1ThemedInternalServerError(t *testing.T) {
	base := baseURL(t)
	resp, err := getHTML(client(), base+"/__e2e__/panic")
	if err != nil {
		t.Fatalf("GET E2E panic route: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "500") || !strings.Contains(string(body), "theme-") || strings.Contains(strings.ToLower(string(body)), "goroutine ") {
		t.Errorf("500 response was not a themed error page (status %d)", resp.StatusCode)
	}
}

func TestTier1FormValidationErrorsRenderOnServer(t *testing.T) {
	base := baseURL(t)
	c, csrfToken := csrfClient(t, base)
	form := url.Values{"content": {""}, "csrf_token": {csrfToken}}
	req, err := http.NewRequest(http.MethodPost, base+"/create", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 pastebin-e2e")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST invalid create form: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("invalid create form response = %d/%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if !strings.Contains(strings.ToLower(string(body)), "content") {
		t.Errorf("server-rendered validation response omitted the content error: %s", body)
	}
}

func TestTier1InvalidOwnerTokenDoesNotRevealPasteExistence(t *testing.T) {
	base := baseURL(t)
	id, _ := createPaste(t, base, map[string]interface{}{"content": "protected owner-token paste"})
	req, err := http.NewRequest(http.MethodDelete, base+"/api/v1/pastes/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer tok_invalid-owner-token")
	resp, err := client().Do(req)
	if err != nil {
		t.Fatalf("DELETE with invalid owner token: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("invalid owner-token delete status = %d, want 404", resp.StatusCode)
	}
	if strings.Contains(string(body), "protected owner-token paste") || strings.Contains(string(body), id) {
		t.Errorf("invalid owner-token response revealed paste data: %s", body)
	}
}

func TestTier1CompatibilityAndDocsRoutes(t *testing.T) {
	base := baseURL(t)
	c, csrfToken := csrfClient(t, base)
	compatReq, _ := http.NewRequest(http.MethodPost, base+"/documents", strings.NewReader("hastebin compatible content"))
	addCSRFToken(compatReq, csrfToken)
	compatResp, err := c.Do(compatReq)
	if err != nil {
		t.Fatal(err)
	}
	compatBody, _ := io.ReadAll(compatResp.Body)
	compatResp.Body.Close()
	var doc struct {
		Key string `json:"key"`
	}
	if compatResp.StatusCode != http.StatusOK || json.Unmarshal(compatBody, &doc) != nil || doc.Key == "" {
		t.Fatalf("hastebin compatibility response = %d %s", compatResp.StatusCode, compatBody)
	}
	documentResp, err := client().Get(base + "/documents/" + url.PathEscape(doc.Key))
	if err != nil {
		t.Fatalf("GET hastebin document: %v", err)
	}
	var document struct {
		Key  string `json:"key"`
		Data string `json:"data"`
	}
	documentBody, err := io.ReadAll(documentResp.Body)
	documentResp.Body.Close()
	if err != nil || documentResp.StatusCode != http.StatusOK || json.Unmarshal(documentBody, &document) != nil || document.Key != doc.Key || document.Data != "hastebin compatible content" {
		t.Fatalf("hastebin retrieval = %d %s (%v)", documentResp.StatusCode, documentBody, err)
	}
	for _, route := range []string{"/server/docs/swagger", "/server/docs/graphql"} {
		resp, err := getHTML(client(), base+route)
		if err != nil {
			t.Errorf("GET %s: %v", route, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", route, resp.StatusCode)
		}
	}
}

func TestTier1MicrobinRoundTrip(t *testing.T) {
	base := baseURL(t)
	c, csrfToken := csrfClient(t, base)
	const content = "microbin protocol content"
	payload, _ := json.Marshal(map[string]string{"content": content, "syntax": "text"})
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/pasta", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 pastebin-e2e")
	addCSRFToken(req, csrfToken)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("create microbin paste: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var created struct {
		Data struct {
			ID         string `json:"id"`
			OwnerToken string `json:"owner_token"`
		} `json:"data"`
	}
	if (resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK) || json.Unmarshal(body, &created) != nil || created.Data.ID == "" || created.Data.OwnerToken == "" {
		t.Fatalf("microbin create response = %d %s", resp.StatusCode, body)
	}
	getReq, err := http.NewRequest(http.MethodGet, base+"/api/v1/pasta/"+url.PathEscape(created.Data.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	getReq.Header.Set("Accept", "application/json")
	getReq.Header.Set("User-Agent", "Mozilla/5.0 pastebin-e2e")
	getResp, err := client().Do(getReq)
	if err != nil {
		t.Fatalf("get microbin paste: %v", err)
	}
	var result struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	getBody, err := io.ReadAll(getResp.Body)
	getResp.Body.Close()
	if err != nil || getResp.StatusCode != http.StatusOK || json.Unmarshal(getBody, &result) != nil || result.Data.Content != content {
		t.Fatalf("microbin get response = %d %s (%v)", getResp.StatusCode, getBody, err)
	}
	delReq, _ := http.NewRequest(http.MethodDelete, base+"/api/v1/pasta/"+url.PathEscape(created.Data.ID), nil)
	delReq.Header.Set("Authorization", "Bearer "+created.Data.OwnerToken)
	delResp, err := client().Do(delReq)
	if err != nil {
		t.Fatalf("delete microbin paste: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK && delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("microbin delete status = %d", delResp.StatusCode)
	}
}

func TestTier1ZeroXUploadProtocol(t *testing.T) {
	base := baseURL(t)
	c, csrfToken := csrfClient(t, base)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "e2e.txt")
	if err != nil {
		t.Fatal(err)
	}
	const content = "0x0.st protocol content"
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addCSRFToken(req, csrfToken)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("upload 0x0 file: %v", err)
	}
	responseBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(resp.Header.Get("X-Token")) == "" {
		t.Fatalf("0x0 create response = %d %s", resp.StatusCode, responseBody)
	}
	parsed, err := url.Parse(strings.TrimSpace(string(responseBody)))
	if err != nil || parsed == nil {
		t.Fatalf("parse 0x0 raw URL %q: %v", responseBody, err)
	}
	rawResp, err := client().Get(base + parsed.Path)
	if err != nil {
		t.Fatalf("get 0x0 raw paste: %v", err)
	}
	raw, _ := io.ReadAll(rawResp.Body)
	rawResp.Body.Close()
	if rawResp.StatusCode != http.StatusOK || string(raw) != content {
		t.Fatalf("0x0 raw response = %d %q, want %q", rawResp.StatusCode, raw, content)
	}
}

func TestTier1TermbinTCPProtocol(t *testing.T) {
	base := baseURL(t)
	baseURL, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(baseURL.Hostname(), "9999"), 5*time.Second)
	if err != nil {
		t.Fatalf("connect to termbin listener: %v", err)
	}
	defer conn.Close()
	const content = "termbin raw TCP protocol content"
	if _, err := io.WriteString(conn, content); err != nil {
		t.Fatalf("write termbin paste: %v", err)
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.CloseWrite(); err != nil {
			t.Fatalf("finish termbin upload: %v", err)
		}
	}
	if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	response, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read termbin URL: %v", err)
	}
	pasteURL, err := url.Parse(strings.TrimSpace(string(response)))
	if err != nil || pasteURL.Scheme != "http" || pasteURL.Host == "" || pasteURL.Path == "" ||
		!strings.HasSuffix(string(response), "\n") {
		t.Fatalf("parse termbin response %q: %v", response, err)
	}
	rawResp, err := client().Get(base + "/raw" + pasteURL.Path)
	if err != nil {
		t.Fatalf("get termbin paste: %v", err)
	}
	raw, _ := io.ReadAll(rawResp.Body)
	rawResp.Body.Close()
	if rawResp.StatusCode != http.StatusOK || string(raw) != content {
		t.Fatalf("termbin raw response = %d %q, want %q", rawResp.StatusCode, raw, content)
	}
}

// TestTier1CompatibilityCreateProtocols covers the form and raw-body protocols
// that expose distinct wire formats instead of the native JSON API.
func TestTier1CompatibilityCreateProtocols(t *testing.T) {
	base := baseURL(t)
	c, csrfToken := csrfClient(t, base)
	tests := []struct {
		name     string
		endpoint string
		form     url.Values
		want     string
	}{
		{name: "pastebin", endpoint: "/api/api_post.php", form: url.Values{"api_paste_code": {"pastebin protocol content"}}, want: "pastebin protocol content"},
		{name: "lenpaste", endpoint: "/api/new", form: url.Values{"body": {"lenpaste protocol content"}}, want: "lenpaste protocol content"},
		{name: "stikked", endpoint: "/api/create", form: url.Values{"text": {"stikked protocol content"}}, want: "stikked protocol content"},
		{name: "dpaste", endpoint: "/api/v2/", form: url.Values{"content": {"dpaste protocol content"}, "format": {"json"}}, want: "dpaste protocol content"},
		{name: "sprunge", endpoint: "/", form: url.Values{"sprunge": {"sprunge protocol content"}}, want: "sprunge protocol content"},
		{name: "ixio", endpoint: "/", form: url.Values{"f:1": {"ixio protocol content"}}, want: "ixio protocol content"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, base+tc.endpoint, strings.NewReader(tc.form.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			addCSRFToken(req, csrfToken)
			resp, err := c.Do(req)
			if err != nil {
				t.Fatalf("POST %s: %v", tc.endpoint, err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("POST %s status = %d, body=%s", tc.endpoint, resp.StatusCode, body)
			}
			var pasteURL string
			if tc.name == "dpaste" || tc.name == "lenpaste" {
				var result struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Fatalf("parse dpaste response: %v; body=%s", err, body)
				}
				pasteURL = result.URL
			} else {
				pasteURL = strings.TrimSpace(string(body))
			}
			parsed, err := url.Parse(pasteURL)
			if err != nil || parsed == nil || parsed.Path == "" {
				t.Fatalf("invalid create response URL %q from body %s: %v", pasteURL, body, err)
			}
			parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
			id := parts[len(parts)-1]
			rawResp, err := client().Get(base + "/raw/" + id)
			if err != nil {
				t.Fatalf("GET raw paste: %v", err)
			}
			raw, err := io.ReadAll(rawResp.Body)
			rawResp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if rawResp.StatusCode != http.StatusOK || string(raw) != tc.want {
				t.Fatalf("raw response = %d %q, want %q", rawResp.StatusCode, raw, tc.want)
			}
		})
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
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, endpoint)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	cleanup := func() {
		if t.Failed() {
			captureFailureArtifacts(t, browserCtx)
		}
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

func captureFailureArtifacts(t *testing.T, ctx context.Context) {
	t.Helper()
	dir := os.Getenv("E2E_ARTIFACT_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "apimgr", "pastebin-e2e-artifacts")
	}
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	name := strings.NewReplacer("/", "_", "\\", "_").Replace(t.Name())
	var screenshot []byte
	var pageHTML string
	if chromedp.Run(ctx, chromedp.CaptureScreenshot(&screenshot), chromedp.OuterHTML("html", &pageHTML)) != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name+".png"), screenshot, 0o600)
	_ = os.WriteFile(filepath.Join(dir, name+".html"), []byte(pageHTML), 0o600)
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
	var ownerToken string
	err = chromedp.Run(ctx,
		chromedp.SendKeys(`#content`, content),
		chromedp.Click(`#paste-form button[type="submit"]`),
	)
	if err != nil {
		t.Fatalf("submit create form without JavaScript: %v", err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	err = chromedp.Run(waitCtx, chromedp.WaitVisible(`.result a.long-string`, chromedp.ByQuery))
	waitCancel()
	if err != nil {
		_ = chromedp.Run(ctx, chromedp.OuterHTML("html", &html))
		t.Fatalf("create paste without JavaScript: %v; page=%s", err, html)
	}
	if err := chromedp.Run(ctx,
		chromedp.AttributeValue(`.result a.long-string`, "href", &viewLink, nil, chromedp.ByQuery),
		chromedp.Text(`#created-token`, &ownerToken, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("read no-JS create result: %v", err)
	}
	viewURL, err := url.Parse(viewLink)
	if err != nil {
		t.Fatalf("parse created paste URL: %v", err)
	}
	err = chromedp.Run(ctx,
		chromedp.Navigate(base+viewURL.EscapedPath()),
		chromedp.WaitVisible(`#paste-code`, chromedp.ByQuery),
		chromedp.OuterHTML(`#paste-code`, &html, chromedp.ByQuery),
	)
	if err != nil {
		t.Fatalf("create and view paste without JavaScript: %v", err)
	}
	if !strings.Contains(html, content) {
		t.Fatalf("no-JS paste view is missing created content: %s", html)
	}
	if strings.TrimSpace(ownerToken) == "" {
		t.Fatal("no-JS create flow did not display the one-time owner token")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base + "/remove/" + viewURL.Path[strings.LastIndex(viewURL.Path, "/")+1:]),
		chromedp.WaitVisible(`form[action^="/remove/"]`, chromedp.ByQuery),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("open owner-token management form without JavaScript: %v", err)
	}
	if !strings.Contains(html, `name="token"`) || !strings.Contains(html, `name="csrf_token"`) {
		t.Fatal("no-JS owner-token form omitted its token or CSRF field")
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
		case *network.EventResponseReceived:
			if e.Type != network.ResourceTypeDocument && e.Response.Status >= 400 {
				failure = fmt.Sprintf("%s returned %v", e.Response.URL, e.Response.Status)
			}
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
	var englishTitle, spanishTitle, localeHTML string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/create?lang=en"),
		chromedp.Evaluate(`document.querySelector(".section-title").textContent.trim()`, &englishTitle),
		chromedp.Navigate(base+"/create?lang=es"),
		chromedp.Evaluate(`document.querySelector(".section-title").textContent.trim()`, &spanishTitle),
		chromedp.OuterHTML("html", &localeHTML),
	); err != nil {
		t.Errorf("switch browser locale: %v", err)
	} else if englishTitle == "" || spanishTitle == "" || englishTitle == spanishTitle || !strings.Contains(localeHTML, `lang="es"`) {
		t.Errorf("browser locale did not change a rendered string: en=%q es=%q", englishTitle, spanishTitle)
	}
	var darkBackground, lightBackground string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.cookie = "theme=dark; path=/"`, nil),
		chromedp.Navigate(base + "/"),
		chromedp.Evaluate(`getComputedStyle(document.documentElement).getPropertyValue("--color-bg").trim()`, &darkBackground),
		chromedp.Evaluate(`document.cookie = "theme=light; path=/"`, nil),
		chromedp.Navigate(base + "/"),
		chromedp.Evaluate(`getComputedStyle(document.documentElement).getPropertyValue("--color-bg").trim()`, &lightBackground),
	); err != nil {
		t.Errorf("read browser theme styles: %v", err)
	} else if darkBackground == "" || lightBackground == "" || darkBackground == lightBackground {
		t.Errorf("dark and light theme styles did not differ: dark=%q light=%q", darkBackground, lightBackground)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/remove/e2e-validation")); err != nil {
		t.Fatalf("open owner-token form in browser: %v", err)
	}
	if err := chromedp.Run(ctx, chromedp.WaitVisible(`#token`, chromedp.ByQuery)); err != nil {
		t.Fatalf("wait for owner-token form in browser: %v", err)
	}
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.querySelector("#token").form.requestSubmit()`, nil),
	); err != nil {
		t.Fatalf("submit empty owner-token form in browser: %v", err)
	}
	var jsValidation string
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`(() => { const token = document.querySelector("#token"); const error = document.querySelector("#token-error"); return JSON.stringify({required:token.required, valid:token.form.checkValidity(), ariaInvalid:token.getAttribute("aria-invalid"), errorVisible:!error.hidden}); })()`, &jsValidation),
	); err != nil {
		t.Errorf("check client-side form validation: %v", err)
	} else if !strings.Contains(jsValidation, `"required":true`) || !strings.Contains(jsValidation, `"valid":false`) || !strings.Contains(jsValidation, `"ariaInvalid":"true"`) || !strings.Contains(jsValidation, `"errorVisible":true`) {
		t.Errorf("browser form validation is not active: %s", jsValidation)
	}
	var notFoundHTML string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/missing-e2e-browser-route"),
		chromedp.OuterHTML("html", &notFoundHTML),
	); err != nil {
		t.Errorf("load themed 404 page in browser: %v", err)
	} else if !strings.Contains(notFoundHTML, "404") || !strings.Contains(notFoundHTML, "theme-") || strings.Contains(strings.ToLower(notFoundHTML), "goroutine ") {
		t.Errorf("browser 404 page was not themed or exposed a stack trace")
	}
	const browserFormContent = "e2e tier 3 browser form paste"
	var formPasteLink string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/create"),
		chromedp.WaitVisible(`#paste-form`, chromedp.ByQuery),
		chromedp.SendKeys(`#content`, browserFormContent),
		chromedp.Click(`#paste-form button[type="submit"]`),
		chromedp.WaitVisible(`.result a.long-string`, chromedp.ByQuery),
		chromedp.AttributeValue(`.result a.long-string`, "href", &formPasteLink, nil, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("create paste through full browser form: %v", err)
	}
	formPasteURL, err := url.Parse(formPasteLink)
	if err != nil {
		t.Fatalf("parse full browser create link %q: %v", formPasteLink, err)
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+formPasteURL.EscapedPath()),
		chromedp.WaitVisible(`#paste-code`, chromedp.ByQuery),
		chromedp.Text(`#paste-code`, &html, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("view paste created through full browser form: %v", err)
	} else if !strings.Contains(html, browserFormContent) {
		t.Errorf("browser-created paste view omitted its content: %q", html)
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
	baseParsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse browser crawl base URL: %v", err)
	}
	queue := []string{"/"}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		page := queue[0]
		queue = queue[1:]
		if visited[page] {
			continue
		}
		visited[page] = true
		if len(visited) > 200 {
			t.Fatal("full browser internal-link crawl exceeded 200 pages")
		}
		var links []string
		if err := chromedp.Run(ctx,
			chromedp.Navigate(base+page),
			chromedp.WaitReady("body", chromedp.ByQuery),
			chromedp.Evaluate(`Array.from(document.querySelectorAll('a[href]')).map(a => a.href)`, &links),
		); err != nil {
			t.Errorf("browser crawl %s: %v", page, err)
			continue
		}
		for _, link := range links {
			ref, parseErr := url.Parse(link)
			if parseErr != nil {
				t.Errorf("parse browser link %q from %s: %v", link, page, parseErr)
				continue
			}
			dest := baseParsed.ResolveReference(ref)
			if dest.Scheme == baseParsed.Scheme && dest.Host == baseParsed.Host {
				queue = append(queue, dest.RequestURI())
			}
		}
	}
	const browserPasteContent = "e2e full browser rendered paste content"
	pasteID, _ := createPaste(t, base, map[string]interface{}{"content": browserPasteContent, "syntax": "text"})
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"+pasteID),
		chromedp.WaitVisible(`#paste-code`, chromedp.ByQuery),
		chromedp.Text(`#paste-code`, &html, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load created paste in full browser: %v", err)
	}
	if !strings.Contains(html, browserPasteContent) {
		t.Errorf("full browser paste content = %q, want it to contain %q", strings.TrimSpace(html), browserPasteContent)
	}
	var overflowDetails string
	if err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(375, 812, 1, false),
		chromedp.Navigate(base+"/"),
		chromedp.Evaluate(`JSON.stringify(Array.from(document.querySelectorAll('body *')).filter(el => !(el.closest('.nav-panel') && !document.querySelector('.nav-checkbox:checked'))).map(el => { const r = el.getBoundingClientRect(); return {tag: el.tagName, id: el.id, className: typeof el.className === 'string' ? el.className : '', left: Math.round(r.left), right: Math.round(r.right), width: Math.round(r.width), scrollWidth: el.scrollWidth, clientWidth: el.clientWidth}; }).filter(el => el.right > window.innerWidth || el.left < 0).slice(0, 20))`, &overflowDetails),
	); err != nil {
		t.Errorf("check mobile layout: %v", err)
	} else if overflowDetails != "[]" {
		t.Errorf("home page has horizontal overflow at 375px viewport: %s", overflowDetails)
	}
	failuresMu.Lock()
	defer failuresMu.Unlock()
	if len(failures) > 0 {
		t.Errorf("browser reported console or network errors: %s", strings.Join(failures, "; "))
	}
}

func TestTier3ThemedInternalServerError(t *testing.T) {
	base := baseURL(t)
	ctx, cancel := browserContext(t)
	defer cancel()
	var pageHTML string
	if err := chromedp.Run(ctx,
		network.Enable(),
		chromedp.Navigate(base+"/__e2e__/panic"),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.OuterHTML("html", &pageHTML),
	); err != nil {
		t.Fatalf("load 500 page in browser: %v", err)
	}
	if !strings.Contains(pageHTML, "500") || !strings.Contains(pageHTML, "theme-") || strings.Contains(strings.ToLower(pageHTML), "goroutine ") {
		t.Fatal("browser 500 response was not a themed error page")
	}
}
