// Package task provides the bodies for the built-in PART 18 scheduler tasks.
// Each exported function returns a TaskFunc (func() error) ready to pass to
// scheduler.Register. The closures capture only the paths they need, so callers
// in main.go can construct them once and register them directly.
package task

import (
	"compress/gzip"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/pastebin/src/maintenance"
)

// BackupRetention controls how many old backups to keep for each period.
type BackupRetention struct {
	// MaxBackups is the number of dated full backups to keep (≥1, default 1).
	MaxBackups int
	// KeepWeekly keeps Sunday backups; 0 = disabled.
	KeepWeekly int
	// KeepMonthly keeps 1st-of-month backups; 0 = disabled.
	KeepMonthly int
	// KeepYearly keeps Jan 1st backups; 0 = disabled.
	KeepYearly int
}

// Mailer is the subset of email.Mailer used by task functions. Callers inject a
// concrete *email.Mailer so the task package stays free of direct email imports.
type Mailer interface {
	Enabled() bool
	Send(to, tmplName string, vars map[string]string) error
}

// BackupConfig is the configuration passed to BackupDaily and BackupHourly.
type BackupConfig struct {
	ProjectName string
	ConfigDir   string
	DataDir     string
	BackupDir   string
	AppVersion  string
	// Password is the AES-256-GCM encryption password; empty = no encryption.
	Password string
	// OperatorEmail is the admin recipient for backup_complete/backup_failed emails.
	// Empty or nil Mailer disables email for that task invocation.
	OperatorEmail string
	// Mailer sends backup outcome emails; nil = no email.
	Mailer    Mailer
	// SendOnComplete controls whether backup_complete emails are sent (AI.md:26591).
	SendOnComplete bool
	// SendOnFailed controls whether backup_failed emails are sent (AI.md:26592).
	SendOnFailed bool
	Retention BackupRetention
}

// sslWarnThresholds are the days-before-expiry at which ssl_expiring emails are sent
// per AI.md:26206 ("Sent 30, 14, 7, 3, 1 days before expiry").
var sslWarnThresholds = []int{30, 14, 7, 3, 1}

// SSLRenewalConfig carries optional email settings for the ssl_renewal task.
type SSLRenewalConfig struct {
	ConfigDir     string
	FQDN          string
	// OperatorEmail is the admin recipient for ssl_expiring/ssl_renewed emails.
	OperatorEmail string
	// Mailer sends SSL-expiry emails; nil = no email.
	Mailer        Mailer
	// SendExpiring controls whether ssl_expiring emails are sent (AI.md:26593).
	SendExpiring  bool
	// SendRenewed controls whether ssl_renewed emails are sent (AI.md:26594).
	SendRenewed   bool
}

// SSLRenewal returns a task that checks certificates in
// {configDir}/ssl/letsencrypt/{fqdn}/ and logs a warning for any cert that
// expires within 30 days. Emails are sent at the 30/14/7/3/1-day thresholds
// per AI.md:26206 when a Mailer and OperatorEmail are provided.
// Actual ACME renewal is delegated to autocert; this task provides visibility
// and alerting only.
func SSLRenewal(configDir, fqdn string) func() error {
	return SSLRenewalWithEmail(SSLRenewalConfig{ConfigDir: configDir, FQDN: fqdn})
}

