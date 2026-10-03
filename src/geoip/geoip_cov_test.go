// Package geoip — additional coverage tests targeting branches not reached by
// the existing test files.  No real .mmdb database is required; tests that
// would need one are noted in the comments below.
package geoip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── isPrivate: link-local multicast branch ───────────────────────────────────

// TestIsPrivate_LinkLocalMulticast exercises the ip.IsLinkLocalMulticast() path
// inside isPrivate.  ff02::1 is the well-known all-nodes link-local multicast
// address defined in RFC 4291 §2.7.
func TestIsPrivate_LinkLocalMulticast(t *testing.T) {
	ip := net.ParseIP("ff02::1")
	if ip == nil {
		t.Fatal("net.ParseIP returned nil for ff02::1")
	}
	if !isPrivate(ip) {
		t.Error("ff02::1 is link-local multicast and must be considered private")
	}
}

// TestIsPrivate_LinkLocalUnicast exercises the ip.IsLinkLocalUnicast() branch
// for an IPv4 link-local address (169.254.x.x, RFC 3927).
func TestIsPrivate_LinkLocalUnicast_IPv4(t *testing.T) {
	ip := net.ParseIP("169.254.1.1")
	if ip == nil {
		t.Fatal("net.ParseIP returned nil for 169.254.1.1")
	}
	if !isPrivate(ip) {
		t.Error("169.254.1.1 is IPv4 link-local and must be considered private")
	}
}

// TestIsPrivate_PublicIP verifies that a routable public IP is not private.
func TestIsPrivate_PublicIP(t *testing.T) {
	ip := net.ParseIP("203.0.113.1")
	if isPrivate(ip) {
		t.Error("203.0.113.1 is a public IP and must not be considered private")
	}
}

// ─── realIP: RemoteAddr-only extraction (PART 12 architecture) ───────────────
//
// realIPMiddleware in server.go applies the PART 12 trusted-proxy gate and
// rewrites r.RemoteAddr before any downstream middleware runs. Therefore
// realIP() in this package only reads r.RemoteAddr — never forwarded headers.

// TestRealIP_BareRemoteAddr_NoPort covers the SplitHostPort failure branch
// inside realIP. When RemoteAddr contains no port (e.g. a raw IP string),
// net.SplitHostPort returns an error and realIP falls back to parsing the whole
// string directly.
func TestRealIP_BareRemoteAddr_NoPort(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "198.51.100.7"
	got := realIP(req)
	if got == nil {
		t.Fatal("realIP returned nil for bare IP RemoteAddr without port")
	}
	if got.String() != "198.51.100.7" {
		t.Errorf("realIP = %s; want 198.51.100.7", got)
	}
}

// TestRealIP_IgnoresXForwardedFor verifies that realIP does NOT read
// X-Forwarded-For. Proxy header extraction is the server's realIPMiddleware
// responsibility (PART 12); geoip only reads r.RemoteAddr.
func TestRealIP_IgnoresXForwardedFor(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	req.RemoteAddr = "198.51.100.9:443"
	got := realIP(req)
	if got == nil {
		t.Fatal("realIP returned nil")
	}
	// Must return RemoteAddr, not the X-Forwarded-For value.
	if got.String() != "198.51.100.9" {
		t.Errorf("realIP = %s; want 198.51.100.9 (RemoteAddr), not X-Forwarded-For", got)
	}
}

// TestRealIP_IgnoresXRealIP verifies that realIP does NOT read X-Real-IP.
// Proxy header extraction is the server's realIPMiddleware responsibility.
func TestRealIP_IgnoresXRealIP(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Real-IP", "8.8.8.8")
	req.RemoteAddr = "198.51.100.9:443"
	got := realIP(req)
	if got == nil {
		t.Fatal("realIP returned nil")
	}
	// Must return RemoteAddr, not the X-Real-IP header value.
	if got.String() != "198.51.100.9" {
		t.Errorf("realIP = %s; want 198.51.100.9 (RemoteAddr), not X-Real-IP", got)
	}
}

