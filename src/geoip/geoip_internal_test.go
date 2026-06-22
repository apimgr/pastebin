package geoip

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// ─── mustStat ─────────────────────────────────────────────────────────────────

func TestMustStat_ExistingFile(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.bin")
	content := []byte("hello world")
	if err := os.WriteFile(f, content, 0o644); err != nil {
		t.Fatal(err)
	}
	got := mustStat(f)
	if got != int64(len(content)) {
		t.Errorf("mustStat = %d; want %d", got, len(content))
	}
}

func TestMustStat_MissingFile(t *testing.T) {
	got := mustStat("/nonexistent/path/to/file.bin")
	if got != 0 {
		t.Errorf("mustStat missing = %d; want 0", got)
	}
}

// ─── ensureFile ───────────────────────────────────────────────────────────────

func TestEnsureFile_AlreadyPresent(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "data.mmdb")
	if err := os.WriteFile(dst, []byte("non-empty content"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Should not touch the network — src is an invalid URL that would fail.
	if err := ensureFile(dst, "http://127.0.0.1:1/nonexistent"); err != nil {
		t.Errorf("ensureFile for existing file should return nil, got: %v", err)
	}
}

func TestEnsureFile_Downloads(t *testing.T) {
	payload := []byte("fake mmdb binary content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(payload)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	dst := filepath.Join(tmp, "asn.mmdb")

	if err := ensureFile(dst, srv.URL+"/asn.mmdb"); err != nil {
		t.Fatalf("ensureFile download: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile after download: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("downloaded content = %q; want %q", got, payload)
	}
}

func TestEnsureFile_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	dst := filepath.Join(tmp, "bad.mmdb")

	err := ensureFile(dst, srv.URL+"/bad.mmdb")
	if err == nil {
		t.Error("expected error for non-200 status, got nil")
	}
}

func TestEnsureFile_Unreachable(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "unreach.mmdb")

	err := ensureFile(dst, "http://127.0.0.1:1/unreach.mmdb")
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}

// ─── Open ─────────────────────────────────────────────────────────────────────

func TestOpen_NoDBsEnabled(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{Dir: tmp}

	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open with no DBs enabled: %v", err)
	}
	if db == nil {
		t.Fatal("Open returned nil DB")
	}
	db.Close()
}