// SSLRenewalWithEmail is the full SSLRenewal implementation that also sends
// ssl_expiring emails at the PART 17 thresholds (30/14/7/3/1 days).
func SSLRenewalWithEmail(cfg SSLRenewalConfig) func() error {
	return func() error {
		certRoot := filepath.Join(cfg.ConfigDir, "ssl", "letsencrypt", cfg.FQDN)
		if _, err := os.Stat(certRoot); os.IsNotExist(err) {
			return nil
		}

		return filepath.WalkDir(certRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".pem" && ext != ".crt" {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("ssl_renewal: cannot read %s: %v", path, err)
				return nil
			}

			// Try to decode as DER directly; fall back to PEM.
			var certs []*x509.Certificate
			if cert, parseErr := x509.ParseCertificate(data); parseErr == nil {
				certs = append(certs, cert)
			} else {
				// crypto/tls can decode PEM chains.
				tlsCert, tlsErr := tls.X509KeyPair(data, data)
				if tlsErr == nil {
					for _, der := range tlsCert.Certificate {
						if c, e := x509.ParseCertificate(der); e == nil {
							certs = append(certs, c)
						}
					}
				}
			}

			for _, cert := range certs {
				remaining := time.Until(cert.NotAfter)
				remainingDays := int(remaining.Hours() / 24)

				if remaining < 30*24*time.Hour {
					log.Printf("ssl_renewal: WARNING — %s expires in %d day(s) (at %s)",
						path, remainingDays, cert.NotAfter.Format(time.RFC3339))
				} else {
					log.Printf("ssl_renewal: %s valid for %d day(s) (expires %s)",
						path, remainingDays, cert.NotAfter.Format("2006-01-02"))
				}

				stateFile := path + ".ssl_state.json"
				prevExpiry := sslLoadExpiry(stateFile)

				// Detect renewal: NotAfter advanced by ≥24h vs the stored state.
				if cfg.SendRenewed && cfg.Mailer != nil && cfg.Mailer.Enabled() && cfg.OperatorEmail != "" {
					if !prevExpiry.IsZero() && cert.NotAfter.Sub(prevExpiry) >= 24*time.Hour {
						if mailErr := cfg.Mailer.Send(cfg.OperatorEmail, "ssl_renewed", map[string]string{
							"fqdn":        cfg.FQDN,
							"valid_until": cert.NotAfter.Format("2006-01-02"),
						}); mailErr != nil {
							log.Printf("ssl_renewal: failed to send ssl_renewed email: %v", mailErr)
						}
					}
				}
				// Persist current expiry for next run's renewal detection.
				sslSaveExpiry(stateFile, cert.NotAfter)

				// Send ssl_expiring email at each of the spec-mandated thresholds.
				if cfg.SendExpiring && cfg.Mailer != nil && cfg.Mailer.Enabled() && cfg.OperatorEmail != "" {
					for _, threshold := range sslWarnThresholds {
						if remainingDays <= threshold {
							if mailErr := cfg.Mailer.Send(cfg.OperatorEmail, "ssl_expiring", map[string]string{
								"fqdn":        cfg.FQDN,
								"expires_in":  fmt.Sprintf("%d", remainingDays),
								"expiry_date": cert.NotAfter.Format("2006-01-02"),
							}); mailErr != nil {
								log.Printf("ssl_renewal: failed to send ssl_expiring email: %v", mailErr)
							}
							break
						}
					}
				}
			}
			return nil
		})
	}
}

// BlocklistUpdate returns a task that ensures the blocklist directory exists.
// Any blocklist sources configured externally (e.g., ip-location-db, ipsum)
// should be placed in this directory by the operator or via a separate
// sidecar process.  This task creates the directory and logs its content count.
// Source identifies a remote security-data file to download. Name is the
// destination filename within the target directory; URL is the download source.
type Source struct {
	Name string
	URL  string
}

// BlocklistUpdate returns a task that refreshes the IP/domain blocklists in
// {dataDir}/security/blocklists/. Each configured source is downloaded
// atomically; a failed download is logged and the existing copy (if any) is
// kept — GeoIP/blocklist data is a risk signal, never a hard gate, so the task
// degrades gracefully (PART 18/19, AI.md:9135). With no sources configured the
// task only ensures the directory exists.
func BlocklistUpdate(dataDir string, sources ...Source) func() error {
	return securityFetchTask("blocklist_update",
		filepath.Join(dataDir, "security", "blocklists"), sources)
}

