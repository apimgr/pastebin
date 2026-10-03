//go:build !gui || freebsd || netbsd || openbsd

package main

// guiAvailable reports whether a native GUI can be launched. Without the
// `gui` build tag — or on an OS with no gogpu/ui windowing backend (netbsd,
// openbsd) — no GUI code is linked in at all, so GUI mode is never available.
func guiAvailable() bool {
	return false
}
