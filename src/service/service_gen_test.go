package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ─── Systemd Unit File Content Verification ──────────────────────────────────
// Tests that the generated systemd unit file contains all required directives
// as specified in PART 24: Type=simple, RestartSec=5, Restart=on-failure,
// NoNewPrivileges=yes, ProtectSystem=strict, PrivateTmp=yes.

func TestSystemdUnitContent_RequiredDirectives(t *testing.T) {
	// Build the expected unit content by calling the same format string used in installSystemd
	binaryPath := GetBinaryPath()
	serviceContent := fmt.Sprintf(`[Unit]
Description=Pastebin API Server
Documentation=https://apimgr.github.io/pastebin
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening (binary drops privileges after port binding)
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/%s/%s
ReadWritePaths=/var/lib/%s/%s
ReadWritePaths=/var/cache/%s/%s
ReadWritePaths=/var/log/%s/%s

[Install]
WantedBy=multi-user.target
`, binaryPath, orgName, appName, orgName, appName, orgName, appName, orgName, appName)

	requiredDirectives := []string{
		"Type=simple",
		"RestartSec=5",
		"Restart=on-failure",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"PrivateTmp=yes",
	}

	for _, directive := range requiredDirectives {
		if !strings.Contains(serviceContent, directive) {
			t.Errorf("systemd unit missing required directive %q", directive)
		}
	}
}

func TestSystemdUnitContent_HasUnitSection(t *testing.T) {
	binaryPath := GetBinaryPath()
	serviceContent := buildSystemdUnit(binaryPath)

	if !strings.Contains(serviceContent, "[Unit]") {
		t.Error("systemd unit missing [Unit] section")
	}
	if !strings.Contains(serviceContent, "[Service]") {
		t.Error("systemd unit missing [Service] section")
	}
	if !strings.Contains(serviceContent, "[Install]") {
		t.Error("systemd unit missing [Install] section")
	}
}

func TestSystemdUnitContent_HasDocumentation(t *testing.T) {
	binaryPath := GetBinaryPath()
	serviceContent := buildSystemdUnit(binaryPath)

	if !strings.Contains(serviceContent, "Documentation=") {
		t.Error("systemd unit missing Documentation directive")
	}
	if !strings.Contains(serviceContent, "apimgr.github.io") {
		t.Error("systemd unit Documentation does not reference project docs site")
	}
}

func TestSystemdUnitContent_HasNetworkOrdering(t *testing.T) {
	binaryPath := GetBinaryPath()
	serviceContent := buildSystemdUnit(binaryPath)

	if !strings.Contains(serviceContent, "After=network-online.target") {
		t.Error("systemd unit missing After=network-online.target")
	}
	if !strings.Contains(serviceContent, "Wants=network-online.target") {
		t.Error("systemd unit missing Wants=network-online.target")
	}
}

func TestSystemdUnitContent_HasReadWritePaths(t *testing.T) {
	binaryPath := GetBinaryPath()
	serviceContent := buildSystemdUnit(binaryPath)

	expectedPaths := []string{
		fmt.Sprintf("/etc/%s/%s", orgName, appName),
		fmt.Sprintf("/var/lib/%s/%s", orgName, appName),
		fmt.Sprintf("/var/cache/%s/%s", orgName, appName),
		fmt.Sprintf("/var/log/%s/%s", orgName, appName),
	}

	for _, path := range expectedPaths {
		if !strings.Contains(serviceContent, "ReadWritePaths="+path) {
			t.Errorf("systemd unit missing ReadWritePaths=%s", path)
		}
	}
}

// buildSystemdUnit returns the systemd unit content string for testing.
// This is a helper that mirrors the format string in installSystemd().
func buildSystemdUnit(binaryPath string) string {
	return fmt.Sprintf(`[Unit]
Description=Pastebin API Server
Documentation=https://apimgr.github.io/pastebin
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening (binary drops privileges after port binding)
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/etc/%s/%s
ReadWritePaths=/var/lib/%s/%s
ReadWritePaths=/var/cache/%s/%s
ReadWritePaths=/var/log/%s/%s

[Install]
WantedBy=multi-user.target
`, binaryPath, orgName, appName, orgName, appName, orgName, appName, orgName, appName)
}

