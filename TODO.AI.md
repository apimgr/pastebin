# TODO.AI.md

Open items below need a decision from the project owner before any code can be
written — both are blocked on information the spec does not supply.

## Blocked on an owner decision

- [ ] `src/client`: PART 32 requires a native GUI display mode (GTK4/Qt6 on
  Linux, Cocoa on macOS, Win32/WinUI on Windows) and forbids Electron/web
  views. Every one of those toolkits needs cgo, which contradicts the absolute
  `CGO_ENABLED=0` pure-Go rule in PART 2/PART 7. The CLI currently ships
  TUI/CLI/plain modes only. Resolving this needs an explicit owner ruling on
  which constraint yields.
