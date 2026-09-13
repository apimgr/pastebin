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

- [ ] Rewrite `src/client/gui/gui.go`, `screens.go`, and `theme*.go` against
  the real `gogpu/ui` widget/app API (in progress)
- [ ] Regenerate `go.mod`/`go.sum` via `go mod tidy` in Docker for the new
  `gogpu/ui` dependency tree
- [ ] Verify `go build ./...` (no tag) and `go build -tags gui ./...` both
  succeed in Docker with `CGO_ENABLED=0`
- [ ] Wire GUI mode into `main.go` (`detectMode()`, `guiEnvAvailable()`,
  dispatch to `gui.LaunchGUI`) if not already wired
- [ ] Run `make test` + `go-lint` agent, write `COMMIT_MESS`, commit
