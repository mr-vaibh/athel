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
| 2026-11-14 | Security-owner waivers of `security-scan` with a verified identity (for example an approval from the owner's own GitHub login) | M4 task 09 refuses them all, since a human-origin grant proves a human but not which one (M4-plan G18) | M6 or later |
| 2026-11-15 | Per-repo test-file patterns for QA's write scope (for example from a structured field in conventions) | ADR 024 uses the language defaults of the gates package, since `conventions.md` is free text | Done in M5 task 10: `athel conventions tests` (checked patterns, `tests.yaml`), and JVM, .NET and Ruby test naming in the defaults |
| 2026-11-15 | Per-command limits for the scout and the architect, like ADR 024's table for the build roles | They may run any writing `athel` command today; human-only commands are still denied by rule H | Done in M5 task 11: `engine.RoleCommands` lists theirs |
| 2026-11-15 | Perform a plan's approved `env_actions` (ticket comments, chat messages) from the build loop, under the tools policy | ADR 027: in M4 they go to the developer in the PR body | M5 or M6 |

- **Several ordered PRs per repo (FR-GIT-009 "a big PRD becomes several ordered PRs").** Athel
  opens one branch and PR per repo per change; ADR 032 recommends raising the limit meanwhile.
  Later milestone, owner's choice 2026-11-21.

- Checker scope creep: in build full 4 (`saved-addresses` 2) the security checker raised one new
  minor Unicode edge case per round after all real findings were fixed, until the 5-round card
  stopped the task. Consider limiting later rounds to verifying earlier findings plus new major ones.
| 2026-11-25 | **Gates for TypeScript and Node repos.** Athel cannot finish a task in a Node repo today: `coverage` has no Node command (it reads Go profiles only), and `security-scan` has none either while only a security owner may waive it, which the CLI refuses (G18); `format` is never waivable, so a repo without a `format:check` script blocks too. Found by M5 full run 1 (the architect rightly planned work in the TypeScript `notifications` repo). Needs: lcov coverage on changed lines (`node --test --experimental-test-coverage --test-reporter=lcov`), a dependency and secret scan for Node, and the fixture's TypeScript repos given real scripts | M4 built and evaluated Go repos only (M4-plan G1); M5 kept its evals on the Go services | Planned: M5.5, first after M5, before any pilot (owner, 2026-11-25) |
| 2026-11-26 | **Single-file migrations with an in-file down section** (goose `-- +goose Down`, dbmate `-- migrate:down`, sql-migrate `-- +migrate Down`). The migration gate and the plan check (ADR 039) accept only `NNN_name.up.sql` with `NNN_name.down.sql` | No repo Athel has met uses these tools yet; found while fixing M5 full eval 3 | When a pilot repo needs it |
| 2026-11-26 | **Keep every investigation of a change.** Each scout run replaces `investigation.md`, so a change investigated three times keeps only the last; the skills keep earlier summaries in checkpoint notes (noticed by the orchestrator in M5 full eval 3, phone-e164 run 3) | By design today, and no run failed from it | Later milestone |