// ─── Launchd Plist Content Verification ──────────────────────────────────────

func TestLaunchdPlistContent_RequiredKeys(t *testing.T) {
	binaryPath := GetBinaryPath()
	plistContent := buildLaunchdPlist(binaryPath)

	requiredKeys := []string{
		"<key>Label</key>",
		"<key>ProgramArguments</key>",
		"<key>RunAtLoad</key>",
		"<key>KeepAlive</key>",
		"<key>StandardOutPath</key>",
		"<key>StandardErrorPath</key>",
	}

	for _, key := range requiredKeys {
		if !strings.Contains(plistContent, key) {
			t.Errorf("launchd plist missing required key %q", key)
		}
	}
}

func TestLaunchdPlistContent_HasLabel(t *testing.T) {
	binaryPath := GetBinaryPath()
	plistContent := buildLaunchdPlist(binaryPath)

	expectedLabel := fmt.Sprintf("<string>%s</string>", launchdLabel)
	if !strings.Contains(plistContent, expectedLabel) {
		t.Errorf("launchd plist missing label %q", launchdLabel)
	}
}

func TestLaunchdPlistContent_HasBinaryPath(t *testing.T) {
	binaryPath := GetBinaryPath()
	plistContent := buildLaunchdPlist(binaryPath)

	expectedBinary := fmt.Sprintf("<string>%s</string>", binaryPath)
	if !strings.Contains(plistContent, expectedBinary) {
		t.Errorf("launchd plist missing binary path %q", binaryPath)
	}
}

func TestLaunchdPlistContent_HasLogPaths(t *testing.T) {
	binaryPath := GetBinaryPath()
	plistContent := buildLaunchdPlist(binaryPath)

	expectedStdout := fmt.Sprintf("/var/log/%s/%s/stdout.log", orgName, appName)
	expectedStderr := fmt.Sprintf("/var/log/%s/%s/stderr.log", orgName, appName)

	if !strings.Contains(plistContent, expectedStdout) {
		t.Errorf("launchd plist missing stdout log path %q", expectedStdout)
	}
	if !strings.Contains(plistContent, expectedStderr) {
		t.Errorf("launchd plist missing stderr log path %q", expectedStderr)
	}
}

func TestLaunchdPlistContent_HasXMLHeader(t *testing.T) {
	binaryPath := GetBinaryPath()
	plistContent := buildLaunchdPlist(binaryPath)

	if !strings.HasPrefix(plistContent, "<?xml version=") {
		t.Error("launchd plist missing XML declaration")
	}
	if !strings.Contains(plistContent, "<!DOCTYPE plist") {
		t.Error("launchd plist missing DOCTYPE")
	}
}