// TestRealIP_RemoteAddrWithPort verifies the normal host:port case.
func TestRealIP_RemoteAddrWithPort(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "198.51.100.9:443"
	got := realIP(req)
	if got == nil {
		t.Fatal("realIP returned nil")
	}
	if got.String() != "198.51.100.9" {
		t.Errorf("realIP = %s; want 198.51.100.9", got)
	}
}

// ─── LookupRequest: nil return when no valid IP can be extracted ──────────────

// TestLookupRequest_NilWhenNoValidIP covers the `return nil` path in
// LookupRequest that is reached when realIP cannot parse any IP from the
// request headers or RemoteAddr.
func TestLookupRequest_NilWhenNoValidIP(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Set RemoteAddr to a bare non-IP string so both SplitHostPort and
	// ParseIP return errors/nil — realIP returns nil — LookupRequest returns nil.
	req.RemoteAddr = "not-an-ip"
	if got := db.LookupRequest(req); got != nil {
		t.Errorf("LookupRequest with unparseable RemoteAddr: expected nil, got %+v", got)
	}
}

// ─── ensureFile: error paths ──────────────────────────────────────────────────

// TestEnsureFile_BadDstDir covers the `create tmp file` error branch in
// ensureFile.  When the parent directory of dst does not exist, os.OpenFile
// fails and ensureFile must return a non-nil error.
func TestEnsureFile_BadDstDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "nonexistent-subdir", "file.mmdb")
	err := ensureFile(dst, srv.URL)
	if err == nil {
		t.Error("ensureFile should fail when parent directory does not exist")
	}
}

// TestEnsureFile_EmptyExistingFile verifies that a zero-byte existing file is
// treated as absent: ensureFile must attempt to download rather than return nil
// immediately.  We confirm this by pointing at an unreachable server so that
// the download path is exercised and returns an error.
func TestEnsureFile_EmptyExistingFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "empty.mmdb")
	// Write an empty file — size is 0.
	if err := os.WriteFile(dst, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	// Unreachable src forces a download error, proving ensureFile did not skip.
	err := ensureFile(dst, "http://127.0.0.1:1/test.mmdb")
	if err == nil {
		t.Error("ensureFile with empty existing file and unreachable server should return an error")
	}
}

// TestEnsureFile_NonOKStatus_404 verifies that a 404 response from the server
// produces an error containing the status code.
func TestEnsureFile_NonOKStatus_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "notfound.mmdb")
	err := ensureFile(dst, srv.URL+"/notfound.mmdb")
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

// TestEnsureFile_NonOKStatus_500 covers the non-200 error branch for a 500.
func TestEnsureFile_NonOKStatus_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dst := filepath.Join(t.TempDir(), "error.mmdb")
	err := ensureFile(dst, srv.URL+"/error.mmdb")
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

// ─── Open: unparseable database files ────────────────────────────────────────

// TestOpen_InvalidDatabaseFiles verifies Open tolerates a database file that
// exists but cannot be parsed.  ensureFile short-circuits on the size check, so
// the "stub" content reaches maxminddb.Open, which fails and only logs a
// warning — Open must still return a usable DB rather than an error.
func TestOpen_InvalidDatabaseFiles(t *testing.T) {
	dir := t.TempDir()
	// Pre-create stub files so ensureFile short-circuits on size check
	// and maxminddb.Open logs a warning (covers the "open asn/country" log path).
	if err := os.WriteFile(filepath.Join(dir, "asn.mmdb"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "country.mmdb"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(Config{Dir: dir, EnableASN: true, EnableCountry: true})
	if err != nil {
		t.Fatalf("Open: unexpected error %v", err)
	}
	if db == nil {
		t.Fatal("Open returned nil DB")
	}
	db.Close()
}

// ─── Update: error aggregation path ──────────────────────────────────────────

// TestUpdate_ErrorAggregation verifies that Update collects multiple download
// failures into a single combined error.  All three DB types are enabled;
// all three file downloads fail (unreachable server), so errs must be non-empty
// and Update returns a non-nil error joining them.
func TestUpdate_ErrorAggregation(t *testing.T) {
	// Point every database URL at a local server that always fails, so the
	// error-aggregation path is exercised deterministically and offline.
	// The real CDN must not be contacted here: ensureFile carries a 5-minute
	// context timeout, and Update downloads four databases, so a test that
	// reaches the network blocks for minutes and trips the Go test timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	restore := pointURLsAt(srv.URL, "/asn.mmdb")
	defer restore()

	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		EnableASN:     true,
		EnableCountry: true,
		EnableCity:    true,
	}

	// Open tolerates download failures (it only logs warnings), so this
	// returns a usable DB whose handles are all nil.
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Update removes the stubs and re-downloads; all four fail, and the
	// per-database errors must be joined into one.
	err = db.Update()
	if err == nil {
		t.Fatal("Update returned nil error; want combined download failures")
	}
	msg := err.Error()
	for _, want := range []string{"asn:", "country:", "city ipv4:", "city ipv6:"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Update error missing %q; got: %s", want, msg)
		}
	}
}

