# Claude Loader

Project: pastebin · Org: apimgr
**AI.md is the SOURCE OF TRUTH. Read it before any code change.**

## Purpose

This file is a short loader. All spec and rules live in `AI.md` and `.claude/rules/`.
Project intent and business logic live in `IDEA.md`.

## Rule hierarchy

```
SPEC.md  >  AI.md  >  global CLAUDE.md
```

## Quick-load rule files

| Topic | File | AI.md PARTs |
|-------|------|-------------|
| AI behavior & commit workflow | `.claude/rules/ai-rules.md` | 0, 1 |
| Project structure & layout | `.claude/rules/project-rules.md` | 2, 3, 4 |
| Config & ports | `.claude/rules/config-rules.md` | 5, 6, 12 |
| Binary, PID, completions, client | `.claude/rules/binary-rules.md` | 7, 8, 32 |
| Backend, DB, tokens, Tor | `.claude/rules/backend-rules.md` | 9, 10, 11, 31 |
| API shape, content negotiation, SSL | `.claude/rules/api-rules.md` | 13, 14, 15 |
| Frontend & templates | `.claude/rules/frontend-rules.md` | 16 |
| Scheduler, GeoIP, Tor, features | `.claude/rules/features-rules.md` | 17–22 |
| Service manager (systemd/launchd) | `.claude/rules/service-rules.md` | 23, 24 |
| Makefile targets | `.claude/rules/makefile-rules.md` | 25 |
| Docker files & compose | `.claude/rules/docker-rules.md` | 26 |
| CI/CD & Renovate | `.claude/rules/cicd-rules.md` | 27 |
| Testing & coverage | `.claude/rules/testing-rules.md` | 28, 29, 30 |

## Key identifiers (FROZEN)

- `internal_name` = `pastebin`
- `internal_org` = `apimgr`
- Server binary = `pastebin` · Client binary = `pastebin-cli`

## Current project state

- Last AI.md read: 2026-06-07
- Active work: spec compliance complete through Pass 18 + coverage milestone (≥80% all packages)
- No workflow files until user explicitly permits
