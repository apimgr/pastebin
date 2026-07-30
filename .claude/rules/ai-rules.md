# AI Assistant Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Guess or assume a requirement, file location, default value, or intent — STOP and ASK, or search first
- Rush, skip verification, or claim "done" without reading, testing, and verifying
- Modify `AI.md` (PARTs 0-33) — it is READ-ONLY. Project-specific rule overrides go in `SPEC.md`
- Pre-load the entire `AI.md` speculatively — read only the PART(s) relevant to the current task
- Add unrequested features, "improve"/"optimize" the spec, or invent patterns not in spec
- Use plain `git commit` / `git push` — use `gitcommit --dir {dir} all` only, after writing and re-reading `.git/COMMIT_MESS`
- Let a subagent write `.git/COMMIT_MESS` or call `gitcommit` — only the parent instance commits
- Use a bare `@name` in a commit message (creates unintended GitHub mention)
- Include any assistant-authorship trailer or byline anywhere (code, comments, commits, PRs, docs) — output must read as human-authored
- Place comments inline or below code — comments always go ABOVE
- Use `SELECT *` in application code — name columns explicitly
- Read an image larger than 1000×1000 directly into context — resize first (see Large Image Handling)
- Treat a non-conforming `IDEA.md` as authoritative without running the Migration procedure
- Create report/analysis files (AUDIT.md, COMPLIANCE.md, SUMMARY.md) — fix issues directly; `AUDIT.AI.md` only for explicit audits with 5+ issues, deleted when resolved
- Jump between half-finished features — complete ONE thing fully before starting the next
- Skip any MANDATORY PART (0-33) when implementing a new project — there is no "lite" version
- Reveal internals to clients: stack traces, DB structure, internal IPs/paths, dependency versions, or the specific reason for an auth failure
- Weaken security (authn/authz, TLS, CSRF/CSP/CORS, rate limiting, input validation) to reduce friction — solve usability with better defaults/UX instead
- Run `go` (or other language toolchain) commands directly on the host — ALL builds/tests run in Docker/Incus

## CRITICAL - ALWAYS DO

- Read the relevant `AI.md` PART before implementing anything; when unsure, ask
- Verify before claiming completion: read → search existing patterns → test → verify output → confirm certainty
- Ask using numbered/lettered options when clarification is needed
- Update `IDEA.md` when features change; keep README, Swagger, GraphQL, docs in sync with code
- Use `.claude/rules/*.md` (this directory) as the fast-loading summary index; regenerate if `AI.md` changes or the directory is missing
- Check `TODO.AI.md` and `TODO.md` at start of work; mark `TODO.md` items done in place, never delete/empty it; delete resolved items from `TODO.AI.md` individually
- Every text file ends with exactly one trailing newline; no trailing whitespace
- Use tabs for Go/Makefile indentation; 2 spaces for JSON/YAML/HTML/CSS/JS/shell
- Use parameterized queries, escape all template output, require CSRF tokens on state-changing forms
- Use `curl -q -LSsf` (plus method-specific flags) for all documented/scripted curl calls
- Use `{official_site}/path` in documentation and `{fqdn}` via `BuildURL(r, ...)` in embedded/runtime code — never bare paths in code that renders links
- Trim whitespace on all text input with `strings.TrimSpace()`; reject (don't trim) leading/trailing whitespace in passwords
- Follow the Golden Rule hierarchy: Correct > Verified > Fast
- Translate all user-facing text added or modified in code (see Translation Rule); treat uncertain cases as user-facing
- Download remote images/screenshots via curl before viewing them with Read

## Key Rules Summary

**Rule file map (`.claude/rules/`, all 13 required):**

| File | PARTs |
|---|---|
| ai-rules.md | 0, 1 |
| project-rules.md | 2, 3, 4 |
| config-rules.md | 5, 6, 12 |
| binary-rules.md | 7, 8, 32 |
| backend-rules.md | 9, 10, 11, 31 |
| api-rules.md | 13, 14, 15 |
| frontend-rules.md | 16 |
| features-rules.md | 17-22 |
| service-rules.md | 23, 24 |
| makefile-rules.md | 25 |
| docker-rules.md | 26 |
| cicd-rules.md | 27 |
| testing-rules.md | 28, 29, 30 |

**File hierarchy:** `SPEC.md` > `AI.md` > global `CLAUDE.md`. `AI.md` = HOW (read-only). `IDEA.md` = WHAT (editable, must follow AI.md). If `IDEA.md` conflicts with `AI.md`, fix `IDEA.md`.

**Session init checklist:** read CLAUDE.md files → migrate stale content into IDEA.md if needed → ensure `.claude/rules/` exists and is current → ensure CLAUDE.md loader exists → check TODO.AI.md/TODO.md → commit NEVER/MUST rules to memory.

**Audit** is triggered ONLY by explicit "audit"/"check compliance"/"verify project" — it fixes issues directly (never just lists them), tracks 5+ issues in temporary `AUDIT.AI.md` (deleted on completion), and never touches TODO.AI.md for findings.

**Verification by change type** (partial — see PART 0 for full table): backend/API → `make test` + curl; CLI → build + run with `--help`/`--version`; frontend → dev server + browser; Docker → build + run + smoke test; CI/CD → run workflow, check exit status per job.

**TODO.AI.md completion ritual:** when all items done, remove them individually, then commit with title `✅ all todo items have been completed ✅` (exact) and a body summarizing completed tasks — this format is reserved for TODO.AI.md completion only.

**Naming conventions:** files `lowercase_snake.go`; public `PascalCase`; private `camelCase`; interfaces `PascalCase` + `-er`. Names must reveal intent — never bare `Mode`/`Type`/`Status`/`Config`/`Get()`/`Set()` etc.; qualify them (`AppMode`, `GetUserByID()`).

**Formatting table:** Go/Makefile = tabs; HTML/JSON/YAML/CSS/JS = 2 spaces, 120 col; shell = 2 spaces, 180 col; every file ends with exactly one `\n`.

**README section order (mandatory):** Title/Badges → About → Official Site → Features → Production → Client → Configuration → API → Other → Development (always last) → Disclaimer → License. Every badge must be a linked badge; license badge must use `img.shields.io/github/license/...` so GitHub auto-detects it.

For complete details, see AI.md PART 0, 1.
