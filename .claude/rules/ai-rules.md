# AI Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- ❌ Guess or assume — READ THE SPEC or ASK
- ❌ Implement without reading the relevant PART first
- ❌ Modify AI.md PART content (read-only spec)
- ❌ Add features not in spec without asking
- ❌ Use "I think" or "probably" — KNOW from spec or ASK
- ❌ Ask multiple plain-text questions in separate messages — use AskUserQuestion wizard
- ❌ Use generic placeholder content ("Your app name", "Feature 1")
- ❌ Create /server/about or /server/help with placeholder text
- ❌ Leave TODO comments in code — implement fully or don't implement
- ❌ Create stub functions or "future" placeholders
- ❌ Partial implementations — every feature must be 100% complete
- ❌ "I'll come back to this later" — there is no later, do it NOW

## CRITICAL - ALWAYS DO
- ✅ Read relevant PART before implementing ANY feature
- ✅ Search AI.md before asking questions (answer is likely there)
- ✅ Follow spec EXACTLY — no "improvements" without approval
- ✅ Update IDEA.md when features change
- ✅ Keep all docs in sync with code
- ✅ When unsure, ASK — never guess or assume
- ✅ Implement features 100% complete — no stubs, no TODOs, no "future"
- ✅ ONE thing at a time — finish current task completely before starting another

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Config/backup password hash? | Argon2id (NEVER bcrypt) | PART 11 |
| Where is Dockerfile? | `docker/Dockerfile` (NEVER root) | PART 26 |
| CGO enabled? | NEVER (CGO_ENABLED=0 always) | PART 7 |
| Premium features? | NEVER (all features free) | PART 1 |
| External cron? | NEVER (built-in scheduler) | PART 18 |
| Client-side rendering? | NEVER (server-side Go templates) | PART 16 |
| Boolean parsing? | config.ParseBool() — NEVER strconv.ParseBool() | PART 5 |
| Token storage? | SHA-256 hash only — raw token NEVER persisted | PART 11 |
| Constant-time compare? | crypto/subtle for ALL token/hash checks | PART 11 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| server | Main binary `pastebin` — runs as service |
| client | CLI binary `pastebin-cli` — REQUIRED |
| Operator | Person who deploys and manages the server |

## COMPLIANCE CHECK
Before completing ANY task:
- [ ] Read relevant PART(s) in AI.md
- [ ] Implementation matches spec EXACTLY
- [ ] No guessing — all decisions from spec
- [ ] Docs updated if code changed

---
For complete details, see AI.md PART 0, 1
