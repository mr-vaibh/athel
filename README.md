# Athel

Spec-driven agentic development framework, shipped as a Claude Code plugin (`plugin/`) with a Go
helper CLI named `athel` (`cli/`). The specification is in `docs/specs/` (8 PDFs, source of
truth).

**Status:** M0 Skeleton and M1 Guardrails are done; M2 Understand is next. See
`docs/plans/ROADMAP.md`.

**Continuing the work (new machine or new session):** start with `docs/plans/HANDOFF.md`. It
has the setup steps and the exact prompt to give Claude Code.

| What | Where |
|---|---|
| Rules for every session | `CLAUDE.md` |
| Background, owner decisions | `docs/context/` |
| Plans, reports, handoff, backlog | `docs/plans/` |
| Architecture decisions | `docs/adr/` |
| Session journals | `docs/journal/` |

Build and test: `source scripts/env.sh && scripts/check.sh` (details in
`docs/context/environment.md`).
