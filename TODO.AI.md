## [ ] Implement --service CLI command

`--service {start,restart,stop,reload,--install,--uninstall,--disable,--help}` is currently stubbed out. Full implementation required.

Read: AI.md PART 8

## [ ] Implement --maintenance CLI command

`--maintenance {backup,restore,update,mode,setup,--help}` is currently stubbed out. Full implementation required.

Read: AI.md PART 21

## [ ] Implement --update CLI command

`--update [check|yes|branch {stable|beta|daily}]` is currently stubbed out. Full implementation required.

Read: AI.md PART 22

## [ ] Implement --daemon CLI command

`--daemon` (daemonize, detach from terminal) is currently a no-op. Full implementation required.

Read: AI.md PART 8

## [ ] Add unit tests

All packages have 0% coverage. Add unit tests for database, handler, and server packages.

Read: AI.md PART 28

## [ ] Add src/data/ embedded data files

`src/data/` directory for static JSON files embedded in binary (e.g., GeoIP data, language list for Chroma).

Read: AI.md PART 0

## [ ] Implement GeoIP support

GeoIP detection required by spec (PART 19). Currently not implemented.

Read: AI.md PART 19

## [ ] Implement Tor hidden service support

Tor is installed in the container (PART 31). Auto-enable when Tor binary found. Currently not implemented.

Read: AI.md PART 31
