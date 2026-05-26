// Package task provides the bodies for the built-in PART 18 scheduler tasks.
// Each exported function returns a TaskFunc (func() error) ready to pass to
// scheduler.Register. The closures capture only the paths they need, so callers
// in main.go can construct them once and register them directly.
package task

import (
	"archive/tar"
	"compress/gzip"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SSLRenewal returns a task that checks certificates in
// {configDir}/ssl/letsencrypt/{fqdn}/ and logs a warning for any cert that
// expires within 7 days.  Actual ACME renewal is delegated to autocert /
// certbot; this task provides visibility and alerting.
func SSLRenewal(configDir, fqdn string) func() error {
	return func() error {
		certRoot := filepath.Join(configDir, "ssl", "letsencrypt", fqdn)
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
				if remaining < 7*24*time.Hour {
					log.Printf("ssl_renewal: WARNING — %s expires in %s (at %s)",
						path, remaining.Round(time.Hour), cert.NotAfter.Format(time.RFC3339))
				} else {
					log.Printf("ssl_renewal: %s valid for %s (expires %s)",
						path, remaining.Round(time.Hour), cert.NotAfter.Format("2006-01-02"))
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
func BlocklistUpdate(dataDir string) func() error {
	return func() error {
		dir := filepath.Join(dataDir, "security", "blocklists")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("blocklist_update: mkdir %s: %w", dir, err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("blocklist_update: readdir: %w", err)
		}

		log.Printf("blocklist_update: %s — %d file(s) present", dir, len(entries))
		return nil
	}
}

// CVEUpdate returns a task that ensures the CVE database directory exists.
// Actual CVE data files are expected to be placed here by the operator.
func CVEUpdate(dataDir string) func() error {
	return func() error {
		dir := filepath.Join(dataDir, "security", "cve")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("cve_update: mkdir %s: %w", dir, err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("cve_update: readdir: %w", err)
		}

		log.Printf("cve_update: %s — %d file(s) present", dir, len(entries))
		return nil
	}
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

// BackupDaily returns a task that creates a full tar.gz backup of dataDir in
// backupDir, then trims old backups to keep at most maxBackups copies.
// The archive is named {project}_{date}.tar.gz.  A maxBackups of 0 defaults
// to 1.
func BackupDaily(projectName, dataDir, backupDir string, maxBackups int) func() error {
	if maxBackups < 1 {
		maxBackups = 1
	}
	return func() error {
		if err := os.MkdirAll(backupDir, 0o750); err != nil {
			return fmt.Errorf("backup_daily: mkdir: %w", err)
		}

		date := time.Now().Format("2006-01-02")
		dst := filepath.Join(backupDir, fmt.Sprintf("%s_backup_%s.tar.gz", projectName, date))

		if err := createTarGz(dst, dataDir); err != nil {
			return fmt.Errorf("backup_daily: create archive: %w", err)
		}

		info, _ := os.Stat(dst)
		sizeKB := int64(0)
		if info != nil {
			sizeKB = info.Size() / 1024
		}
		log.Printf("backup_daily: created %s (%d KB)", filepath.Base(dst), sizeKB)

		if err := trimBackups(backupDir, projectName+"_backup_", maxBackups); err != nil {
			log.Printf("backup_daily: trim warning: %v", err)
		}
		return nil
	}
}

// BackupHourly returns a task that replaces a single rolling backup archive
// {project}-hourly.tar.gz in backupDir with a fresh snapshot of dataDir.
func BackupHourly(projectName, dataDir, backupDir string) func() error {
	return func() error {
		if err := os.MkdirAll(backupDir, 0o750); err != nil {
			return fmt.Errorf("backup_hourly: mkdir: %w", err)
		}

		dst := filepath.Join(backupDir, projectName+"-hourly.tar.gz")
		tmp := dst + ".tmp"

		if err := createTarGz(tmp, dataDir); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("backup_hourly: create archive: %w", err)
		}

		if err := os.Rename(tmp, dst); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("backup_hourly: rename: %w", err)
		}

		log.Printf("backup_hourly: updated %s", filepath.Base(dst))
		return nil
	}
}

// createTarGz writes a tar.gz archive of src (a directory) to dst (a file).
func createTarGz(dst, src string) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}

	gz, err := gzip.NewWriterLevel(out, gzip.BestSpeed)
	if err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	tw := tar.NewWriter(gz)

	base := filepath.Base(src)
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		name := filepath.Join(base, rel)

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = name
		if info.IsDir() {
			hdr.Name += "/"
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})

	if err != nil {
		tw.Close()
		gz.Close()
		out.Close()
		os.Remove(dst)
		return err
	}

	if err := tw.Close(); err != nil {
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
	return out.Close()
}

// trimBackups removes the oldest backup files with the given prefix so that
// at most keep files remain.
func trimBackups(dir, prefix string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var files []os.FileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, info)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	for len(files) > keep {
		oldest := filepath.Join(dir, files[0].Name())
		if err := os.Remove(oldest); err != nil {
			return err
		}
		log.Printf("backup: removed old archive %s", files[0].Name())
		files = files[1:]
	}
	return nil
}

// TorHealth returns a task that checks whether Tor is running and logs the
// result.  torRunning is a function that reports the current Tor state; it is
// injected from server.TorRunning() so the task package has no import cycle.
func TorHealth(torRunning func() bool) func() error {
	return func() error {
		if torRunning == nil {
			return nil
		}
		if torRunning() {
			log.Printf("tor_health: Tor hidden service is running")
		} else {
			log.Printf("tor_health: Tor hidden service is not running (no Tor binary found or startup failed)")
		}
		return nil
	}
}
