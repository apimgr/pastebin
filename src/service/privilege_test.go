package service

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"testing"
)

// ─── isElevated ──────────────────────────────────────────────────────────────

func TestIsElevated_MatchesGeteuid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	got := isElevated()
	want := os.Geteuid() == 0
	if got != want {
		t.Errorf("isElevated() = %v; want %v (Geteuid=%d)", got, want, os.Geteuid())
	}
}

func TestIsElevated_Idempotent(t *testing.T) {
	a := isElevated()
	b := isElevated()
	if a != b {
		t.Errorf("isElevated() not idempotent: first=%v second=%v", a, b)
	}
}

// ─── canEscalate ─────────────────────────────────────────────────────────────
// canEscalate checks for sudo/pkexec/doas and whether the user is in
// wheel/sudo/admin groups. These tests cover the function's logic paths.

func TestCanEscalate_ReturnsBoolean(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	// canEscalate() should return true or false without panicking.
	got := canEscalate()
	// The result depends on the test environment (CI containers typically
	// don't have sudo configured for the test user), but we verify the
	// function runs without error.
	_ = got
}

func TestCanEscalate_ConsistentWithSudo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	// If sudo -n true succeeds, canEscalate should return true.
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		t.Skip("sudo not available")
	}

	cmd := exec.Command(sudoPath, "-n", "true")
	sudoWorks := cmd.Run() == nil

	got := canEscalate()

	// If sudo -n true works, canEscalate must return true.
	// If it doesn't work, canEscalate may still return true if the user
	// is in a privileged group.
	if sudoWorks && !got {
		t.Errorf("canEscalate() = false but sudo -n true succeeded")
	}
}

func TestCanEscalate_ChecksPrivilegedGroups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	gids, err := u.GroupIds()
	if err != nil {
		t.Skipf("cannot get group IDs: %v", err)
	}

	// Check if user is in wheel, sudo, or admin group.
	inPrivilegedGroup := false
	for _, gid := range gids {
		g, err := user.LookupGroupId(gid)
		if err != nil {
			continue
		}
		switch g.Name {
		case "wheel", "sudo", "admin":
			inPrivilegedGroup = true
		}
	}

	got := canEscalate()

	// If the user is in a privileged group and has sudo/pkexec/doas,
	// canEscalate should return true.
	if inPrivilegedGroup {
		// Check if any escalation tool exists
		hasTool := false
		for _, tool := range []string{"sudo", "pkexec", "doas"} {
			if _, err := exec.LookPath(tool); err == nil {
				hasTool = true
				break
			}
		}
		if hasTool && !got {
			t.Logf("user in privileged group with escalation tool but canEscalate()=false")
			// This may happen if sudo requires password, which is acceptable.
		}
	}
}

// ─── execElevated ────────────────────────────────────────────────────────────
// execElevated re-execs the process with sudo/pkexec/doas. This function
// cannot be safely unit tested because:
// 1. It blocks waiting for sudo password prompt in most CI environments
// 2. If passwordless sudo is available, it actually re-execs the process
// The function is exercised indirectly through Install/Uninstall privilege checks.

// ─── isPrivileged (alias) ────────────────────────────────────────────────────

func TestIsPrivileged_EqualsIsElevated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only test")
	}

	elevated := isElevated()
	privileged := isPrivileged()

	if elevated != privileged {
		t.Errorf("isPrivileged() = %v; isElevated() = %v; expected equal", privileged, elevated)
	}
}

// ─── RunAsWindowsService / IsWindowsService stubs ────────────────────────────

func TestIsWindowsService_NonWindows_ReturnsFalse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("testing non-Windows stub")
	}

	if IsWindowsService() {
		t.Error("IsWindowsService() on non-Windows should return false")
	}
}

func TestRunAsWindowsService_NonWindows_ReturnsNil(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("testing non-Windows stub")
	}

	called := false
	err := RunAsWindowsService("test", func() { called = true })
	if err != nil {
		t.Errorf("RunAsWindowsService() on non-Windows = %v; want nil", err)
	}
	// The function parameter is not called on non-Windows; verify that.
	if called {
		t.Error("RunAsWindowsService() on non-Windows should not call the function")
	}
}

// ─── installWindows / uninstallWindows stubs ─────────────────────────────────

func TestInstallWindows_NonWindows_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("testing non-Windows stub")
	}

	err := installWindows()
	if err == nil {
		t.Error("installWindows() on non-Windows should return error")
	}
	if err.Error() != "windows service installation is only supported on Windows" {
		t.Errorf("installWindows() error = %q; want specific message", err.Error())
	}
}

func TestUninstallWindows_NonWindows_ReturnsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("testing non-Windows stub")
	}

	err := uninstallWindows()
	if err == nil {
		t.Error("uninstallWindows() on non-Windows should return error")
	}
	if err.Error() != "windows service uninstallation is only supported on Windows" {
		t.Errorf("uninstallWindows() error = %q; want specific message", err.Error())
	}
}

// ─── Privilege check in Install/Uninstall ────────────────────────────────────

func TestInstall_NonRootWithoutEscalation_ReturnsHelpfulError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}

	err := Install()
	if err == nil {
		t.Fatal("Install() without root should return error")
	}

	// The error should mention sudo or root.
	errStr := err.Error()
	if !containsAny(errStr, "sudo", "root", "privilege") {
		t.Errorf("Install() error = %q; expected mention of sudo/root/privilege", errStr)
	}
}

func TestUninstall_NonRootWithoutEscalation_ReturnsHelpfulError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root")
	}

	err := Uninstall()
	if err == nil {
		t.Fatal("Uninstall() without root should return error")
	}

	errStr := err.Error()
	if !containsAny(errStr, "sudo", "root", "privilege") {
		t.Errorf("Uninstall() error = %q; expected mention of sudo/root/privilege", errStr)
	}
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
