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
- [x] Wire GUI mode into `main.go` (`detectMode()`, `guiAvailable()`,
  dispatch to `gui.LaunchGUI`) — done 2026-09-27. `gui.LaunchGUI` and
  `gui.IsGUIAvailable` now have callers. `detectMode` returns a new `"gui"`
  mode; `main` dispatches it to `runGUI` and reports `gui_unavailable` when
  no display exists. Availability is build-tag split so `main.go` itself
  carries no tags: `gui_supported.go` (tag `gui`, not the three BSDs) calls
  `gui.IsGUIAvailable`; `gui_unsupported.go` (no tag, or those BSDs) is a
  constant `false`, so an untagged release build never selects GUI.
  `run_gui.go`/`run_gui_stub.go` split the same way.
  Verified in Docker: build, vet, and tests pass with and without `-tags gui`;
  `./src/client` cross-compiles for 9 platform pairs × both tag configs.
- [x] Reconcile AI.md's two conflicting PART 32 rules for what `display.mode:
  auto` should select — AI.md 43890 ("Detect: GUI if display, TUI if
  terminal, error if neither") contradicts 43906 ("Interactive terminal +
  config flags only → TUI mode"). Resolved with the project owner on
  2026-09-27 in favour of the literal modes table: **`auto` prefers GUI
  whenever a display is present.** Note this changes the no-arg default for
  desktop users with a `-tags gui` binary — they now get the GUI window
  instead of the TUI; an untagged release build is unaffected (GUI is never
  available there, so it falls through to the existing TUI/plain logic).
  A command argument always beats both display modes, so
  `pastebin-cli list` still prints a list even with `display.mode: gui`.
- [x] Record the `auto`-prefers-GUI decision as a SPEC.md override
  alongside the `gogpu/ui` override, since AI.md 43906 still disagrees
  — added "PART 32 — `display.mode: auto` resolution" to `SPEC.md`
- [x] Run `make test` + `go-lint` agent, write `COMMIT_MESS`, commit

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

- [x] Fix `src/task/disk_unix.go` and `src/server/disk_unix.go` to build on
  netbsd/openbsd — already done via per-OS variants (verified 2026-09-27,
  this item was stale). `src/{task,server}/disk_netbsd.go` use
  `unix.Statvfs`/`Statvfs_t` (NetBSD's real API; its `Statfs_t` is an opaque
  `[0]byte` placeholder) and `src/{task,server}/disk_openbsd.go` use the
  `F_`-prefixed `Statfs_t` fields OpenBSD exposes, clamping signed
  `F_bavail` at zero. `disk_unix.go` in both packages is now tagged
  `//go:build !windows && !netbsd && !openbsd`. Confirmed: `go build ./...`
  (all packages, not just `src/client`) succeeds for linux/darwin/windows/
  freebsd/netbsd/openbsd on amd64+arm64, with and without `-tags gui`
  (18/18 combinations) — **except `netbsd/arm64`**, see the item below; the
  "18/18" figure above was measured only on the `disk_*.go` files and is wrong
  for a full `./src` build.

## `GOOS=netbsd GOARCH=arm64` cannot build the server at all (upstream gap)

Discovered 2026-09-27 while re-verifying the cross-compile matrix. A full
`go build ./src` (not just `go build ./...` over the `disk_*.go` packages) fails
on **netbsd/arm64** only; every other target in the matrix is clean —
linux/386, linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64,
windows/arm64, freebsd/amd64, freebsd/arm64, netbsd/amd64, openbsd/amd64,
openbsd/arm64 all build `./src` and `./src/client` successfully. The verbatim
error:

```
package github.com/apimgr/pastebin/src
	imports github.com/apimgr/pastebin/src/database
	imports modernc.org/sqlite
	imports modernc.org/libc
	imports modernc.org/libc/errno: build constraints exclude all Go files in
	  /usr/local/share/go/pkg/mod/modernc.org/libc@v1.41.0/errno
```

This is **not** our code and not the `gui` build tag: the import chain is
`src → src/database → modernc.org/sqlite → modernc.org/libc`, and
`modernc.org/libc` ships no `errno` files for `netbsd/arm64` at all. Note that
`./src/client` *does* build for netbsd/arm64 — it just doesn't link the database
driver, which is why the earlier `disk_*.go`-only verification missed it.

Impact today: **none shipped**. `Makefile` `PLATFORMS` (line 41) is
`linux/{amd64,arm64},darwin/{amd64,arm64},windows/{amd64,arm64},
freebsd/{amd64,arm64}` — netbsd and openbsd are not release targets, so
`make build`/`make release` never hit this. It only bites someone deliberately
cross-compiling for netbsd/arm64.

- [x] Document NetBSD/arm64 as unsupported in `README.md` because the upstream
  `modernc.org/libc` dependency provides no NetBSD/arm64 `errno` implementation;
  no local replace/fork workaround was added (2026-10-04).

## E2E Tier 2 / Tier 3 are stubbed out (PART 28 "Browser E2E Testing")

AI.md 38812-38884 mandates **three** E2E tiers behind the `e2e` build tag in
`tests/e2e/`, driven by `github.com/chromedp/chromedp` over CDP: Tier 1 (SSR,
plain `net/http`), Tier 2 (No-JS browser, `emulation.SetScriptExecutionDisabled`),
Tier 3 (full browser, zero console errors). All three are REQUIRED — the spec is
explicit that a feature passing only Tier 3 is a PART 14 progressive-enhancement
violation.

Only Tier 1 is implemented. `TestTier2NoJS` and `TestTier3FullBrowser` in
`tests/e2e/e2e_test.go` are `t.Skip` stubs, so the suite silently reports
"all green" while two of the three mandatory tiers never run. The file's own
header comment says the cause was that chromedp could not be added to the shared
`go.mod` "while many other automated passes [were] concurrently working in this
repository", and that it "cannot be verified here (no Go toolchain on the
host)".

Both stated blockers are now false: there is no concurrent pass in this repo,
and verification is possible via the `casjaysdev/go` container the rest of this
file already uses. Confirmed 2026-09-27 that `go mod download
github.com/chromedp/chromedp@latest` succeeds through the default
`GOPROXY=https://proxy.golang.org,direct` with rc=0, so the dependency is
fetchable. The `go get` itself was then refused by the harness permission
classifier ("Untrusted Code Integration" — adding a new third-party dependency),
so it needs an explicit human go-ahead before it can land.

- [ ] Add `github.com/chromedp/chromedp` to `go.mod` and implement
  `TestTier2NoJS` + `TestTier3FullBrowser` in `tests/e2e/` (per AI.md
  38821-38824 for the Tier 2 `emulation.SetScriptExecutionDisabled` pattern).
  Blocked on: permission to add the third-party dependency. `tests/e2e.sh`
  already wraps `go test -tags e2e ./tests/e2e/...` in Docker and needs no
  change; per AI.md 38812 the six-target Makefile set is fixed, so no new
  target.
