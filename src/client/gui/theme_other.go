//go:build gui && !linux && !darwin && !windows && !freebsd && !netbsd && !openbsd

package gui

// systemPrefersDark falls back to the dark default on BSD and any other
// platform without a documented detection method in AI.md's "System Theme
// Detection" table.
func systemPrefersDark() bool {
	return true
}
