// Package geoip provides GeoIP lookup and country-blocking middleware using
// ip-location-db MMDB files (MIT-licensed, no API key required).
// Databases are downloaded on first use and stored in a configurable directory.
// The package uses maxminddb-golang directly because ip-location-db uses
// non-MaxMind database_type strings that geoip2-golang rejects.
package geoip

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	maxminddb "github.com/oschwald/maxminddb-golang"
)

// Database URLs from sapics/ip-location-db via jsDelivr CDN.
const (
	urlASN     = "https://cdn.jsdelivr.net/npm/@ip-location-db/asn-mmdb/asn.mmdb"
	urlCountry = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb"
	urlCity    = "https://cdn.jsdelivr.net/npm/@ip-location-db/dbip-city-mmdb/dbip-city-ipv4.mmdb"
	urlWHOIS   = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country-mmdb/geo-whois-asn-country.mmdb"
)

// ASNRecord holds ASN lookup fields from the asn.mmdb database.
type ASNRecord struct {
	ASN uint32 `maxminddb:"autonomous_system_number"`
	Org string `maxminddb:"autonomous_system_organization"`
}

// WHOISRecord holds registrant organisation from the whois.mmdb database.
type WHOISRecord struct {
	RegistrantOrg string `maxminddb:"registrant_org"`
}

// CountryRecord holds country lookup fields from the country/whois database.
type CountryRecord struct {
	CountryCode string `maxminddb:"country_code"`
}

// CityRecord holds city lookup fields from the city database.
type CityRecord struct {
	City        string  `maxminddb:"city"`
	CountryCode string  `maxminddb:"country_code"`
	Latitude    float64 `maxminddb:"latitude"`
	Longitude   float64 `maxminddb:"longitude"`
	Postcode    string  `maxminddb:"postcode"`
	State1      string  `maxminddb:"state1"`
	State2      string  `maxminddb:"state2"`
	Timezone    string  `maxminddb:"timezone"`
}

// Info is the combined GeoIP result for a single IP address.
type Info struct {
	IP            string
	CountryCode   string
	City          string
	State1        string
	Timezone      string
	Latitude      float64
	Longitude     float64
	ASN           uint32
	ASNOrg        string
	RegistrantOrg string
}

// Config is consumed by Open; mirrors config.GeoIPConfig.
type Config struct {
	Dir            string
	EnableASN      bool
	EnableCountry  bool
	EnableCity     bool
	EnableWHOIS    bool
	DenyCountries  []string
	AllowCountries []string
	Allowlist      []string // IPs that always bypass country blocking
}

// DB holds open MMDB handles and the blocking configuration.
type DB struct {
	mu      sync.RWMutex
	asnDB   *maxminddb.Reader
	countDB *maxminddb.Reader
	cityDB  *maxminddb.Reader
	whoisDB *maxminddb.Reader
	cfg     Config
	// countryOverride, when non-nil, supplies the country code for an IP in
	// place of the MMDB lookup. It lets tests exercise the country-policy paths
	// without a committed binary database fixture; it is always nil in production.
	countryOverride func(net.IP) string
}

// Open downloads any missing MMDB files and opens the configured databases.
// If no databases are enabled, a no-op DB is returned (all lookups return nil).
func Open(cfg Config) (*DB, error) {
	d := &DB{cfg: cfg}

	if err := os.MkdirAll(cfg.Dir, 0o750); err != nil {
		return nil, fmt.Errorf("geoip: create dir: %w", err)
	}

	if cfg.EnableASN || cfg.EnableWHOIS {
		path := filepath.Join(cfg.Dir, "asn.mmdb")
		if err := ensureFile(path, urlASN); err != nil {
			log.Printf("geoip: warning: ASN database unavailable: %v", err)
		} else {
			r, err := maxminddb.Open(path)
			if err != nil {
				log.Printf("geoip: warning: open asn.mmdb: %v", err)
			} else {
				d.asnDB = r
			}
		}
	}

	if cfg.EnableCountry || cfg.EnableWHOIS {
		path := filepath.Join(cfg.Dir, "country.mmdb")
		if err := ensureFile(path, urlCountry); err != nil {
			log.Printf("geoip: warning: country database unavailable: %v", err)
		} else {
			r, err := maxminddb.Open(path)
			if err != nil {
				log.Printf("geoip: warning: open country.mmdb: %v", err)
			} else {
				d.countDB = r
			}
		}
	}

	if cfg.EnableCity {
		path := filepath.Join(cfg.Dir, "city.mmdb")
		if err := ensureFile(path, urlCity); err != nil {
			log.Printf("geoip: warning: city database unavailable: %v", err)
		} else {
			r, err := maxminddb.Open(path)
			if err != nil {
				log.Printf("geoip: warning: open city.mmdb: %v", err)
			} else {
				d.cityDB = r
			}
		}
	}

	if cfg.EnableWHOIS {
		path := filepath.Join(cfg.Dir, "whois.mmdb")
		if err := ensureFile(path, urlWHOIS); err != nil {
			log.Printf("geoip: warning: WHOIS database unavailable: %v", err)
		} else {
			r, err := maxminddb.Open(path)
			if err != nil {
				log.Printf("geoip: warning: open whois.mmdb: %v", err)
			} else {
				d.whoisDB = r
			}
		}
	}

	return d, nil
}

