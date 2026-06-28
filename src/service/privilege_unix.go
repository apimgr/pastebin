//go:build !windows

package service

import (
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// isElevated reports whether the current process is running as root (UID 0).
func isElevated() bool {
	return os.Geteuid() == 0
}

// canEscalate reports whether privilege escalation is possible via sudo, pkexec, or doas.
// It checks whether the invoking user is in a privileged group (wheel, sudo, admin)
// or whether a no-password sudo path is available.
func canEscalate() bool {
	for _, tool := range []string{"sudo", "pkexec", "doas"} {
		if _, err := exec.LookPath(tool); err == nil {
			if tool == "sudo" {
				cmd := exec.Command("sudo", "-n", "true")
				if cmd.Run() == nil {
					return true
				}
			}
			u, err := user.Current()
			if err != nil {
				continue
			}
			gids, err := u.GroupIds()
			if err != nil {
				continue
			}
			for _, gid := range gids {
				g, err := user.LookupGroupId(gid)
				if err != nil {
					continue
				}
				name := strings.ToLower(g.Name)
				if name == "wheel" || name == "sudo" || name == "admin" {
					return true
				}
			}
		}
	}
	return false
}

// execElevated re-executes the current process with elevated privileges using
// sudo, pkexec, or doas (first available). Returns an error if no escalation
// tool is available or if the re-exec fails.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := os.Args[1:]
	for _, tool := range []string{"sudo", "pkexec", "doas"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, append([]string{self}, args...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return os.ErrPermission
}
