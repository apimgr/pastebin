# TODO.AI.md

- Burn-after-read is check-then-act, not atomic: `GetPasteByID`/cache read →
  `IncrementPasteViews` → in-memory `paste.Views >= paste.BurnAfter` compare,
  each a separate step with no locking/transaction tying them together.
  Concurrent requests to the same burn-after-N paste can all pass the
  threshold check before any delete lands, serving more reads than
  configured. Sites: `src/handler/paste.go:509-510` (`GetPaste`),
  `src/handler/paste.go:559` (`recordView`-equivalent), `src/handler/
  paste.go:733` (`GetPasteForWeb`), `src/handler/compat.go:229`, `src/
  handler/compat.go:391` (Len-compat), plus the third `paste.go` site
  covered by the same pattern. Needs a real fix: either a single
  DB-level atomic `UPDATE ... SET views = views + 1 WHERE ... RETURNING`
  combined with the burn check in one statement/transaction, or a
  per-paste mutex — touches every read path and the DB layer, so it
  needs its own deliberate pass rather than a quick patch.
