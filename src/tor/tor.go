// Package tor manages a dedicated Tor hidden service for the pastebin server.
// The server binary fully owns the Tor process lifecycle — it starts, monitors,
// and stops the process. The hidden service is auto-enabled whenever a Tor
// binary is found in PATH or common install locations; there is no enable flag.
//
// Uses github.com/cretz/bine (CGO_ENABLED=0 compatible) with ADD_ONION to
// create v3 hidden services without modifying any system Tor installation.
package tor

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cretz/bine/control"
	binettor "github.com/cretz/bine/tor"
)

// commonTorPaths lists well-known Tor binary locations per OS.
var commonTorPaths = map[string][]string{
	"linux":   {"/usr/bin/tor", "/usr/local/bin/tor", "/bin/tor"},
	"darwin":  {"/usr/local/bin/tor", "/opt/homebrew/bin/tor"},
	"windows": {`C:\Program Files\Tor\tor.exe`, `C:\Program Files (x86)\Tor\tor.exe`},
	"freebsd": {"/usr/local/bin/tor"},
	"openbsd": {"/usr/local/bin/tor"},
	"netbsd":  {"/usr/local/bin/tor"},
}

// FindBinary locates the Tor binary. Returns empty string if not found.
// Checks (in order): configured path, PATH lookup, common OS locations.
func FindBinary(configuredPath string) string {
	if configuredPath != "" {
		if _, err := os.Stat(configuredPath); err == nil {
			return configuredPath
		}
		return ""
	}
	// PATH lookup.
	if p, err := findInPath("tor"); err == nil {
		return p
	}
	// Common locations.
	for _, p := range commonTorPaths[runtime.GOOS] {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func findInPath(name string) (string, error) {
	// Cross-platform PATH search without exec.LookPath to keep CGO_ENABLED=0.
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return "", fmt.Errorf("PATH empty")
	}
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
		if !strings.HasSuffix(name, ".exe") {
			name += ".exe"
		}
	}
	for _, dir := range strings.Split(pathEnv, sep) {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("not found in PATH")
}

// Config holds Tor-related configuration; mirrors config.TorConfig.
type Config struct {
	Binary                    string
	UseNetwork                bool
	AllowUserPreference       bool
	MaxCircuits               int
	CircuitTimeout            int
	BootstrapTimeout          int
	SafeLogging               bool
	MaxStreamsPerCircuit       int
	CloseCircuitOnStreamLimit bool
	BandwidthRate             string
	BandwidthBurst            string
	MaxMonthlyBandwidth       string
	NumIntroPoints            int
	VirtualPort               int

	// Directory paths resolved at startup.
	ConfigDir string // {config_dir} — torrc written here
	DataDir   string // {data_dir} — Tor data + hidden service keys
}

// service holds a running Tor instance.
type service struct {
	t          *binettor.Tor
	serviceID  string
	key        *control.ED25519Key
	serverPort int
	dialer     *binettor.Dialer
}

// Manager owns the Tor process lifecycle.
type Manager struct {
	mu         sync.Mutex
	svc        *service
	cfg        Config
	serverPort int
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewManager returns a Manager. Call Start() to launch Tor.
func NewManager(ctx context.Context, serverPort int, cfg Config) *Manager {
	child, cancel := context.WithCancel(ctx)
	return &Manager{
		cfg:        cfg,
		serverPort: serverPort,
		ctx:        child,
		cancel:     cancel,
	}
}

// Start finds the Tor binary, ensures directories, writes torrc, and starts
// the dedicated Tor process.  Returns nil if Tor is not installed (non-fatal).
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked()
}

func (m *Manager) startLocked() error {
	bin := FindBinary(m.cfg.Binary)
	if bin == "" {
		log.Printf("Tor binary not found, hidden service disabled")
		return nil
	}

	// The hidden service forwards to the server's real HTTP listener; without a
	// valid target port there is nothing to forward to. Skip rather than point the
	// onion at a guessed port (PART 31: server port is always runtime-detected).
	if m.serverPort <= 0 {
		log.Printf("Tor: server HTTP port unknown, hidden service disabled")
		return nil
	}

	if err := ensureTorDirs(m.cfg.ConfigDir, m.cfg.DataDir); err != nil {
		return fmt.Errorf("tor dirs: %w", err)
	}

	torrcPath := filepath.Join(m.cfg.ConfigDir, "tor", "torrc")
	torDataDir := filepath.Join(m.cfg.DataDir, "tor")
	// Create torrc only when absent — never overwrite an existing operator-customised file.
	if _, err := os.Stat(torrcPath); os.IsNotExist(err) {
		if writeErr := os.WriteFile(torrcPath, []byte(getTorConfig(&m.cfg)), 0o600); writeErr != nil {
			return fmt.Errorf("write torrc: %w", writeErr)
		}
	}

	conf := &binettor.StartConf{
		ExePath:         bin,
		TorrcFile:       torrcPath,
		DataDir:         torDataDir,
		NoAutoSocksPort: true,
	}

	log.Printf("Starting Tor hidden service...")
	t, err := binettor.Start(m.ctx, conf)
	if err != nil {
		return fmt.Errorf("start tor: %w", err)
	}

	bootstrapTimeout := time.Duration(m.cfg.BootstrapTimeout) * time.Second
	dialCtx, cancel := context.WithTimeout(m.ctx, bootstrapTimeout)
	defer cancel()

	// Show "connecting…" message if bootstrap takes >30 s.
	slow := time.AfterFunc(30*time.Second, func() {
		log.Printf("Tor: connecting...")
	})
	if err := t.EnableNetwork(dialCtx, true); err != nil {
		slow.Stop()
		t.Close()
		return fmt.Errorf("tor bootstrap: %w", err)
	}
	slow.Stop()

	// Load or generate ed25519 key for persistent .onion address.
	keyPath := filepath.Join(m.cfg.DataDir, "tor", "site", "hs_ed25519_secret_key")
	var privKey *control.ED25519Key
	if keyData, err := os.ReadFile(keyPath); err == nil && len(keyData) > 0 {
		if k, err := control.ED25519KeyFromBlob(string(keyData)); err == nil {
			privKey = k
		}
	}

	addReq := &control.AddOnionRequest{
		Ports: []*control.KeyVal{
			control.NewKeyVal(
				fmt.Sprintf("%d", m.cfg.VirtualPort),
				fmt.Sprintf("127.0.0.1:%d", m.serverPort),
			),
		},
	}
	if privKey != nil {
		addReq.Key = privKey
	} else {
		addReq.Key = control.GenKey(control.KeyAlgoED25519V3)
	}

	resp, err := t.Control.AddOnion(addReq)
	if err != nil {
		t.Close()
		return fmt.Errorf("add_onion: %w", err)
	}

	// Persist key for stable .onion address across restarts.
	if privKey == nil && resp.Key != nil {
		if err := saveKey(keyPath, resp.Key); err != nil {
			log.Printf("Tor: warning: could not save onion key: %v", err)
		}
	}

	svc := &service{
		t:          t,
		serviceID:  resp.ServiceID,
		key:        privKey,
		serverPort: m.serverPort,
	}

	// Outbound dialer for optional Tor-routed HTTP clients.
	if m.cfg.UseNetwork || m.cfg.AllowUserPreference {
		if d, err := t.Dialer(m.ctx, nil); err != nil {
			log.Printf("Tor: warning: outbound dialer failed: %v", err)
		} else {
			svc.dialer = d
		}
	}

	m.svc = svc
	log.Printf("Tor: %s.onion:%d → 127.0.0.1:%d", resp.ServiceID, m.cfg.VirtualPort, m.serverPort)
	return nil
}

// Close shuts down the Tor process.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancel()
	if m.svc != nil {
		_ = m.svc.t.Close()
		m.svc = nil
	}
}