// Update downloads fresh copies of all configured databases and reopens the
// handles atomically. Intended to be called by the weekly scheduler task.
func (d *DB) Update() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var errs []string

	if d.cfg.EnableASN || d.cfg.EnableWHOIS {
		path := filepath.Join(d.cfg.Dir, "asn.mmdb")
		// Remove so ensureFile always downloads.
		_ = os.Remove(path)
		if err := ensureFile(path, urlASN); err != nil {
			errs = append(errs, fmt.Sprintf("asn: %v", err))
		} else if r, err := maxminddb.Open(path); err != nil {
			errs = append(errs, fmt.Sprintf("open asn: %v", err))
		} else {
			if d.asnDB != nil {
				_ = d.asnDB.Close()
			}
			d.asnDB = r
		}
	}

	if d.cfg.EnableCountry || d.cfg.EnableWHOIS {
		path := filepath.Join(d.cfg.Dir, "country.mmdb")
		_ = os.Remove(path)
		if err := ensureFile(path, urlCountry); err != nil {
			errs = append(errs, fmt.Sprintf("country: %v", err))
		} else if r, err := maxminddb.Open(path); err != nil {
			errs = append(errs, fmt.Sprintf("open country: %v", err))
		} else {
			if d.countDB != nil {
				_ = d.countDB.Close()
			}
			d.countDB = r
		}
	}

	if d.cfg.EnableCity {
		path := filepath.Join(d.cfg.Dir, "city.mmdb")
		_ = os.Remove(path)
		if err := ensureFile(path, urlCity); err != nil {
			errs = append(errs, fmt.Sprintf("city: %v", err))
		} else if r, err := maxminddb.Open(path); err != nil {
			errs = append(errs, fmt.Sprintf("open city: %v", err))
		} else {
			if d.cityDB != nil {
				_ = d.cityDB.Close()
			}
			d.cityDB = r
		}
	}

	if d.cfg.EnableWHOIS {
		path := filepath.Join(d.cfg.Dir, "whois.mmdb")
		_ = os.Remove(path)
		if err := ensureFile(path, urlWHOIS); err != nil {
			errs = append(errs, fmt.Sprintf("whois: %v", err))
		} else if r, err := maxminddb.Open(path); err != nil {
			errs = append(errs, fmt.Sprintf("open whois: %v", err))
		} else {
			if d.whoisDB != nil {
				_ = d.whoisDB.Close()
			}
			d.whoisDB = r
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("geoip update errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Close releases all open database handles.
func (d *DB) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.asnDB != nil {
		_ = d.asnDB.Close()
		d.asnDB = nil
	}
	if d.countDB != nil {
		_ = d.countDB.Close()
		d.countDB = nil
	}
	if d.cityDB != nil {
		_ = d.cityDB.Close()
		d.cityDB = nil
	}
	if d.whoisDB != nil {
		_ = d.whoisDB.Close()
		d.whoisDB = nil
	}
}

// Lookup returns GeoIP information for the given IP address.
// Returns nil if GeoIP is not available or the IP is invalid.
func (d *DB) Lookup(ip net.IP) *Info {
	if d == nil || ip == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()

	info := &Info{IP: ip.String()}

	if d.countDB != nil {
		var rec CountryRecord
		if err := d.countDB.Lookup(ip, &rec); err == nil {
			info.CountryCode = rec.CountryCode
		}
	}

	if d.asnDB != nil {
		var rec ASNRecord
		if err := d.asnDB.Lookup(ip, &rec); err == nil {
			info.ASN = rec.ASN
			info.ASNOrg = rec.Org
		}
	}

	if d.cityDB != nil {
		var rec CityRecord
		if err := d.cityDB.Lookup(ip, &rec); err == nil {
			info.City = rec.City
			if info.CountryCode == "" {
				info.CountryCode = rec.CountryCode
			}
			info.State1 = rec.State1
			info.Timezone = rec.Timezone
			info.Latitude = rec.Latitude
			info.Longitude = rec.Longitude
		}
	}

	if d.whoisDB != nil {
		var rec WHOISRecord
		if err := d.whoisDB.Lookup(ip, &rec); err == nil {
			info.RegistrantOrg = rec.RegistrantOrg
		}
	}

	if d.countryOverride != nil {
		info.CountryCode = d.countryOverride(ip)
	}

	return info
}

// IsBlocked returns true if the IP should be blocked based on country policy.
//
// Blocking logic (from spec PART 19):
//   - RFC 1918 / loopback / link-local IPs are never blocked
//   - IPs in the allowlist always pass
//   - If AllowCountries is non-empty: block unless country is in the list,
//     but fail-open (never block) when the country cannot be determined
//   - Else if DenyCountries is non-empty: block if country is in the list
//   - Both empty: never block
func (d *DB) IsBlocked(ip net.IP) bool {
	if d == nil || ip == nil {
		return false
	}

	// Never block private/loopback/link-local addresses.
	if isPrivate(ip) {
		return false
	}

	// Allowlist bypass.
	ipStr := ip.String()
	for _, a := range d.cfg.Allowlist {
		if a == ipStr {
			return false
		}
		if _, net, err := net.ParseCIDR(a); err == nil && net.Contains(ip) {
			return false
		}
	}

	info := d.Lookup(ip)
	cc := ""
	if info != nil {
		cc = strings.ToUpper(info.CountryCode)
	}

	if len(d.cfg.AllowCountries) > 0 {
		// Fail-open: if the country cannot be determined (DB missing, stale, or
		// lookup failed) never block — GeoIP is a risk signal, not an auth gate
		// (PART 19, AI.md:26584).
		if cc == "" {
			return false
		}
		for _, c := range d.cfg.AllowCountries {
			if strings.ToUpper(c) == cc {
				return false
			}
		}
		return true
	}

	if len(d.cfg.DenyCountries) > 0 {
		for _, c := range d.cfg.DenyCountries {
			if strings.ToUpper(c) == cc {
				return true
			}
		}
	}

	return false
}

// Middleware returns a chi-compatible middleware that blocks requests from
// country-blocked IPs. Must run after the allowlist middleware.
func (d *DB) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if ip != nil && d.IsBlocked(ip) {
				// Log the block without retaining the exact IP (redact last octet).
				info := d.Lookup(ip)
				cc := ""
				if info != nil {
					cc = info.CountryCode
				}
				log.Printf("geoip_block: ip=[redacted], country=%s", cc)
				http.Error(w, "access denied: your region is not permitted", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LookupRequest returns GeoIP info for the remote address of an HTTP request.
func (d *DB) LookupRequest(r *http.Request) *Info {
	ip := realIP(r)
	if ip == nil {
		return nil
	}
	return d.Lookup(ip)
}

// realIP extracts the client IP from X-Real-IP, X-Forwarded-For, or RemoteAddr.
func realIP(r *http.Request) net.IP {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		if ip := net.ParseIP(strings.TrimSpace(v)); ip != nil {
			return ip
		}
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// Take the leftmost (client) IP.
		parts := strings.SplitN(v, ",", 2)
		if ip := net.ParseIP(strings.TrimSpace(parts[0])); ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// isPrivate returns true for RFC 1918, loopback, link-local, and ULA addresses.
func isPrivate(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7",
		"100.64.0.0/10", // CGNAT
	}
	for _, cidr := range private {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// ensureFile downloads src to dst if dst does not exist or is empty.
func ensureFile(dst, src string) error {
	if info, err := os.Stat(dst); err == nil && info.Size() > 0 {
		return nil // already present
	}

	log.Printf("geoip: downloading %s → %s", src, dst)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "pastebin-geoip/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, src)
	}

	tmp := dst + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("create tmp file: %w", err)
	}

	// Cap download at 512 MiB to guard against runaway responses.
	lr := io.LimitReader(resp.Body, 512<<20)
	if _, err := io.Copy(f, lr); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close: %w", err)
	}

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}

	log.Printf("geoip: downloaded %s (%.1f MiB)", filepath.Base(dst), float64(mustStat(dst))/float64(1<<20))
	return nil
}

func mustStat(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