// CVEUpdate returns a task that refreshes the CVE/security databases in
// {dataDir}/security/cve/. Behaves identically to BlocklistUpdate: each
// configured source is downloaded atomically with graceful degradation, and
// with no sources configured the task only ensures the directory exists.
func CVEUpdate(dataDir string, sources ...Source) func() error {
	return securityFetchTask("cve_update",
		filepath.Join(dataDir, "security", "cve"), sources)
}

// securityFetchTask builds the common download-with-graceful-degradation body
// shared by BlocklistUpdate and CVEUpdate.
func securityFetchTask(name, dir string, sources []Source) func() error {
	return func() error {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("%s: mkdir %s: %w", name, dir, err)
		}

		updated := 0
		for _, src := range sources {
			if src.Name == "" || src.URL == "" {
				continue
			}
			dst := filepath.Join(dir, filepath.Base(src.Name))
			if err := downloadFile(src.URL, dst); err != nil {
				log.Printf("%s: %s: %v (keeping existing copy)", name, src.Name, err)
				continue
			}
			updated++
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("%s: readdir: %w", name, err)
		}

		if updated > 0 {
			if err := os.WriteFile(filepath.Join(dir, ".last_updated"),
				[]byte(time.Now().UTC().Format(time.RFC3339)+"\n"), 0o640); err != nil {
				log.Printf("%s: write .last_updated: %v", name, err)
			}
		}

		log.Printf("%s: %s — %d source(s) updated, %d file(s) present", name, dir, updated, len(entries))
		return nil
	}
}

// downloadFile fetches url and writes the body to dst atomically (temp file in
// the same directory, then rename). An empty body or non-2xx status is treated
// as an error and dst is left untouched.
func downloadFile(url, dst string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("get %s: status %d", url, resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".dl-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	n, err := io.Copy(tmp, resp.Body)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("write body: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("empty response body")
	}
	if err := os.Chmod(tmpName, 0o640); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// LogRotation returns a task that rotates and compresses log files in logsDir.
//   - Files matching *.log older than maxAge are gzip-compressed in place.
//   - Already-compressed files (*.log.gz) older than 3×maxAge are deleted.
//
// A maxAge of 0 defaults to 30 days.
func LogRotation(logsDir string, maxAge time.Duration) func() error {
	if maxAge <= 0 {
		maxAge = 30 * 24 * time.Hour
	}
	return func() error {
		now := time.Now()
		var errs []string

		walkErr := filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}

			info, err := d.Info()
			if err != nil {
				return nil
			}

			age := now.Sub(info.ModTime())

			if strings.HasSuffix(path, ".log.gz") {
				if age > 3*maxAge {
					if rmErr := os.Remove(path); rmErr != nil {
						errs = append(errs, fmt.Sprintf("remove %s: %v", path, rmErr))
					} else {
						log.Printf("log_rotation: deleted old archive %s", filepath.Base(path))
					}
				}
				return nil
			}

			if strings.HasSuffix(path, ".log") && age > maxAge {
				if gzErr := gzipFile(path); gzErr != nil {
					errs = append(errs, fmt.Sprintf("compress %s: %v", path, gzErr))
				} else {
					log.Printf("log_rotation: compressed %s", filepath.Base(path))
				}
			}

			return nil
		})

		if walkErr != nil {
			return fmt.Errorf("log_rotation: walk: %w", walkErr)
		}
		if len(errs) > 0 {
			return fmt.Errorf("log_rotation: %s", strings.Join(errs, "; "))
		}
		return nil
	}
}

// gzipFile compresses src to src+".gz" then removes src.
func gzipFile(src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	dst := src + ".gz"
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}

	gz, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}

	if _, err := io.Copy(gz, in); err != nil {
		gz.Close()
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := gz.Close(); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}

	return os.Remove(src)
}

