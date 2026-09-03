//go:build windows

package server

// Windows uses a Virtual Service Account configured at service-install time
// (PART 23); there is no runtime UID/GID drop, so these are inert stubs.

// currentlyRoot always reports false on Windows.
func currentlyRoot() bool {
	return false
}

// resolvePrivDropTarget never resolves a target on Windows.
func resolvePrivDropTarget(_, _ string) (uid, gid int, name string, ok bool) {
	return 0, 0, "", false
}

// dropPrivileges is a no-op on Windows.
func dropPrivileges(_, _ int) error {
	return nil
}
