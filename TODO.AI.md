# TODO.AI.md

## Deferred lint findings (require a design decision — not auto-fixed)

- [ ] `Makefile` has a 7th target, `clean` (line 223), beyond the six core targets PART 25
      mandates (`dev`, `local`, `build`, `test`, `release`, `docker`). It's also a documented
      prerequisite of `build`/`local` (PART 25: "ALWAYS run `clean` before `build` and `local`"),
      so it can't simply be deleted — decide whether to fold its recipe inline into `build`/`local`
      or keep it as a non-`.PHONY`-listed internal prerequisite-only target.

## Robustness improvements (surfaced by audit — optional, not spec-mandated)

- [ ] Scheduler `TaskFunc` (`src/scheduler/scheduler.go`) takes no `context.Context`, so a hung
      task (e.g. a stalled GeoIP/download) has no per-execution timeout and holds its slot until
      it returns. Adding a bounded context requires changing the `TaskFunc` signature and every
      task registration.
- [ ] `notify.Dispatcher.Dispatch` (`src/notify/notify.go:93`) launches `go d.deliver(context.Background(), ...)`
      fire-and-forget with a retry ladder up to 24h. In-flight retries are not tied to a
      cancellable server-lifecycle context, so they can't be aborted promptly on shutdown
      (harmless at process exit, but a clean-shutdown improvement).
