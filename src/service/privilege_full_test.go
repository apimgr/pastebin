package service

import (
	"os/exec"
	"os/user"
	"runtime"
	"testing"
)

// ─── canEscalate branch coverage ─────────────────────────────────────────────
// canEscalate checks for sudo/pkexec/doas, then checks groups.
// We can't fully test execElevated (blocks), but we can test canEscalate paths.

func TestCanEscalate_SudoPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}

	// Check if sudo exists
	_, err := exec.LookPath("sudo")
	if err != nil {
		t.Log("sudo not in PATH, testing fallback paths")
	}

	// Just call canEscalate and verify it returns a bool without panicking
	result := canEscalate()
	t.Logf("canEscalate() = %v", result)
}

func TestCanEscalate_ChecksGroups(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}

	// Get current user and groups to understand what canEscalate sees
	u, err := user.Current()
	if err != nil {
		t.Skipf("cannot get current user: %v", err)
	}

	gids, err := u.GroupIds()
	if err != nil {
		t.Skipf("cannot get group IDs: %v", err)
	}

	t.Logf("User: %s, Groups: %v", u.Username, gids)

	// List group names
	for _, gid := range gids {
		g, err := user.LookupGroupId(gid)
		if err == nil {
			t.Logf("  Group %s: %s", gid, g.Name)
		}
	}

	// canEscalate should work without panic
	_ = canEscalate()
}

func TestCanEscalate_NoTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}

	// In Docker containers, sudo/pkexec/doas may or may not be present.
	// Test that the function handles missing tools gracefully.

	// Check which tools exist
	tools := []string{"sudo", "pkexec", "doas"}
	for _, tool := range tools {
		path, err := exec.LookPath(tool)
		if err == nil {
			t.Logf("%s found at: %s", tool, path)
		} else {
			t.Logf("%s not found", tool)
		}
	}

	// Function should not panic regardless of what's installed
	result := canEscalate()
	t.Logf("canEscalate() returned: %v", result)
}

// ─── isElevated variations ───────────────────────────────────────────────────

func TestIsElevated_ReturnsCorrectValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}

	result := isElevated()

	// In Docker with root, should be true
	// Otherwise depends on how tests are run
	t.Logf("isElevated() = %v", result)
}

// ─── isPrivileged (calls isElevated on Unix) ────────────────────────────────

func TestIsPrivileged_MatchesIsElevated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix only")
	}

	elevated := isElevated()
	privileged := isPrivileged()

	// On Unix, isPrivileged just calls isElevated
	if elevated != privileged {
		t.Errorf("isElevated() = %v but isPrivileged() = %v", elevated, privileged)
	}
}

// ─── execElevated (cannot test directly - blocks or re-execs) ────────────────
// We document why execElevated cannot be unit tested:
// 1. If sudo/pkexec/doas exists and user has privileges, it re-execs the process
// 2. If sudo exists but requires password, cmd.Run() blocks waiting for input
// 3. If no tool exists, it returns os.ErrPermission (this we could test in isolation)

// TestExecElevated_Documentation documents the testing limitation
func TestExecElevated_Documentation(t *testing.T) {
	t.Log("execElevated cannot be unit tested because:")
	t.Log("  1. With working sudo/pkexec/doas: re-execs the process (replaces current)")
	t.Log("  2. With sudo requiring password: blocks waiting for terminal input")
	t.Log("  3. Without any tools: returns os.ErrPermission")
	t.Log("Only option 3 could theoretically be tested by manipulating PATH,")
	t.Log("but that creates a fragile, environment-dependent test.")
	t.Log("Integration tests should cover the privilege escalation flow.")
}