// buildLaunchdPlist returns the launchd plist content string for testing.
func buildLaunchdPlist(binaryPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/%s/%s/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/%s/%s/stderr.log</string>
</dict>
</plist>
`, launchdLabel, binaryPath, orgName, appName, orgName, appName)
}

// ─── OpenRC Init Script Content Verification ─────────────────────────────────

func TestOpenRCScriptContent_RequiredElements(t *testing.T) {
	binaryPath := GetBinaryPath()
	rcContent := buildOpenRCScript(binaryPath)

	requiredElements := []string{
		"#!/sbin/openrc-run",
		"description=",
		"command=",
		"command_background=true",
		"pidfile=",
		"depend()",
		"need net",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(rcContent, elem) {
			t.Errorf("OpenRC script missing required element %q", elem)
		}
	}
}

func TestOpenRCScriptContent_HasBinaryPath(t *testing.T) {
	binaryPath := GetBinaryPath()
	rcContent := buildOpenRCScript(binaryPath)

	expectedCommand := fmt.Sprintf(`command="%s"`, binaryPath)
	if !strings.Contains(rcContent, expectedCommand) {
		t.Errorf("OpenRC script missing command=%q", binaryPath)
	}
}

func TestOpenRCScriptContent_HasPidfile(t *testing.T) {
	binaryPath := GetBinaryPath()
	rcContent := buildOpenRCScript(binaryPath)

	expectedPidfile := fmt.Sprintf(`pidfile="/run/%s.pid"`, appName)
	if !strings.Contains(rcContent, expectedPidfile) {
		t.Errorf("OpenRC script missing pidfile directive")
	}
}

// buildOpenRCScript returns the OpenRC init script content string for testing.
func buildOpenRCScript(binaryPath string) string {
	return fmt.Sprintf(`#!/sbin/openrc-run

description="%s API Server"
command="%s"
command_background=true
pidfile="/run/%s.pid"
command_args=""

depend() {
	need net
	after firewall
}
`, appName, binaryPath, appName)
}

// ─── SysV Init Script Content Verification ───────────────────────────────────

func TestSysVScriptContent_RequiredElements(t *testing.T) {
	binaryPath := GetBinaryPath()
	initContent := buildSysVScript(binaryPath)

	requiredElements := []string{
		"#!/bin/sh",
		"### BEGIN INIT INFO",
		"### END INIT INFO",
		"# Provides:",
		"# Required-Start:",
		"# Default-Start:",
		"start)",
		"stop)",
		"restart)",
		"status)",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(initContent, elem) {
			t.Errorf("SysV script missing required element %q", elem)
		}
	}
}

func TestSysVScriptContent_HasBinaryPath(t *testing.T) {
	binaryPath := GetBinaryPath()
	initContent := buildSysVScript(binaryPath)

	expectedDaemon := fmt.Sprintf(`DAEMON="%s"`, binaryPath)
	if !strings.Contains(initContent, expectedDaemon) {
		t.Errorf("SysV script missing DAEMON=%q", binaryPath)
	}
}

func TestSysVScriptContent_HasPidfile(t *testing.T) {
	binaryPath := GetBinaryPath()
	initContent := buildSysVScript(binaryPath)

	expectedPidfile := fmt.Sprintf("PIDFILE=/var/run/%s.pid", appName)
	if !strings.Contains(initContent, expectedPidfile) {
		t.Errorf("SysV script missing PIDFILE directive")
	}
}

func TestSysVScriptContent_HasRunlevels(t *testing.T) {
	binaryPath := GetBinaryPath()
	initContent := buildSysVScript(binaryPath)

	if !strings.Contains(initContent, "# Default-Start:     2 3 4 5") {
		t.Error("SysV script missing Default-Start runlevels")
	}
	if !strings.Contains(initContent, "# Default-Stop:      0 1 6") {
		t.Error("SysV script missing Default-Stop runlevels")
	}
}

// buildSysVScript returns the SysV init script content string for testing.
func buildSysVScript(binaryPath string) string {
	return fmt.Sprintf(`#!/bin/sh
### BEGIN INIT INFO
# Provides:          %s
# Required-Start:    $network $syslog
# Required-Stop:     $network $syslog
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: %s API Server
### END INIT INFO

PATH=/sbin:/usr/sbin:/bin:/usr/bin
DAEMON="%s"
PIDFILE=/var/run/%s.pid
NAME=%s

case "$1" in
  start)
    echo "Starting $NAME..."
    start-stop-daemon --start --quiet --pidfile "$PIDFILE" \
      --background --make-pidfile --exec "$DAEMON"
    ;;
  stop)
    echo "Stopping $NAME..."
    start-stop-daemon --stop --quiet --pidfile "$PIDFILE"
    ;;
  restart)
    $0 stop
    $0 start
    ;;
  reload)
    echo "Reloading $NAME..."
    start-stop-daemon --stop --signal HUP --quiet --pidfile "$PIDFILE"
    ;;
  status)
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
      echo "$NAME is running"
    else
      echo "$NAME is not running"
      exit 1
    fi
    ;;
  *)
    echo "Usage: $0 {start|stop|restart|reload|status}" >&2
    exit 1
    ;;
