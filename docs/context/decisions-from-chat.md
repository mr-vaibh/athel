# Decisions from chat (append-only)

Every decision the owner makes in chat is logged here: date, question, answer, where applied.
Never edit or delete past entries; add a new entry that supersedes an old one instead.

| Date | Question / topic | Owner's answer | Applied in |
|---|---|---|---|
| 2026-11-01 | How M0 is run | Four phases with hard stops: (1) understand and plan, no code, stop for approval; (2) scaffold PR, stop for approval; (3) build task by task, one PR each, 3–5 line update after each, continue unless told to stop; (4) prove exit criteria and write `docs/plans/M0-report.md`. Source: kickoff prompt (`KICKOFF-PROMPT.md`) | `docs/plans/M0-plan.md`, `docs/plans/HANDOFF.md` |
| 2026-11-01 | M0 scope | Only M0 (Skeleton). Policy engine, shell parsing, environment detection, secret scrubbing, hash-chained audit, scout, specs, blast radius, decision cards, plans, git ops, worktrees, subagents, gates, verification, UAT, lessons are out of scope; ask before taking even a small piece of them. Source: kickoff | `docs/plans/M0-plan.md` |
| 2026-11-01 | PreToolUse in M0 | Stub that returns ALLOW, marked loudly as a stub in code, in the hook's output reason and in the M0 plan. Source: kickoff | M0-plan §6.6 |
| 2026-11-01 | Hook logging in M0 | One line per hook invocation in `.athel/audit/hook-events.jsonl`, forward compatible with the M1 hash-chained audit. Source: kickoff | M0-plan §6.6 |
| 2026-11-01 | Plugin installation | Never install the plugin into this machine's Claude Code user settings; test project-locally only. Source: kickoff | M0-plan §3 D9 |
| 2026-11-01 | Config tests on this machine | Org and user config layers must use temporary directories in tests, never the real home folder. Source: kickoff | M0-plan §7 |
| 2026-11-01 | Human-only transitions | M0 requires `--by human`; hook-level enforcement of "human-originated" arrives in M1, and M0 data must make that possible. Source: kickoff | M0-plan §6.3 |
| 2026-11-01 | How to ask questions | In batches; each with why it matters, 2–3 options with consequences, a recommendation, and the default assumed if unanswered; only when the answer changes what is built. Source: kickoff | this file; Phase 1 report |
| 2026-11-01 | Required ADRs in M0 | At least 001 (plugin), 002 (state as files), 003 (Go CLI), 010 (worktrees), plus one ADR per dependency. Source: kickoff | M0-plan §8, §9 |
