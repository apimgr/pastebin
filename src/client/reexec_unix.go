//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// reExec replaces the current process image with the binary at exe.
// On Unix systems this uses syscall.Exec (an atomic replacement).
func reExec(exe string) error {
	fmt.Println("update applied — restarting...")
	return syscall.Exec(exe, os.Args, os.Environ())
}