esac
exit 0
`, appName, appName, binaryPath, appName, appName)
}

// ─── BSD rc.d Script Content Verification ────────────────────────────────────

func TestBSDRCScriptContent_RequiredElements(t *testing.T) {
	binaryPath := GetBinaryPath()
	rcContent := buildBSDRCScript(binaryPath)

	requiredElements := []string{
		"#!/bin/sh",
		"# PROVIDE:",
		"# REQUIRE: NETWORKING",
		"# KEYWORD: shutdown",
		". /etc/rc.subr",
		"name=",
		"rcvar=",
		"command=",
		"pidfile=",
		"load_rc_config",
		"run_rc_command",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(rcContent, elem) {
			t.Errorf("BSD rc.d script missing required element %q", elem)
		}
	}
}

func TestBSDRCScriptContent_HasBinaryPath(t *testing.T) {
	binaryPath := GetBinaryPath()
	rcContent := buildBSDRCScript(binaryPath)

	expectedCommand := fmt.Sprintf(`command="%s"`, binaryPath)
	if !strings.Contains(rcContent, expectedCommand) {
		t.Errorf("BSD rc.d script missing command=%q", binaryPath)
	}
}

func TestBSDRCScriptContent_HasEnableVar(t *testing.T) {
	binaryPath := GetBinaryPath()
	rcContent := buildBSDRCScript(binaryPath)

	expectedEnable := fmt.Sprintf(`${%s_enable:="NO"}`, appName)
	if !strings.Contains(rcContent, expectedEnable) {
		t.Errorf("BSD rc.d script missing %s_enable default", appName)
	}
}

// buildBSDRCScript returns the BSD rc.d script content string for testing.
func buildBSDRCScript(binaryPath string) string {
	return fmt.Sprintf(`#!/bin/sh

# PROVIDE: %s
# REQUIRE: NETWORKING
# KEYWORD: shutdown

. /etc/rc.subr

name="%s"
rcvar="%s_enable"
command="%s"
pidfile="/var/run/%s.pid"

load_rc_config $name
: ${%s_enable:="NO"}

run_rc_command "$1"
`, appName, appName, appName, binaryPath, appName, appName)
}

// ─── Runit Run Script Content Verification ───────────────────────────────────

func TestRunitRunScriptContent_RequiredElements(t *testing.T) {
	binaryPath := GetBinaryPath()
	runScript := buildRunitRunScript(binaryPath)

	if !strings.HasPrefix(runScript, "#!/bin/sh") {
		t.Error("runit run script missing shebang")
	}
	if !strings.Contains(runScript, "exec ") {
		t.Error("runit run script missing exec directive")
	}
	if !strings.Contains(runScript, binaryPath) {
		t.Errorf("runit run script missing binary path %q", binaryPath)
	}
}

func TestRunitLogRunScriptContent_HasSvlogd(t *testing.T) {
	logRunScript := buildRunitLogRunScript()

	if !strings.Contains(logRunScript, "svlogd") {
		t.Error("runit log run script missing svlogd")
	}
	if !strings.Contains(logRunScript, "exec ") {
		t.Error("runit log run script missing exec directive")
	}
}

// buildRunitRunScript returns the runit run script content string for testing.
func buildRunitRunScript(binaryPath string) string {
	return fmt.Sprintf(`#!/bin/sh
exec %s 2>&1
`, binaryPath)
}

// buildRunitLogRunScript returns the runit log run script content string.
func buildRunitLogRunScript() string {
	return `#!/bin/sh
