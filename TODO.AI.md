# TODO.AI.md

## Deferred audit findings (require a design/release decision — not auto-fixed)

- [ ] api_version is hardcoded as `v1` (`src/server/server.go:1427,1516` and literal
      `/api/v1/...` route registrations throughout `server.go`). PART 14 requires using
      `{api_version}` / `APIBasePath()` instead of a hardcoded `v1`. This is a cross-cutting
      route-tree refactor; centralize the version in one constant/helper and thread it through
      every `/api/...` registration and the endpoint-listing maps.
- [ ] `release.txt` is `0.0.9`. PART 13 requires SemVer to start at `1.0.0` (never `0.x.x`).
      Bumping to `1.0.0` is an operator release decision (signals production-ready) — confirm
      before changing.

## Robustness improvements (surfaced by audit — optional, not spec-mandated)

- [ ] Scheduler `TaskFunc` (`src/scheduler/scheduler.go`) takes no `context.Context`, so a hung
      task (e.g. a stalled GeoIP/download) has no per-execution timeout and holds its slot until
      it returns. Adding a bounded context requires changing the `TaskFunc` signature and every
      task registration.
- [ ] `notify.Dispatcher.Dispatch` (`src/notify/notify.go:93`) launches `go d.deliver(context.Background(), ...)`
      fire-and-forget with a retry ladder up to 24h. In-flight retries are not tied to a
      cancellable server-lifecycle context, so they can't be aborted promptly on shutdown
      (harmless at process exit, but a clean-shutdown improvement).