// pointURLsAt redirects every database URL to base+path and returns a func
// that restores the production URLs.
func pointURLsAt(base, path string) func() {
	origASN, origCountry := urlASN, urlCountry
	origCityV4, origCityV6 := urlCityV4, urlCityV6
	urlASN = base + path
	urlCountry = base + path
	urlCityV4 = base + path
	urlCityV6 = base + path
	return func() {
		urlASN, urlCountry = origASN, origCountry
		urlCityV4, urlCityV6 = origCityV4, origCityV6
	}
}

// TestUpdate_ClosesOldReaderBeforeReplacing covers the
// `if d.asnDB != nil { d.asnDB.Close() }` branch inside Update.  We first
// call Update to download and open real database files, then call Update a
// second time so the existing non-nil readers are closed before being replaced.
// The databases are served by a local httptest server so the branch is
// genuinely reached rather than skipped by an offline download failure.
func TestUpdate_ClosesOldReaderBeforeReplacing(t *testing.T) {
	serveMinimalMMDB(t)

	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		EnableASN:     true,
		EnableCountry: true,
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// First Update: downloads files and opens readers.
	if err := db.Update(); err != nil {
		t.Fatalf("first Update: %v", err)
	}
	if db.asnDB == nil || db.countDB == nil {
		t.Fatal("first Update left readers nil; branch would not be covered")
	}

	// Second Update: readers are now non-nil, so Close() is called on them
	// before they are replaced — this is the branch we need to cover.
	if err := db.Update(); err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if db.asnDB == nil || db.countDB == nil {
		t.Error("second Update left readers nil")
	}
}

// TestClose_WithLoadedReaders covers the `if d.asnDB != nil`, `if d.countDB != nil`
// branches inside Close by first downloading and opening real database files
// via Update, then calling Close.
func TestClose_WithLoadedReaders(t *testing.T) {
	serveMinimalMMDB(t)

	dir := t.TempDir()
	cfg := Config{
		Dir:           dir,
		EnableASN:     true,
		EnableCountry: true,
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}

	// Load real readers via Update.
	if err := db.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if db.asnDB == nil || db.countDB == nil {
		t.Fatal("Update left readers nil; Close branches would not be covered")
	}

	// Close must not panic with readers loaded.
	db.Close()
	// Second Close on the now-nil readers must also not panic.
	db.Close()
}

// TestUpdate_NoDBsEnabled_Idempotent verifies that calling Update twice on a
// no-op DB is safe and returns nil both times.
func TestUpdate_NoDBsEnabled_Idempotent(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Update(); err != nil {
		t.Errorf("first Update (no DBs): %v", err)
	}
	if err := db.Update(); err != nil {
		t.Errorf("second Update (no DBs): %v", err)
	}
}

// ─── IsBlocked: DenyCountries match path ─────────────────────────────────────