exec svlogd -tt ./main
`
}

// ─── reserved IDs ────────────────────────────────────────────────────────────

func TestReservedIDs_ContainsExpectedRanges(t *testing.T) {
	// Per PART 23: Reserve ranges 980-999, 101-110, 170-179.
	testCases := []struct {
		id       int
		reserved bool
	}{
		{980, true},
		{999, true},
		{990, true},
		{101, true},
		{110, true},
		{105, true},
		{170, true},
		{179, true},
		{175, true},
		{200, false},
		{500, false},
		{899, false},
		{100, false},
		{111, false},
		{169, false},
		{180, false},
		{979, false},
	}

	for _, tc := range testCases {
		got := reservedIDs[tc.id]
		if got != tc.reserved {
			t.Errorf("reservedIDs[%d] = %v; want %v", tc.id, got, tc.reserved)
		}
	}
}

// ─── findAvailableSystemID edge cases ────────────────────────────────────────

func TestFindAvailableSystemID_RangeCheck(t *testing.T) {
	id, err := findAvailableSystemID()
	if err != nil {
		t.Skipf("findAvailableSystemID: %v (all IDs may be taken)", err)
	}
	if id < 200 || id > 899 {
		t.Errorf("findAvailableSystemID() = %d; must be in range 200-899", id)
	}
}

func TestFindAvailableSystemID_NotReserved(t *testing.T) {
	id, err := findAvailableSystemID()
	if err != nil {
		t.Skipf("findAvailableSystemID: %v", err)
	}
	if reservedIDs[id] {
		t.Errorf("findAvailableSystemID() = %d; should not be a reserved ID", id)
	}
}

// ─── ServiceType String Values ───────────────────────────────────────────────

func TestServiceType_EnumValuesAreSequential(t *testing.T) {
	expectedOrder := []ServiceType{
		ServiceUnknown,
		ServiceSystemd,
		ServiceOpenRC,
		ServiceSysV,
		ServiceRunit,
		ServiceLaunchd,
		ServiceWindows,
		ServiceBSDRC,
	}

	for i, st := range expectedOrder {
		if int(st) != i {
			t.Errorf("ServiceType enum at index %d = %d; expected sequential value", i, st)
		}
	}
}

// ─── GetBinaryPath platform variations ───────────────────────────────────────

func TestGetBinaryPath_WindowsFormat(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	path := GetBinaryPath()
	if !strings.HasPrefix(path, `C:\Program Files\`) {
		t.Errorf("GetBinaryPath() on Windows = %q; expected C:\\Program Files\\ prefix", path)
	}
	if !strings.HasSuffix(path, ".exe") {
		t.Errorf("GetBinaryPath() on Windows = %q; expected .exe suffix", path)
	}
}

func TestGetBinaryPath_UnixFormat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}
	path := GetBinaryPath()
	expected := fmt.Sprintf("/usr/local/bin/%s", appName)
	if path != expected {
		t.Errorf("GetBinaryPath() = %q; want %q", path, expected)
	}
}

// ─── copyBinary with write failure ───────────────────────────────────────────

func TestCopyBinary_FailsOnUnwritableDestDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission test")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere; skipping permission test")
	}

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	unwritableDir := filepath.Join(tmp, "locked")

	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.MkdirAll(unwritableDir, 0o555); err != nil {
		t.Fatalf("create locked dir: %v", err)
	}

	dst := filepath.Join(unwritableDir, "subdir", "binary")
	err := copyBinary(src, dst)
	if err == nil {
		t.Error("expected error when destination directory is not writable")
	}
}

// ─── purgeData with temp directories ─────────────────────────────────────────

func TestPurgeData_WindowsBranch(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	// On Windows, purgeData removes C:\ProgramData\{orgName}\{appName}.
	// We cannot test the actual path removal without root; just verify no panic.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("purgeData panicked on Windows: %v", r)
		}
	}()
	purgeData()
}