// BackupDaily returns a task that runs the PART 21 backup protocol:
//  1. Create a dated full backup using maintenance.Backup (with encryption if configured).
//  2. Verify the backup immediately (6 checks per spec).
//  3. Create/replace the rolling daily incremental ({project}-daily.tar.gz[.enc]).
//  4. Verify the daily incremental.
//  5. Apply retention policy (daily + optional weekly/monthly/yearly).
//
// If any verification step fails the failed file is deleted, existing backups
// are preserved, and an error is returned for the scheduler to log.
func BackupDaily(cfg BackupConfig) func() error {
	if cfg.Retention.MaxBackups < 1 {
		cfg.Retention.MaxBackups = 1
	}
	return func() error {
		if err := os.MkdirAll(cfg.BackupDir, 0o750); err != nil {
			return fmt.Errorf("backup_daily: mkdir: %w", err)
		}

		ext := ".tar.gz"
		if cfg.Password != "" {
			ext = ".tar.gz.enc"
		}

		// Step 1: full dated backup.
		date := time.Now().Format("2006-01-02")
		fullName := fmt.Sprintf("%s_backup_%s%s", cfg.ProjectName, date, ext)
		fullPath := filepath.Join(cfg.BackupDir, fullName)

		if err := maintenance.Backup(maintenance.BackupOptions{
			ConfigDir:  cfg.ConfigDir,
			DataDir:    cfg.DataDir,
			BackupDir:  cfg.BackupDir,
			AppVersion: cfg.AppVersion,
			Password:   cfg.Password,
			Filename:   fullName,
		}); err != nil {
			os.Remove(fullPath)
			backupSendFailed(cfg, fullName, err.Error())
			return fmt.Errorf("backup_daily: create full backup: %w", err)
		}

		// Step 2: verify the full backup immediately.
		if err := maintenance.VerifyBackup(fullPath, cfg.Password); err != nil {
			os.Remove(fullPath)
			backupSendFailed(cfg, fullName, err.Error())
			return fmt.Errorf("backup_daily: verification failed for %s: %w", fullName, err)
		}

		info, _ := os.Stat(fullPath)
		sizeKB := int64(0)
		if info != nil {
			sizeKB = info.Size() / 1024
		}
		log.Printf("backup_daily: full backup verified: %s (%d KB)", fullName, sizeKB)
		backupSendComplete(cfg, fullName, fmt.Sprintf("%d KB", sizeKB))

		// Step 3: create/replace the daily incremental.
		dailyName := fmt.Sprintf("%s-daily%s", cfg.ProjectName, ext)
		dailyPath := filepath.Join(cfg.BackupDir, dailyName)
		dailyTmp := dailyPath + ".tmp"

		if err := maintenance.Backup(maintenance.BackupOptions{
			ConfigDir:  cfg.ConfigDir,
			DataDir:    cfg.DataDir,
			BackupDir:  cfg.BackupDir,
			AppVersion: cfg.AppVersion,
			Password:   cfg.Password,
			Filename:   dailyName + ".tmp",
		}); err != nil {
			os.Remove(dailyTmp)
			log.Printf("backup_daily: daily incremental create warning: %v", err)
		} else {
			// Step 4: verify the daily incremental before replacing the existing one.
			if err := maintenance.VerifyBackup(dailyTmp, cfg.Password); err != nil {
				os.Remove(dailyTmp)
				log.Printf("backup_daily: daily incremental verification warning: %v", err)
			} else {
				if err := os.Rename(dailyTmp, dailyPath); err != nil {
					os.Remove(dailyTmp)
					log.Printf("backup_daily: daily incremental rename warning: %v", err)
				} else {
					log.Printf("backup_daily: daily incremental updated: %s", dailyName)
				}
			}
		}

		// Step 5: apply retention policy.
		if err := applyRetention(cfg.BackupDir, cfg.ProjectName, cfg.Retention); err != nil {
			log.Printf("backup_daily: retention warning: %v", err)
		}

		return nil
	}
}

