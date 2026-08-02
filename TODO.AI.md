# TODO.AI.md

## New feature: URL shortening (`is_link`) — implemented, all tests passing

- [x] `src/model/models.go`: add `IsLink bool` field to `Paste` (and `CreateResponse` if the
      create response should echo it)
- [x] DB schema (`EnsureSchema`): idempotent `ALTER TABLE pastes ADD COLUMN is_link INTEGER
      DEFAULT 0` (or equivalent), no migration table
- [x] Create validation (`src/server/*paste*create*`, JSON API + web form): when `is_link: true`,
      validate `content` is an absolute `http://`/`https://` URL, reject anything else with
      `VALIDATION_FAILED`; `language` ignored/cleared for links
- [x] View routes: `GET /{id}` (web, root catch-all) and `GET /api/{api_version}/pastes/{id}`
      (API) — when `paste.IsLink`, respond `302 Found` with `Location: content` instead of
      rendering/returning paste body; raw route (`GET /{id}/raw`) still returns the target URL
      as plain text (no redirect) for parity with other raw endpoints
- [x] Owner-token deletion, burn-after, expiry, view-count tracking: confirm unchanged shared
      code path handles links correctly (no special-casing needed beyond create + view)
- [x] Web UI: add a "Link" mode to the create form (toggle/tab next to paste), disable
      language/syntax fields when active
- [x] CLI (`pastebin-cli`): support creating a link (flag or subcommand — decide naming)
- [x] i18n: any new user-facing strings (create-form toggle label, validation error) added to
      `en.json`/`es.json`, other locales flagged for the missing key per testing-rules.md
- [x] Swagger + GraphQL: reflect `is_link`/target-URL field and the 302 response behavior
- [x] Tests: `*_test.go` for create validation + redirect behavior; `tests/*.sh` coverage for
      the new route behavior (302 status, `Location` header, raw-endpoint parity)
- [x] `docs/api.md` (and `README.md` features list) updated once implemented

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
