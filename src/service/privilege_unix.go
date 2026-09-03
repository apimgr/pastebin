//go:build !windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

// isElevated reports whether the current process is running as root (UID 0).
func isElevated() bool {
	return os.Geteuid() == 0
}

// escalationTools returns the ordered list of privilege-escalation tools to
// try for the current platform, per AI.md PART 23 "Escalation Detection by OS".
//
// Linux:   sudo, su, pkexec, doas
// macOS:   sudo, osascript
// BSD:     doas, sudo, su
func escalationTools() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"sudo", "osascript"}
	case "freebsd", "openbsd", "netbsd", "dragonfly":
		return []string{"doas", "sudo", "su"}
	default:
		return []string{"sudo", "su", "pkexec", "doas"}
	}
}

// userInPrivilegedGroup reports whether the current user belongs to a
// well-known privileged group (wheel, sudo, admin).
func userInPrivilegedGroup() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	gids, err := u.GroupIds()
	if err != nil {
		return false
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
	return false
}

// canEscalate reports whether privilege escalation is possible via any of the
// platform's ordered escalation tools (see escalationTools). sudo is verified
// with a non-interactive passwordless check, falling back to group membership.
// su and osascript require an interactive password/GUI prompt that cannot be
// probed non-interactively, so they are treated as available whenever the
// binary itself is present.
func canEscalate() bool {
	for _, tool := range escalationTools() {
		path, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		switch tool {
		case "sudo":
			cmd := exec.Command(path, "-n", "true")
			if cmd.Run() == nil {
				return true
			}
			if userInPrivilegedGroup() {
				return true
			}
		case "su", "osascript":
			return true
		case "pkexec", "doas":
			if userInPrivilegedGroup() {
				return true
			}
		}
	}
	return false
}

// execElevated re-executes the current process with elevated privileges using
// the first available tool from escalationTools() (first available). Returns
// an error if no escalation tool is available or if the re-exec fails.
func execElevated() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := os.Args[1:]
	for _, tool := range escalationTools() {
		path, err := exec.LookPath(tool)
		if err != nil {
			continue
		}
		cmd := buildElevationCmd(path, tool, self, args)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	return os.ErrPermission
}

// buildElevationCmd constructs the exec.Cmd used to re-exec self with args
// under the given escalation tool. sudo, pkexec, and doas accept the target
// command directly as trailing arguments. su and osascript instead require
// the full command line to be passed as a single shell-quoted string.
func buildElevationCmd(path, tool, self string, args []string) *exec.Cmd {
	switch tool {
	case "su":
		// su -c requires the command as a single shell string and prompts
		// interactively for the root password.
		return exec.Command(path, "-c", shellJoin(append([]string{self}, args...)))
	case "osascript":
		// osascript triggers a native GUI prompt via AppleScript's
		// "do shell script ... with administrator privileges".
		script := fmt.Sprintf("do shell script %q with administrator privileges", shellJoin(append([]string{self}, args...)))
		return exec.Command(path, "-e", script)
	default:
		return exec.Command(path, append([]string{self}, args...)...)
	}
}

// shellJoin quotes and joins args into a single POSIX shell command string
// suitable for passing to `su -c` or an AppleScript `do shell script`.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps s in single quotes, escaping any embedded single quotes
// so the result is safe to interpolate into a POSIX shell command line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