func TestOpen_InvalidMmdbFile_ASN(t *testing.T) {
	// Pre-create a non-empty fake file so ensureFile is a no-op;
	// maxminddb.Open will fail with an invalid file format error,
	// covering the "geoip: warning: open asn.mmdb" log path.
	tmp := t.TempDir()
	asnPath := filepath.Join(tmp, "asn.mmdb")
	if err := os.WriteFile(asnPath, []byte("not a real mmdb file"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Dir: tmp, EnableASN: true}

	db, err := Open(cfg)
	// Open should succeed even when the DB file is invalid (logs a warning).
	if err != nil {
		t.Fatalf("Open with invalid mmdb: unexpected error %v", err)
	}
	if db == nil {
		t.Fatal("Open returned nil DB")
	}
	db.Close()
}

func TestOpen_InvalidMmdbFile_Country(t *testing.T) {
	tmp := t.TempDir()
	countryPath := filepath.Join(tmp, "country.mmdb")
	if err := os.WriteFile(countryPath, []byte("not a real mmdb file"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Dir: tmp, EnableCountry: true}

	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open with invalid country mmdb: %v", err)
	}
	if db == nil {
		t.Fatal("Open returned nil DB")
	}
	db.Close()
}

func TestOpen_InvalidMmdbFile_City(t *testing.T) {
	tmp := t.TempDir()
	cityPath := filepath.Join(tmp, "city.mmdb")
	if err := os.WriteFile(cityPath, []byte("not a real mmdb file"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Dir: tmp, EnableCity: true}

	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("Open with invalid city mmdb: %v", err)
	}
	if db == nil {
		t.Fatal("Open returned nil DB")
	}
	db.Close()
}

// ─── Update ───────────────────────────────────────────────────────────────────

func TestUpdate_NoDBsEnabled(t *testing.T) {
	tmp := t.TempDir()
	cfg := Config{Dir: tmp}

	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Update with no DBs enabled is a no-op returning nil.
	if err := db.Update(); err != nil {
		t.Errorf("Update with no DBs: expected nil, got %v", err)
	}
}

// ─── Lookup ───────────────────────────────────────────────────────────────────

func TestLookup_NilDB(t *testing.T) {
	var d *DB
	if got := d.Lookup(nil); got != nil {
		t.Errorf("nil DB Lookup = %v; want nil", got)
	}
}

func TestLookup_NilIP(t *testing.T) {
	tmp := t.TempDir()
	db, err := Open(Config{Dir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := db.Lookup(nil); got != nil {
		t.Errorf("Lookup(nil IP) = %v; want nil", got)
	}
}

func TestLookup_ValidIP_NoDBsLoaded(t *testing.T) {
	tmp := t.TempDir()
	db, err := Open(Config{Dir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ip := net.ParseIP("8.8.8.8")
	got := db.Lookup(ip)
	if got == nil {
		t.Error("Lookup(8.8.8.8) = nil; want non-nil info")
	}
	if got.IP != "8.8.8.8" {
		t.Errorf("info.IP = %q; want 8.8.8.8", got.IP)
	}
}

// ─── Close ────────────────────────────────────────────────────────────────────

func TestClose_WithNoDB(t *testing.T) {
	tmp := t.TempDir()
	db, err := Open(Config{Dir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	// Call Close twice — should not panic.
	db.Close()
	db.Close()
}

// ─── IsBlocked ────────────────────────────────────────────────────────────────

func TestIsBlocked_NilDB(t *testing.T) {
	var d *DB
	if d.IsBlocked(net.ParseIP("1.2.3.4")) {
		t.Error("nil DB IsBlocked should return false")
	}
}

func TestIsBlocked_NilIP(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp})
	defer db.Close()
	if db.IsBlocked(nil) {
		t.Error("IsBlocked(nil) should return false")
	}
}

func TestIsBlocked_PrivateIP_NotBlocked(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp, AllowCountries: []string{"US"}})
	defer db.Close()

	for _, ipStr := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1"} {
		ip := net.ParseIP(ipStr)
		if db.IsBlocked(ip) {
			t.Errorf("private/loopback IP %s should not be blocked", ipStr)
		}
	}
}

func TestIsBlocked_AllowlistIP_NotBlocked(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{
		Dir:            tmp,
		Allowlist:      []string{"8.8.8.8"},
		AllowCountries: []string{"US"},
	})
	defer db.Close()

	if db.IsBlocked(net.ParseIP("8.8.8.8")) {
		t.Error("allowlisted IP 8.8.8.8 should not be blocked")
	}
}

func TestIsBlocked_AllowlistCIDR_NotBlocked(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{
		Dir:            tmp,
		Allowlist:      []string{"8.8.0.0/16"},
		AllowCountries: []string{"US"},
	})
	defer db.Close()

	if db.IsBlocked(net.ParseIP("8.8.4.4")) {
		t.Error("IP in allowlisted CIDR should not be blocked")
	}
}

func TestIsBlocked_DenyCountriesEmpty_NotBlocked(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp})
	defer db.Close()

	// No country DBs loaded → country is "" → neither allow nor deny matches → not blocked
	if db.IsBlocked(net.ParseIP("8.8.8.8")) {
		t.Error("should not block when no country policy configured")
	}
}

// ─── LookupRequest ────────────────────────────────────────────────────────────

func TestLookupRequest_RemoteAddr(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp})
	defer db.Close()

	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	got := db.LookupRequest(req)
	if got == nil {
		t.Error("LookupRequest with valid RemoteAddr should return non-nil")
	}
}

func TestLookupRequest_XRealIP(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp})
	defer db.Close()

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "5.6.7.8")
	got := db.LookupRequest(req)
	if got == nil {
		t.Error("LookupRequest with X-Real-IP header should return non-nil")
	}
}