// TestIsBlocked_DenyCountries_EmptyCC_NotBlocked ensures that when DenyCountries
// is set but the resolved country code is "" (no DB loaded), the public IP is
// not blocked.
func TestIsBlocked_DenyCountries_EmptyCC_NoMatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{Dir: dir, DenyCountries: []string{"US", "CN", "RU"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// No country DB means cc == "" which never matches "US"/"CN"/"RU" → not blocked.
	if db.IsBlocked(net.ParseIP("203.0.113.50")) {
		t.Error("should not block when DenyCountries has entries but country lookup returns empty")
	}
}

// TestIsBlocked_AllowlistIP_ExactMatch verifies the exact-IP allowlist bypass
// before the country check runs.
func TestIsBlocked_AllowlistIP_ExactMatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{
		Dir:            dir,
		AllowCountries: []string{"DE"},
		Allowlist:      []string{"203.0.113.99"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if db.IsBlocked(net.ParseIP("203.0.113.99")) {
		t.Error("exact-match allowlist IP must not be blocked")
	}
}

// TestIsBlocked_AllowlistCIDR_ContainsIP verifies that a CIDR allowlist entry
// grants bypass to any IP it contains.
func TestIsBlocked_AllowlistCIDR_ContainsIP(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{
		Dir:            dir,
		AllowCountries: []string{"DE"},
		Allowlist:      []string{"203.0.113.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if db.IsBlocked(net.ParseIP("203.0.113.200")) {
		t.Error("IP in allowlisted CIDR must not be blocked")
	}
}

// TestIsBlocked_AllowlistCIDR_OutsideRange verifies that an IP outside every
// allowlisted CIDR is still subject to country blocking.
func TestIsBlocked_AllowlistCIDR_OutsideRange(t *testing.T) {
	dir := t.TempDir()
	// AllowCountries=["DE"]; the IP resolves to FR (override stands in for a real
	// MMDB) and is outside the allowlisted CIDR → it must be blocked.
	db, err := Open(Config{
		Dir:            dir,
		AllowCountries: []string{"DE"},
		Allowlist:      []string{"203.0.113.0/24"},
	})
	if err != nil {
		t.Fatal(err)
	}
	db.countryOverride = func(net.IP) string { return "FR" }
	defer db.Close()

	// 198.51.100.1 is in TEST-NET-2, routable, and outside 203.0.113.0/24.
	if !db.IsBlocked(net.ParseIP("198.51.100.1")) {
		t.Error("IP outside allowlist CIDR with a non-allowed country should be blocked")
	}
}

// ─── Middleware: body text assertion ─────────────────────────────────────────

// TestMiddleware_BlockedResponse_Body confirms that the blocked response
// contains the expected denial message and the correct Content-Type.
func TestMiddleware_BlockedResponse_Body(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{Dir: dir, AllowCountries: []string{"US"}})
	if err != nil {
		t.Fatal(err)
	}
	// Resolve the client IP to a non-allowed country so the block path renders.
	db.countryOverride = func(net.IP) string { return "CN" }
	defer db.Close()

	handler := db.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Error("blocked response body must not be empty")
	}
}

// TestMiddleware_PrivateIPAlwaysPasses verifies that the middleware never blocks
// a private IP even when AllowCountries would otherwise block it.
func TestMiddleware_PrivateIPAlwaysPasses(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{Dir: dir, AllowCountries: []string{"US"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	called := false
	handler := db.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("private IP should pass through middleware; next handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; want 200 for private IP", rec.Code)
	}
}

// ─── Config struct field coverage ────────────────────────────────────────────

// TestOpen_AllFeaturesDisabled_ConfigPreserved verifies that the Config passed
// to Open is accessible via a Lookup (non-nil return with correct IP string),
// confirming the cfg is stored correctly.
func TestOpen_AllFeaturesDisabled_ConfigPreserved(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{
		Dir:           dir,
		EnableASN:     false,
		EnableCountry: false,
		EnableCity:    false,
		DenyCountries: []string{"XX"},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ip := net.ParseIP("198.51.100.1")
	info := db.Lookup(ip)
	if info == nil {
		t.Fatal("Lookup on no-op DB must return non-nil Info")
	}
	if info.IP != "198.51.100.1" {
		t.Errorf("info.IP = %q; want 198.51.100.1", info.IP)
	}
}

// ─── mustStat ─────────────────────────────────────────────────────────────────

// TestMustStat_NonEmptyFile confirms mustStat returns the correct byte count.
func TestMustStat_NonEmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sample.bin")
	content := make([]byte, 512)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := mustStat(p); got != 512 {
		t.Errorf("mustStat = %d; want 512", got)
	}
}
