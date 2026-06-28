//go:build windows

package service

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

// isElevated reports whether the current process has administrator privileges on Windows.
func isElevated() bool {
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
		return false
	}
	defer windows.FreeSid(sid)
	ok, err := windows.Token(0).IsMember(sid)
	if err != nil {
		return false
	}
	return ok
}

// canEscalate reports whether runas is available for privilege escalation on Windows.
func canEscalate() bool {
	_, err := exec.LookPath("runas")
	return err == nil
}

// execElevated re-executes the current process with elevated privileges via
// ShellExecute "runas" verb, which triggers the UAC elevation prompt.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(self)
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_NORMAL)
}