// BackupHourly returns a task that replaces the rolling hourly incremental
// ({project}-hourly.tar.gz[.enc]) with a fresh snapshot, then verifies it.
func BackupHourly(cfg BackupConfig) func() error {
	return func() error {
		if err := os.MkdirAll(cfg.BackupDir, 0o750); err != nil {
			return fmt.Errorf("backup_hourly: mkdir: %w", err)
		}

		ext := ".tar.gz"
		if cfg.Password != "" {
			ext = ".tar.gz.enc"
		}

		hourlyName := fmt.Sprintf("%s-hourly%s", cfg.ProjectName, ext)
		hourlyPath := filepath.Join(cfg.BackupDir, hourlyName)
		tmpName := hourlyName + ".tmp"
		tmpPath := filepath.Join(cfg.BackupDir, tmpName)

		if err := maintenance.Backup(maintenance.BackupOptions{
			ConfigDir:  cfg.ConfigDir,
			DataDir:    cfg.DataDir,
			BackupDir:  cfg.BackupDir,
			AppVersion: cfg.AppVersion,
			Password:   cfg.Password,
			Filename:   tmpName,
		}); err != nil {
			os.Remove(tmpPath)
			backupSendFailed(cfg, hourlyName, err.Error())
			return fmt.Errorf("backup_hourly: create archive: %w", err)
		}

		if err := maintenance.VerifyBackup(tmpPath, cfg.Password); err != nil {
			os.Remove(tmpPath)
			backupSendFailed(cfg, hourlyName, err.Error())
			return fmt.Errorf("backup_hourly: verification failed: %w", err)
		}

		if err := os.Rename(tmpPath, hourlyPath); err != nil {
			os.Remove(tmpPath)
			backupSendFailed(cfg, hourlyName, err.Error())
			return fmt.Errorf("backup_hourly: rename: %w", err)
		}

		log.Printf("backup_hourly: updated %s", hourlyName)
		return nil
	}
}

// sslStateFile is the JSON structure persisted alongside each cert to track the
// last-seen NotAfter for renewal detection.
type sslStateFile struct {
	NotAfter time.Time `json:"not_after"`
}

// sslLoadExpiry reads the stored NotAfter for a cert state file. Returns zero
// time if the file is missing or unreadable (safe first-run behaviour).
func sslLoadExpiry(path string) time.Time {
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	var s sslStateFile
	if err := json.Unmarshal(data, &s); err != nil {
		return time.Time{}
	}
	return s.NotAfter
}

