// Package maintenance implements the --maintenance CLI subcommands:
// backup, restore, mode, setup, and update (alias for --update yes).
package maintenance

import (
	"archive/tar"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	orgName     = "apimgr"
	projectName = "pastebin"
)

// Manifest is embedded in every backup archive as manifest.json.
type Manifest struct {
	Version          string   `json:"version"`
	CreatedAt        string   `json:"created_at"`
	AppVersion       string   `json:"app_version"`
	Contents         []string `json:"contents"`
	Encrypted        bool     `json:"encrypted"`
	EncryptionMethod string   `json:"encryption_method,omitempty"`
	Checksum         string   `json:"checksum"`
}

// BackupOptions controls what gets included in the backup.
type BackupOptions struct {
	ConfigDir  string
	DataDir    string
	BackupDir  string
	AppVersion string
	Password   string // empty = no encryption
	Filename   string // empty = auto-generate
}

// Backup creates a tar.gz (optionally AES-256-GCM encrypted) backup of the
// config and database files and writes it to BackupDir.
func Backup(opts BackupOptions) error {
	if err := os.MkdirAll(opts.BackupDir, 0o700); err != nil {
		return fmt.Errorf("creating backup dir: %w", err)
	}

	ts := time.Now().UTC().Format("2006-01-02_150405")
	ext := ".tar.gz"
	if opts.Password != "" {
		ext = ".tar.gz.enc"
	}
	filename := opts.Filename
	if filename == "" {
		filename = fmt.Sprintf("%s_backup_%s%s", projectName, ts, ext)
	}
	destPath := filepath.Join(opts.BackupDir, filename)

	// Collect files to archive.
	type entry struct {
		src  string
		name string // path inside the archive
	}
	var entries []entry
	var contents []string

	addFile := func(src, name string) {
		if _, err := os.Stat(src); err == nil {
			entries = append(entries, entry{src, name})
			contents = append(contents, name)
		}
	}
	addDir := func(dir, prefix string) {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			addFile(path, filepath.Join(prefix, rel))
			return nil
		})
	}

	addFile(filepath.Join(opts.ConfigDir, "server.yml"), "server.yml")
	addFile(filepath.Join(opts.DataDir, "db", "server.db"), "server.db")
	addFile(filepath.Join(opts.DataDir, "db", "users.db"), "users.db")
	addDir(filepath.Join(opts.ConfigDir, "template"), "template")
	addDir(filepath.Join(opts.ConfigDir, "theme"), "theme")

	// Build the archive into memory so we can compute a checksum and
	// optionally encrypt without writing a plaintext file to disk.
	var buf strings.Builder
	_ = buf // use bytes.Buffer instead
	archiveBuf := &memBuf{}

	gz := gzip.NewWriter(archiveBuf)
	tw := tar.NewWriter(gz)

	for _, e := range entries {
		if err := addToTar(tw, e.src, e.name); err != nil {
			return fmt.Errorf("archiving %s: %w", e.name, err)
		}
	}

	// Build and add manifest.
	manifest := Manifest{
		Version:    "1.0.0",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		AppVersion: opts.AppVersion,
		Contents:   contents,
		Encrypted:  opts.Password != "",
	}
	if opts.Password != "" {
		manifest.EncryptionMethod = "AES-256-GCM"
	}

	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	hdr := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestJSON)),
		Mode:    0o600,
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write manifest header: %w", err)
	}
	if _, err := tw.Write(manifestJSON); err != nil {
		return fmt.Errorf("write manifest body: %w", err)
	}

	tw.Close()
	gz.Close()

	archiveBytes := archiveBuf.Bytes()

	// Compute SHA-256 of the unencrypted archive.
	sum := sha256.Sum256(archiveBytes)
	manifest.Checksum = "sha256:" + hex.EncodeToString(sum[:])

	// Re-encode the manifest with the checksum and rewrite. For simplicity we
	// re-archive with the updated manifest.
	archiveBuf2 := &memBuf{}
	gz2 := gzip.NewWriter(archiveBuf2)
	tw2 := tar.NewWriter(gz2)
	for _, e := range entries {
		if err := addToTar(tw2, e.src, e.name); err != nil {
			return fmt.Errorf("re-archiving %s: %w", e.name, err)
		}
	}
	manifestJSON2, _ := json.MarshalIndent(manifest, "", "  ")
	hdr2 := &tar.Header{Name: "manifest.json", Size: int64(len(manifestJSON2)), Mode: 0o600, ModTime: time.Now().UTC()}
	tw2.WriteHeader(hdr2)
	tw2.Write(manifestJSON2)
	tw2.Close()
	gz2.Close()
	archiveBytes = archiveBuf2.Bytes()

	var finalBytes []byte
	if opts.Password != "" {
		finalBytes, err = encrypt(archiveBytes, opts.Password)
		if err != nil {
			return fmt.Errorf("encrypting backup: %w", err)
		}
	} else {
		finalBytes = archiveBytes
	}

	if err := os.WriteFile(destPath, finalBytes, 0o600); err != nil {
		return fmt.Errorf("writing backup file: %w", err)
	}

	fmt.Printf("Backup written: %s\n", destPath)
	return nil
}

