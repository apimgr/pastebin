# TODO.AI.md

## Robustness improvements (surfaced by audit — optional, not spec-mandated)

- [ ] Scheduler `TaskFunc` (`src/scheduler/scheduler.go`) takes no `context.Context`, so a hung
      task (e.g. a stalled GeoIP/download) has no per-execution timeout and holds its slot until
      it returns. Adding a bounded context requires changing the `TaskFunc` signature and every
      task registration.
- [ ] `notify.Dispatcher.Dispatch` (`src/notify/notify.go:93`) launches `go d.deliver(context.Background(), ...)`
      fire-and-forget with a retry ladder up to 24h. In-flight retries are not tied to a
      cancellable server-lifecycle context, so they can't be aborted promptly on shutdown
      (harmless at process exit, but a clean-shutdown improvement).