// sslSaveExpiry persists the cert's NotAfter to path for next-run renewal
// detection. Errors are logged and ignored (non-fatal).
func sslSaveExpiry(path string, notAfter time.Time) {
	data, err := json.Marshal(sslStateFile{NotAfter: notAfter})
	if err != nil {
		log.Printf("ssl_renewal: failed to marshal state: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Printf("ssl_renewal: failed to write state %s: %v", path, err)
	}
}

// backupSendComplete sends a backup_complete email when the backup succeeded and
// the BackupConfig has a Mailer, OperatorEmail, and SendOnComplete set (AI.md:26204).
func backupSendComplete(cfg BackupConfig, filename, size string) {
	if !cfg.SendOnComplete || cfg.Mailer == nil || !cfg.Mailer.Enabled() || cfg.OperatorEmail == "" {
		return
	}
	if err := cfg.Mailer.Send(cfg.OperatorEmail, "backup_complete", map[string]string{
		"filename": filename,
		"size":     size,
	}); err != nil {
		log.Printf("backup: failed to send backup_complete email: %v", err)
	}
}

// backupSendFailed sends a backup_failed email when the backup errored and the
// BackupConfig has a Mailer, OperatorEmail, and SendOnFailed set (AI.md:26205).
func backupSendFailed(cfg BackupConfig, filename, errMsg string) {
	if !cfg.SendOnFailed || cfg.Mailer == nil || !cfg.Mailer.Enabled() || cfg.OperatorEmail == "" {
		return
	}
	if err := cfg.Mailer.Send(cfg.OperatorEmail, "backup_failed", map[string]string{
		"filename": filename,
		"size":     "",
		"error":    errMsg,
	}); err != nil {
		log.Printf("backup: failed to send backup_failed email: %v", err)
	}
}

// backupFileRE matches dated backup filenames like project_backup_2025-01-15.tar.gz[.enc].
var backupFileRE = regexp.MustCompile(`_backup_(\d{4}-\d{2}-\d{2})\.tar\.gz(\.enc)?$`)

// applyRetention removes old backups according to the retention policy (PART 21).
// Priority order: yearly > monthly > weekly > daily.
// The daily incremental (*-daily.*) and hourly (*-hourly.*) are never deleted here.
func applyRetention(dir, project string, ret BackupRetention) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Collect dated full backup files.
	type backupEntry struct {
		name string
		date time.Time
	}
	var backups []backupEntry
	prefix := project + "_backup_"
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		m := backupFileRE.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		t, err := time.Parse("2006-01-02", m[1])
		if err != nil {
			continue
		}
		backups = append(backups, backupEntry{name: e.Name(), date: t})
	}

	// Sort newest first.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].date.After(backups[j].date)
	})

	// Classify each backup by its highest-priority retention tier.
	type tier int
	const (
		tierYearly  tier = iota // highest priority
		tierMonthly tier = iota
		tierWeekly  tier = iota
		tierDaily   tier = iota
	)

	classify := func(t time.Time) tier {
		if t.Month() == time.January && t.Day() == 1 && ret.KeepYearly > 0 {
			return tierYearly
		}
		if t.Day() == 1 && ret.KeepMonthly > 0 {
			return tierMonthly
		}
		if t.Weekday() == time.Sunday && ret.KeepWeekly > 0 {
			return tierWeekly
		}
		return tierDaily
	}

	// Count how many of each tier we have seen (newest first).
	countYearly, countMonthly, countWeekly, countDaily := 0, 0, 0, 0
	for _, b := range backups {
		t := classify(b.date)
		keep := false
		switch t {
		case tierYearly:
			countYearly++
			keep = countYearly <= ret.KeepYearly
		case tierMonthly:
			countMonthly++
			keep = countMonthly <= ret.KeepMonthly
		case tierWeekly:
			countWeekly++
			keep = countWeekly <= ret.KeepWeekly
		case tierDaily:
			countDaily++
			keep = countDaily <= ret.MaxBackups
		}
		if !keep {
			p := filepath.Join(dir, b.name)
			if err := os.Remove(p); err != nil {
				log.Printf("backup: retention: remove %s: %v", b.name, err)
			} else {
				log.Printf("backup: retention: removed %s", b.name)
			}
		}
	}
	return nil
}


// TorHealth returns a task that checks whether Tor is running and restarts it
// when it is unhealthy (PART 18, restart_on_fail). torRunning reports the
// current Tor state and torRestart restarts the hidden service; both are
// injected from the server so the task package has no import cycle. torRestart
// may be nil, in which case an unhealthy service is only logged. When Tor is
// not installed, torRestart is a no-op and torRunning stays false — the task
// simply logs and never errors.
func TorHealth(torRunning func() bool, torRestart func() error) func() error {
	return func() error {
		if torRunning == nil {
			return nil
		}
		if torRunning() {
			log.Printf("tor_health: Tor hidden service is running")
			return nil
		}
		if torRestart == nil {
			log.Printf("tor_health: Tor hidden service is not running (no restart configured)")
			return nil
		}
		log.Printf("tor_health: Tor hidden service is not running — attempting restart")
		if err := torRestart(); err != nil {
			return fmt.Errorf("tor_health: restart failed: %w", err)
		}
		if torRunning() {
			log.Printf("tor_health: Tor hidden service restarted successfully")
		} else {
			log.Printf("tor_health: Tor hidden service still not running after restart (no Tor binary found or startup failed)")
		}
		return nil
	}
}