// Restore extracts a backup archive into the config and data directories.
// Per PART 21, the backup is fully verified before any data is written;
// restore only proceeds when every verification check passes.
func Restore(archivePath, configDir, dataDir, password string) error {
	// Verify the backup before touching any live config or data (PART 21:
	// "Only proceed with restore if ALL verification checks pass").
	if err := VerifyBackup(archivePath, password); err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}

	data, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("reading backup: %w", err)
	}

	// Detect encryption by file extension.
	if strings.HasSuffix(archivePath, ".enc") {
		if password == "" {
			return fmt.Errorf("backup is encrypted — provide a password with --password")
		}
		data, err = decrypt(data, password)
		if err != nil {
			return fmt.Errorf("decrypting backup: %w", err)
		}
	}

	gr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("opening gzip: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}
		if hdr.Name == "manifest.json" {
			continue // skip — informational only
		}
		if err := extractEntry(tr, hdr, configDir, dataDir); err != nil {
			return fmt.Errorf("extracting %s: %w", hdr.Name, err)
		}
	}

	fmt.Printf("Restore complete from: %s\n", archivePath)
	return nil
}

// SetMode updates the server.mode value in server.yml.
func SetMode(configDir, mode string) error {
	cfgPath := filepath.Join(configDir, "server.yml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}
	updated := strings.Join(replaceYAMLField(strings.Split(string(data), "\n"), "mode", mode), "\n")
	if err := os.WriteFile(cfgPath, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	fmt.Printf("Mode set to: %s\n", mode)
	return nil
}

// replaceYAMLField does a best-effort in-place replacement of a top-level
// YAML key's value.  Only handles simple scalar values.
func replaceYAMLField(lines []string, key, value string) []string {
	prefix := key + ":"
	out := make([]string, len(lines))
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, prefix) {
			indent := strings.Index(l, strings.TrimLeft(l, " \t"))
			if indent < 0 {
				indent = 0
			}
			out[i] = strings.Repeat(" ", indent) + key + ": " + value
		} else {
			out[i] = l
		}
	}
	return out
}

// Setup prints guidance for resetting admin credentials.
func Setup(configDir string) error {
	fmt.Println("To reset admin credentials, edit the database directly or restart")
	fmt.Println("the server with the PASTEBIN_RESET_ADMIN=1 environment variable.")
	fmt.Printf("Config directory: %s\n", configDir)
	return nil
}

// PrintHelp prints the --maintenance subcommand help.
func PrintHelp(binaryName string) {
	fmt.Printf(`Maintenance operations: %s --maintenance <command>

Commands:
  backup [filename]   Create a backup of config and database files
  restore <file>      Restore from a backup file
  update              Update the binary to the latest release (alias: --update yes)
  mode <mode>         Set the server mode (production|development)
  setup               Reset admin credentials / initial setup
  --help              Show this help

Examples:
  %s --maintenance backup
  %s --maintenance backup mybackup.tar.gz
  %s --maintenance restore pastebin_backup_2025-01-01_120000.tar.gz
  %s --maintenance mode development
  %s --maintenance update
`, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName)
}