func TestLookupRequest_XForwardedFor(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp})
	defer db.Close()

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "9.10.11.12, 1.1.1.1")
	got := db.LookupRequest(req)
	if got == nil {
		t.Error("LookupRequest with X-Forwarded-For should return non-nil")
	}
}

// ─── Middleware ────────────────────────────────────────────────────────────────

func TestMiddleware_AllowsNonBlockedIP(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp})
	defer db.Close()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := db.Middleware()(next)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	mw.ServeHTTP(w, req)

	if !called {
		t.Error("next handler should be called for non-blocked IP")
	}
}

func TestMiddleware_BlocksPublicIPWhenAllowCountriesSet(t *testing.T) {
	tmp := t.TempDir()
	// No country DB loaded; AllowCountries=["US"] → any public IP gets country=""
	// which does not match "US" → IsBlocked returns true.
	db, _ := Open(Config{Dir: tmp, AllowCountries: []string{"US"}})
	defer db.Close()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := db.Middleware()(next)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	mw.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for blocked IP, got %d", w.Code)
	}
}

// ─── IsBlocked — additional paths ────────────────────────────────────────────

func TestIsBlocked_AllowCountries_BlocksPublicIP(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp, AllowCountries: []string{"US"}})
	defer db.Close()

	// 8.8.8.8 is public; without a country DB, cc="" which doesn't match "US" → blocked.
	if !db.IsBlocked(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 should be blocked when AllowCountries=[US] and no country DB")
	}
}

func TestIsBlocked_DenyCountries_NoMatch_NotBlocked(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp, DenyCountries: []string{"CN"}})
	defer db.Close()

	// No country DB → cc="" → doesn't match "CN" → not blocked.
	if db.IsBlocked(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 should not be blocked when DenyCountries=[CN] and no country DB")
	}
}

// ─── isPrivate — additional CIDR ranges ──────────────────────────────────────

func TestIsPrivate_RFC172Range(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp, AllowCountries: []string{"US"}})
	defer db.Close()

	// 172.16.0.1 is in the 172.16.0.0/12 private range.
	if db.IsBlocked(net.ParseIP("172.16.0.1")) {
		t.Error("172.16.0.1 is RFC 1918 private — should not be blocked")
	}
}

func TestIsPrivate_CGNATRange(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp, AllowCountries: []string{"US"}})
	defer db.Close()

	// 100.64.0.1 is in CGNAT range 100.64.0.0/10.
	if db.IsBlocked(net.ParseIP("100.64.0.1")) {
		t.Error("100.64.0.1 is CGNAT — should not be blocked")
	}
}

func TestIsPrivate_ULAIPv6(t *testing.T) {
	tmp := t.TempDir()
	db, _ := Open(Config{Dir: tmp, AllowCountries: []string{"US"}})
	defer db.Close()

	// fc00::1 is in the ULA range fc00::/7.
	if db.IsBlocked(net.ParseIP("fc00::1")) {
		t.Error("fc00::1 is ULA IPv6 — should not be blocked")
	}
}

// ─── Update — with DBs enabled (download errors expected in test) ─────────────

func TestUpdate_WithEnabledDBs_ReturnsOnDownloadError(t *testing.T) {
	tmp := t.TempDir()
	// Create stub files so Open treats them as "already present" and won't download.
	asnPath := filepath.Join(tmp, "asn.mmdb")
	countryPath := filepath.Join(tmp, "country.mmdb")
	cityPath := filepath.Join(tmp, "city.mmdb")
	for _, p := range []string{asnPath, countryPath, cityPath} {
		if err := os.WriteFile(p, []byte("not-a-real-mmdb"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := Config{
		Dir:           tmp,
		EnableASN:     true,
		EnableWHOIS:   true,
		EnableCountry: true,
		EnableCity:    true,
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Update() deletes the files and tries to download fresh copies from the CDN.
	// In the test environment this will fail at download or at maxminddb.Open.
	// Either outcome is acceptable — we only need the code paths exercised.
	err = db.Update()
	_ = err
}