// Running returns true when Tor is active.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.svc != nil
}

// OnionAddress returns the full .onion address, or empty string if not running.
func (m *Manager) OnionAddress() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.svc != nil {
		return m.svc.serviceID + ".onion"
	}
	return ""
}

// GetHTTPClient returns a direct or Tor-routed HTTP client.
func (m *Manager) GetHTTPClient(useTor bool) *http.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if useTor && m.svc != nil && m.svc.dialer != nil {
		return &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				DialContext: m.svc.dialer.DialContext,
			},
		}
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// Restart closes the current Tor process and starts a fresh one, preserving
// the existing torrc and hidden-service keys.
func (m *Manager) Restart() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.svc != nil {
		_ = m.svc.t.Close()
		m.svc = nil
	}
	return m.startLocked()
}

// UpdateConfig applies a new Config to the Manager and restarts the Tor
// process so the change takes effect.
func (m *Manager) UpdateConfig(cfg Config) error {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	if err := m.updateTorrc(); err != nil {
		return err
	}
	return m.Restart()
}

// RegenerateAddress removes the existing hidden-service key so that Start()
// generates a fresh .onion address, and returns the new address on success.
func (m *Manager) RegenerateAddress() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keyPath := filepath.Join(m.cfg.DataDir, "tor", "site", "hs_ed25519_secret_key")
	if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove onion key: %w", err)
	}
	if m.svc != nil {
		_ = m.svc.t.Close()
		m.svc = nil
	}
	if err := m.startLocked(); err != nil {
		return "", err
	}
	if m.svc != nil {
		return m.svc.serviceID + ".onion", nil
	}
	return "", nil
}

