//go:build windows

package service

import (
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
)

// administratorsSID allocates the BUILTIN\Administrators SID. The caller must
// release it with windows.FreeSid.
func administratorsSID() (*windows.SID, error) {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return nil, err
	}
	return sid, nil
}

// isElevated reports whether the current process has administrator privileges on Windows.
func isElevated() bool {
	sid, err := administratorsSID()
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	ok, err := windows.Token(0).IsMember(sid)
	if err != nil {
		return false
	}
	return ok
}

// inAdministratorsGroup reports whether the account owning this process belongs
// to BUILTIN\Administrators, including the UAC-filtered case where the group is
// present only as a deny-only entry (which makes Token.IsMember report false).
// This is what decides whether a UAC prompt could actually succeed.
func inAdministratorsGroup() bool {
	sid, err := administratorsSID()
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	groups, err := windows.Token(0).GetTokenGroups()
	if err != nil {
		return false
	}
	for _, g := range groups.AllGroups() {
		if g.Sid != nil && g.Sid.Equals(sid) {
			return true
		}
	}
	return false
}

// canEscalate reports whether privilege escalation is possible on Windows, in
// the PART 23 order: already-elevated administrator token, then a UAC prompt
// (possible only when the account is in Administrators), then runas.
//
// It never reports true for an account that cannot actually escalate, so the
// caller shows an informative error instead of raising a prompt that must fail.
func canEscalate() bool {
	if isElevated() || inAdministratorsGroup() {
		return true
	}
	_, err := exec.LookPath("runas")
	return err == nil
}

// quoteWindowsArg wraps an argument for a Windows command line, escaping any
// embedded quotes so it survives CommandLineToArgvW parsing in the child.
func quoteWindowsArg(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// windowsArgLine joins the current process arguments into a single command-line
// string for ShellExecute, which takes parameters as one string rather than a
// vector. The original arguments are preserved so the elevated process performs
// the same operation the unelevated one was asked to.
func windowsArgLine(args []string) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = quoteWindowsArg(a)
	}
	return strings.Join(parts, " ")
}

// execElevated re-executes the current process, with its original arguments,
// under elevated privileges via the ShellExecute "runas" verb (the UAC prompt).
// If ShellExecute fails, it falls back to the runas command-line tool.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(self)
	if err != nil {
		return err
	}

	var params *uint16
	if line := windowsArgLine(os.Args[1:]); line != "" {
		params, err = windows.UTF16PtrFromString(line)
		if err != nil {
			return err
		}
	}

	shellErr := windows.ShellExecute(0, verb, file, params, nil, windows.SW_NORMAL)
	if shellErr == nil {
		return nil
	}

	runas, lookErr := exec.LookPath("runas")
	if lookErr != nil {
		return shellErr
	}
	cmd := exec.Command(runas, "/user:Administrator",
		strings.TrimSpace(quoteWindowsArg(self)+" "+windowsArgLine(os.Args[1:])))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
