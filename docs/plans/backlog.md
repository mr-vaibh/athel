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
| 2026-11-01 | Policy cache across hook processes (compiled policy keyed by file content hash) | Not needed: p95 15.9 ms with the engine (ADR 016); would add a stale-policy risk | **Closed 2026-11-09, not built**: profiling showed the cost is filesystem calls (about 66 µs per file open on this machine), which a correct cache cannot avoid (ADR 016 follow-up) |
| 2026-11-02 | Live check of `onFailure: "block"`, the literal `/athel:approve` in UserPromptSubmit and `updatedToolOutput` on Bash output, in a real Claude Code session with fixtures only | Verified from the official docs only; the borrowed machine ran live production | **Done 2026-11-09**: `tests/live/claude-behaviours.sh`, M1-report §6 |
| 2026-11-09 | Seeded local database with a 1M+ row table for the `shop` fixture (doc 08 §Fixture system, Data) | M2 questions need only schemas, migrations and row counts (M2 plan Q9, if approved) | M6 (staging DB helper) |
| 2026-11-09 | Eval cases with a tool removed (expired login, missing CLI) that grade `gaps[]` and the login command (M2 plan §4.9 "tool subset") | Every fixture tool is reachable in the M2 cases; gap handling is unit-tested in the CLI and the scout prompt | M6 (CI eval harness) |
| 2026-11-10 | Negated claims in investigation@1 (a removed endpoint, a consumer that no longer exists) | The eval scout wrote "users exposes legacy-profile" labelled likely while its text said the endpoint was removed; the claim format has no "not" | M3 (next investigation schema version, with a migration) |
| 2026-11-10 | Confidence-label accuracy below the 100% EC-2 bar (85.7–90.5% on expected claims); measure the independent-source hint and re-save rule added after run 4 | Owner accepted M2 with this shortfall; the owner: "We won't let it go" | Tracked in `docs/plans/EVALS.md` until met; first improvement early in M3 |
| 2026-11-12 | Fewer decision cards: card precision was 18–20% over the two M3 full runs (5–9 cards per PRD against 1–2 required), mostly A2 cards a senior engineer would decide and record alone | Reported, not gated, in M3 (M3 plan EC); tightening the ask rules needs its own measured run | M4 (measure with the next paid eval) |
| 2026-11-14 | Symbol-level overlap in git preflight (doc 04 step 4, "when the language tooling allows") | M4 task 02 checks overlap at file level, which is stricter (more cards) but never misses an overlap | M5 or later, with language tooling |
