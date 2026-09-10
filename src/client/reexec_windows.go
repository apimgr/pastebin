//go:build windows

package main

import "fmt"

// reExec is a Windows stub that informs the user to restart manually.
// syscall.Exec is not available on Windows; the binary has been replaced on disk.
func reExec(exe string) error {
	fmt.Println(tf("reexec_manual_restart", "binary", binName()))
	return nil
}
