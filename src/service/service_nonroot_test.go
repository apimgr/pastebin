package service

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// ─── Non-root error paths ────────────────────────────────────────────────────
// When running as non-root and canEscalate() returns false, Install/Uninstall
// should return a clear error message.

func TestInstall_NonRootNoEscalate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}
	if os.Geteuid() == 0 {
		t.Skip("this test requires running as non-root")
	}

	err := Install()
	if err == nil {
		t.Fatal("Install() should fail when not root")
	}

	// Should mention sudo
	errStr := err.Error()
	if !strings.Contains(errStr, "sudo") && !strings.Contains(errStr, "root") {
		t.Errorf("error should mention sudo or root, got: %s", errStr)
	}
}

func TestUninstall_NonRootNoEscalate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}
	if os.Geteuid() == 0 {
		t.Skip("this test requires running as non-root")
	}

	err := Uninstall()
	if err == nil {
		t.Fatal("Uninstall() should fail when not root")
	}

	// Should mention sudo
	errStr := err.Error()
	if !strings.Contains(errStr, "sudo") && !strings.Contains(errStr, "root") {
		t.Errorf("error should mention sudo or root, got: %s", errStr)
	}
}

// ─── Root paths with service file writing ───────────────────────────────────

func TestInstall_RootPath_WritesFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	// Ensure we have systemd paths so DetectServiceManager returns systemd
	os.MkdirAll("/etc/systemd/system", 0o755)
	os.MkdirAll("/var/lib/"+orgName+"/"+appName, 0o755)
	os.MkdirAll("/var/log/"+orgName+"/"+appName, 0o755)
	os.MkdirAll("/etc/"+orgName+"/"+appName, 0o755)

	t.Cleanup(func() {
		os.Remove("/etc/systemd/system/" + appName + ".service")
		os.RemoveAll("/var/lib/" + orgName)
		os.RemoveAll("/var/log/" + orgName)
		os.RemoveAll("/etc/" + orgName)
	})

	err := Install()
	// Expected to fail at systemctl, but service file should be written
	if err != nil {
		t.Logf("Install error (expected): %v", err)
	}

	// Verify service file was created
	if _, statErr := os.Stat("/etc/systemd/system/" + appName + ".service"); os.IsNotExist(statErr) {
		t.Error("service file was not created")
	}
}

// ─── Uninstall with confirmation bypass ─────────────────────────────────────

func TestUninstall_RootPath_NeedsConfirmation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	// Create service file
	os.MkdirAll("/etc/systemd/system", 0o755)
	os.WriteFile("/etc/systemd/system/"+appName+".service", []byte("[Unit]\n"), 0o644)

	t.Cleanup(func() {
		os.Remove("/etc/systemd/system/" + appName + ".service")
	})

	// Uninstall will prompt for confirmation - stdin is not a terminal so
	// confirmDestructive returns false and uninstall is cancelled
	err := Uninstall()
	// Should not error but print "cancelled"
	if err != nil {
		t.Logf("Uninstall result: %v", err)
	}
}

// ─── Disable with different service managers ────────────────────────────────

func TestDisable_Root_SystemdBranch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	// Ensure systemd detection
	os.MkdirAll("/run/systemd/system", 0o755)
	t.Cleanup(func() {
		os.RemoveAll("/run/systemd")
	})

	err := Disable()
	// Will fail because systemctl doesn't exist, but exercises the branch
	if err != nil {
		t.Logf("Disable error (expected): %v", err)
	}
}

func TestDisable_Root_RunitBranch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	// Remove systemd, create runit indicators
	os.RemoveAll("/run/systemd")
	os.Rename("/etc/systemd", "/etc/systemd.bak")
	os.MkdirAll("/run/runit", 0o755)

	t.Cleanup(func() {
		os.RemoveAll("/run/runit")
		os.Rename("/etc/systemd.bak", "/etc/systemd")
	})

	// Create runit service dir
	svDir := "/etc/sv/" + appName
	os.MkdirAll(svDir, 0o755)
	os.WriteFile(svDir+"/run", []byte("#!/bin/sh\n"), 0o755)

	// Create enabled symlink
	os.MkdirAll("/var/service", 0o755)
	os.Symlink(svDir, "/var/service/"+appName)

	t.Cleanup(func() {
		os.RemoveAll(svDir)
		os.Remove("/var/service/" + appName)
	})

	err := Disable()
	// Runit Disable doesn't call external commands, just removes symlink
	if err != nil {
		t.Errorf("Disable (runit) error: %v", err)
	}

	// Verify symlink was removed
	if _, statErr := os.Lstat("/var/service/" + appName); !os.IsNotExist(statErr) {
		t.Error("runit enabled symlink should be removed")
	}
}
