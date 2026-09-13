# TODO.AI.md

## GUI toolkit migration (in progress)

PART 32 requires a native GUI display mode, but every toolkit AI.md names
(GTK4/Qt6/Cocoa/Win32-WinUI) as well as the two candidates tried in their
place (`gioui.org` Gio, `fyne.io/fyne/v2` Fyne) were verified via real Docker
builds (`casjaysdev/go:latest`, `CGO_ENABLED=0`) to require cgo
(`import "C"` found directly in each dependency tree), contradicting the
absolute pure-Go rule in PART 2/PART 7. The project owner ruled: adopt
`github.com/gogpu/ui` (verified cgo-free across all 4 required OS families —
see `SPEC.md`) and rewrite the GUI code against it. Recorded as an override
in `SPEC.md` since `AI.md` is read-only.

- [x] Rewrite `src/client/gui/gui.go`, `screens.go`, and `theme*.go` against
  the real `gogpu/ui` widget/app API
- [x] Regenerate `go.mod`/`go.sum` via `go mod tidy` in Docker for the new
  `gogpu/ui` dependency tree
- [x] Exclude GUI support on `freebsd`/`netbsd`/`openbsd` (`gogpu/gogpu` has
  no BSD windowing backend per the upstream AI.md PART 32 update on
  2026-09-13): build tags on `gui.go`/`screens.go`/`theme.go`/`api.go`/
  `i18n.go`/`api_test.go` now exclude those three GOOS values, and
  `display.autoDetectDisplayMode()` never selects `DisplayModeGUI` there
  (falls back to TUI, or CLI with no TTY), via `guiSupportedOS()` in
  `detect_unix.go`/`detect_windows.go`
- [x] Verify `go build ./...` (no tag) and `go build -tags gui ./...` both
  succeed in Docker with `CGO_ENABLED=0` — confirmed clean on
  linux/darwin/windows/freebsd (amd64); netbsd/openbsd fail, but on a
  pre-existing, unrelated bug (see new item below), not on anything GUI-tag
  related
- [ ] Wire GUI mode into `main.go` (`detectMode()`, `guiEnvAvailable()`,
  dispatch to `gui.LaunchGUI`) — not yet wired; `gui.LaunchGUI`/
  `gui.IsGUIAvailable` currently have no caller anywhere in `src/client/`
- [ ] Run `make test` + `go-lint` agent, write `COMMIT_MESS`, commit

## netbsd/openbsd build failure in disk_unix.go (pre-existing, unrelated to GUI work)

Discovered while Docker-verifying the GUI BSD exclusion above:
`GOOS=netbsd` and `GOOS=openbsd` both fail to build `src/task` and
`src/server` (independent of the `gui` build tag — these packages don't
import `client/gui` at all) with:

```
src/task/disk_unix.go:15:20: undefined: syscall.Statfs
src/task/disk_unix.go:18:21: st.Bsize undefined (type syscall.Statfs_t has no field or method Bsize)
src/task/disk_unix.go:21:19: st.Bavail undefined (type syscall.Statfs_t has no field or method Bavail)
src/task/disk_unix.go:21:46: st.Blocks undefined (type syscall.Statfs_t has no field or method Blocks)
src/server/disk_unix.go:18:20: undefined: syscall.Statfs
src/server/disk_unix.go:22:21: stat.Bavail undefined (type "syscall".Statfs_t has no field or method Bavail)
src/server/disk_unix.go:22:42: stat.Bsize undefined (type "syscall".Statfs_t has no field or method Bsize)
```

`disk_unix.go` (both copies) is tagged `//go:build !windows`, so it's
compiled on netbsd/openbsd too, but `golang.org/x/sys/unix`'s
`Statfs`/`Statfs_t` field names differ on NetBSD/OpenBSD from Linux/
FreeBSD/Darwin (they use `Statvfs`/`Statvfs_t` with different field names
there). `freebsd/amd64` and `linux/amd64` both build clean with the same
file, confirming this is a NetBSD/OpenBSD-specific `syscall` package gap,
not a regression from the GUI/BSD-exclusion change.

- [ ] Fix `src/task/disk_unix.go` and `src/server/disk_unix.go` to build on
  netbsd/openbsd (likely needs a netbsd/openbsd-specific variant using
  `unix.Statvfs`/`Statvfs_t`, or an `x/sys/unix` abstraction that already
  normalizes this) — required for full 8-platform build support
  (`.claude/rules/makefile-rules.md`, `.claude/rules/cicd-rules.md`)