// ApplyKeys persists new hidden-service key material to disk, restarts Tor so
// the new .onion address becomes active, and returns the new address on success.
func (m *Manager) ApplyKeys(keyBlob []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keyPath := filepath.Join(m.cfg.DataDir, "tor", "site", "hs_ed25519_secret_key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return "", fmt.Errorf("applykeys mkdir: %w", err)
	}
	if err := os.WriteFile(keyPath, keyBlob, 0o600); err != nil {
		return "", fmt.Errorf("applykeys write: %w", err)
	}
	if m.svc != nil {
		_ = m.svc.t.Close()
		m.svc = nil
	}
	if err := m.startLocked(); err != nil {
		return "", err
	}
	if m.svc != nil {
		return m.svc.serviceID + ".onion", nil
	}
	return "", nil
}

// Monitor watches the control connection and restarts Tor if it becomes
// unresponsive.  Runs in its own goroutine; exits when ctx is cancelled.
func (m *Manager) Monitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			svc := m.svc
			m.mu.Unlock()
			if svc == nil {
				continue
			}
			if _, err := svc.t.Control.GetInfo("version"); err != nil {
				log.Printf("Tor: control connection lost, restarting")
				m.mu.Lock()
				_ = svc.t.Close()
				m.svc = nil
				if err := m.startLocked(); err != nil {
					log.Printf("Tor: restart failed: %v", err)
				}
				m.mu.Unlock()
			}
		}
	}
}

// ensureTorDirs creates all required Tor directories with 0700 permissions
// and applies Chown to match the process UID/GID (non-fatal on Windows).
func ensureTorDirs(configDir, dataDir string) error {
	dirs := []string{
		filepath.Join(configDir, "tor"),
		filepath.Join(dataDir, "tor"),
		filepath.Join(dataDir, "tor", "site"),
	}
	uid := os.Getuid()
	gid := os.Getgid()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(d, 0o700); err != nil {
				return fmt.Errorf("chmod %s: %w", d, err)
			}
			// Best-effort ownership correction — non-fatal when running as non-root.
			_ = os.Chown(d, uid, gid)
		}
	}
	return nil
}

// updateTorrc overwrites the torrc with fresh generated content and restarts
// Tor so the new configuration takes effect. Callers use this for config
// changes after initial startup (initial startup uses create-only semantics).
func (m *Manager) updateTorrc() error {
	torrcPath := filepath.Join(m.cfg.ConfigDir, "tor", "torrc")
	if err := os.WriteFile(torrcPath, []byte(getTorConfig(&m.cfg)), 0o600); err != nil {
		return fmt.Errorf("updateTorrc write: %w", err)
	}
	return nil
}

// writeIfChanged writes content to path only if it differs or doesn't exist.
func writeIfChanged(path string, content []byte, perm os.FileMode) error {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(content) {
		return nil
	}
	return os.WriteFile(path, content, perm)
}

// saveKey persists the hidden service key for address stability.
func saveKey(path string, key control.Key) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(key.Blob()), 0o600)
}

// getTorConfig generates torrc content from the given Config.
// The hidden service is created via ADD_ONION, not torrc HiddenServiceDir.
func getTorConfig(cfg *Config) string {
	socksLine := "SocksPort 0"
	if cfg.UseNetwork || cfg.AllowUserPreference {
		socksLine = "SocksPort auto"
	}
	safeLog := "1"
	if !cfg.SafeLogging {
		safeLog = "0"
	}
	accounting := ""
	if cfg.MaxMonthlyBandwidth != "" && cfg.MaxMonthlyBandwidth != "unlimited" {
		accounting = fmt.Sprintf("\n# Monthly bandwidth limit\nAccountingStart month 1 00:00\nAccountingMax %s", cfg.MaxMonthlyBandwidth)
	}
	return fmt.Sprintf(`# Tor configuration — managed by pastebin server binary
# NEVER uses default ports 9050/9051 — runtime auto-ports only

# SOCKS (0 = hidden service only, auto = outbound enabled)
%s

# Control port — localhost only, runtime port selection
ControlPort 127.0.0.1:auto

# Security
SafeLogging %s

# Circuit settings
MaxCircuitDirtiness 600

# Bandwidth limits
BandwidthRate %s
BandwidthBurst %s
%s

# Not a relay or exit
ExitRelay 0
ExitPolicy reject *:*
ORPort 0
DirPort 0

# Startup optimization
FetchDirInfoEarly 1
FetchDirInfoExtraEarly 1
DisableDebuggerAttachment 1
`, socksLine, safeLog, cfg.BandwidthRate, cfg.BandwidthBurst, accounting)
}