// VerifyBackup performs the post-creation checks required by PART 21:
//  1. File exists
//  2. Size > 0
//  3. Decrypt (if encrypted, password must work)
//  4. Manifest present and parseable
//  5. All archive entries extractable to temp dir
//  6. Any *.db entries have a valid SQLite magic header
//
// Returns nil when all checks pass. The caller must delete the file and abort
// retention on any error.
func VerifyBackup(path, password string) error {
	// Check 1 + 2: file exists and non-empty.
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("verify: file not found: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("verify: file is empty: %s", path)
	}

	// Read the raw bytes.
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("verify: cannot read: %w", err)
	}

	// Check 3: decrypt if encrypted (.enc or .enc.tmp for in-flight temp files).
	checkPath := strings.TrimSuffix(path, ".tmp")
	if strings.HasSuffix(checkPath, ".enc") {
		if password == "" {
			return fmt.Errorf("verify: encrypted backup requires password")
		}
		data, err = decrypt(data, password)
		if err != nil {
			return fmt.Errorf("verify: decrypt failed: %w", err)
		}
	}

	// Open the gzip+tar archive.
	gr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("verify: not a valid gzip archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	// Check 4 + 5 + 6: walk entries.
	var manifestFound bool
	tmpDir, err := os.MkdirTemp("", "pastebin-verify-*")
	if err != nil {
		return fmt.Errorf("verify: temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("verify: reading archive entry: %w", err)
		}

		if hdr.Name == "manifest.json" {
			manifestFound = true
			// Check 4: parse the manifest.
			var m Manifest
			if jsonErr := json.NewDecoder(tr).Decode(&m); jsonErr != nil {
				return fmt.Errorf("verify: invalid manifest: %w", jsonErr)
			}
			continue
		}

		// Check 5: extract entry to temp dir.
		dest := filepath.Join(tmpDir, filepath.Base(hdr.Name))
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("verify: extract %s: %w", hdr.Name, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return fmt.Errorf("verify: read entry %s: %w", hdr.Name, err)
		}
		f.Close()

		// Check 6: SQLite magic header for .db files.
		if strings.HasSuffix(hdr.Name, ".db") {
			if err := verifySQLiteMagic(dest); err != nil {
				return fmt.Errorf("verify: db integrity %s: %w", hdr.Name, err)
			}
		}
	}

	if !manifestFound {
		return fmt.Errorf("verify: manifest.json missing from archive")
	}

	return nil
}

// verifySQLiteMagic checks that a file begins with the SQLite3 magic header.
func verifySQLiteMagic(path string) error {
	const sqliteMagic = "SQLite format 3\x00"
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, len(sqliteMagic))
	if _, err := io.ReadFull(f, buf); err != nil {
		return fmt.Errorf("cannot read header: %w", err)
	}
	if string(buf) != sqliteMagic {
		return fmt.Errorf("not a valid SQLite3 file")
	}
	return nil
}

// ── internal helpers ─────────────────────────────────────────────────────────

type memBuf struct {
	b []byte
}

func (m *memBuf) Write(p []byte) (int, error) {
	m.b = append(m.b, p...)
	return len(p), nil
}

func (m *memBuf) Bytes() []byte { return m.b }

func addToTar(tw *tar.Writer, srcPath, archiveName string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:    archiveName,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = io.Copy(tw, f)
	return err
}

func extractEntry(tr *tar.Reader, hdr *tar.Header, configDir, dataDir string) error {
	// Map archive names to destination paths.
	var dest string
	switch hdr.Name {
	case "server.yml":
		dest = filepath.Join(configDir, "server.yml")
	case "server.db":
		dest = filepath.Join(dataDir, "db", "server.db")
	case "users.db":
		dest = filepath.Join(dataDir, "db", "users.db")
	default:
		if strings.HasPrefix(hdr.Name, "template/") {
			rel := strings.TrimPrefix(hdr.Name, "template/")
			dest = filepath.Join(configDir, "template", rel)
		} else if strings.HasPrefix(hdr.Name, "theme/") {
			rel := strings.TrimPrefix(hdr.Name, "theme/")
			dest = filepath.Join(configDir, "theme", rel)
		} else {
			return nil // unknown entry — skip
		}
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
	if err != nil {
		return err
	}
	defer f.Close()

	lr := io.LimitReader(tr, 256<<20) // 256 MiB cap per entry
	_, err = io.Copy(f, lr)
	return err
}

// encrypt encrypts data using AES-256-GCM with a key derived via Argon2id.
// Output format: [16-byte salt][12-byte nonce][ciphertext+tag].
func encrypt(plaintext []byte, password string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := argon2Key(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, 16+len(nonce)+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

// decrypt reverses encrypt.
func decrypt(data []byte, password string) ([]byte, error) {
	if len(data) < 16+12 {
		return nil, fmt.Errorf("backup too short to be valid")
	}
	salt := data[:16]
	key := argon2Key(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < 16+nonceSize {
		return nil, fmt.Errorf("backup truncated")
	}
	nonce := data[16 : 16+nonceSize]
	ciphertext := data[16+nonceSize:]

	return gcm.Open(nil, nonce, ciphertext, nil)
}

// argon2Key derives a 32-byte AES key from a password and salt using Argon2id.
func argon2Key(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
}
