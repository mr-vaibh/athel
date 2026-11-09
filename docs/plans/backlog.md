# Backlog (out-of-scope ideas, noted instead of built)

| Date | Idea | Why not now | Likely milestone |
|---|---|---|---|
| 2026-11-01 | Wire `PermissionRequest` / `PermissionDenied` hook events into the audit log | Not in doc 02's hook list; useful for full audit coverage | M1 |
| 2026-11-01 | Windows support (file locking via `LockFileEx`, hook command quoting) | M0 targets macOS + Linux (pending Q8) | Release packaging |
| 2026-11-01 | Multi-platform plugin packaging (one binary per OS/arch in `plugin/bin/`, launcher) | M0 builds the host binary only | Release packaging |
| 2026-11-01 | `in_review → building` transition for PR review changes | Not in spec (pending Q5) | M4 |
| 2026-11-01 | `athel migrate` for schema upgrades | Only `change@1` exists in M0 | First schema bump |
| 2026-11-01 | Network-deny sandbox test for the CLI (doc 08 Practices) | No network code in M0; worth adding once git operations land | M4 |
| 2026-11-01 | Cache compiled JSON Schemas across hook invocations (or skip config re-validation on the hot path when the config files are unchanged) | M0 hook p95 is 13 ms end to end, ~5 ms of it in-process schema compilation; well inside 50 ms, but M1 adds shell parsing and policy evaluation | M1 |
| 2026-11-01 | PowerShell command parsing for the policy engine | M1 treats PowerShell commands as unparseable (deny writes, approve reads); Athel's users run Bash on macOS/Linux | Windows support |
| 2026-11-01 | Delete the 23 merged M0 task branches on GitHub | All contained in main; owner chose to keep them (2026-11-09, also the M1 branches) | only if the owner asks |
| 2026-11-01 | Policy cache across hook processes (compiled policy keyed by file content hash) | Not needed: p95 15.9 ms with the engine (ADR 016); would add a stale-policy risk | When hook p95 exceeds 30 ms |
| 2026-11-02 | Live check of `onFailure: "block"`, the literal `/athel:approve` in UserPromptSubmit and `updatedToolOutput` on Bash output, in a real Claude Code session with fixtures only | Verified from the official docs only; the borrowed machine ran live production | **Done 2026-11-09**: `tests/live/claude-behaviours.sh`, M1-report §6 |
| 2026-11-09 | Seeded local database with a 1M+ row table for the `shop` fixture (doc 08 §Fixture system, Data) | M2 questions need only schemas, migrations and row counts (M2 plan Q9, if approved) | M6 (staging DB helper) |
