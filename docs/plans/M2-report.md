# M2 Understand: report

Date: 2026-11-10 (sessions 3–4). Plan: `docs/plans/M2-plan.md`, approved 2026-11-09 with all
recommendations Q1–Q12. Environment: macOS, Apple M3 Pro, Go 1.27.3, golangci-lint 2.15.0,
Claude Code 2.1.311.

**Verdict: M2 is built and every carried gate is green, but its two exit criteria are not fully
met.**
- Every expected answer is found (100% in the last two runs, against a 95% bar).
- A trap was missed in 1 of 30 trap runs in two of the four runs.
- Confidence labels on expected answers are exactly right 85.7–90.5% of the time, against a
  100% bar.

Section 3 explains every remaining miss. Whether M2 is accepted with this shortfall is the
owner's decision (section 7).

## 1. Exit criteria

| # | Criterion | Proof | Result |
|---|---|---|---|
| EC-1 | All 10 question types pass on fixtures with traps (claim correctness ≥ 95%, every trap handled) | `tests/evals/m2` runs 20 cases (one clean and one trap per type) × 3 runs as real `claude -p "/athel:ask …"` sessions, with the plugin and the scout, against the `shop` fixture. A deterministic grader checks the saved `investigation@1` | Claims: **100%** (42/42) in runs 3 and 4. Traps: **29/30** in runs 3 and 4 (30/30 in run 2). **Partly met** |
| EC-2 | Confidence labelling 100% correct | Every expected claim states its exact label. The grader requires an exact match. Since ADR 018 the CLI also refuses any label other than the one its evidence gives | **90.5%** (run 3), **85.7%** (run 4). **Not met.** Every saved label equals its evidence's ceiling (100%); the misses are evidence not attached to the answering claim (section 3) |
| Carried | M0 and M1 gates | `scripts/check.sh`, red-team and allow-list suites, resume and guardrails scenarios, hook latency in CI | Lint 0 issues. Every package ≥ 90% (lowest: lock 90.0%). Red-team **497/497** deny with the expected rule (M1: 431), allow-list passes. CI, including the hook p95 < 50 ms gate, is green on every PR #43–#57. Search over 10,000 cached facts takes 0.28 s (NFR-DISC-02: < 2 s) |

Coverage per package:

| Package | % | Package | % | Package | % | Package | % |
|---|---|---|---|---|---|---|---|
| asks | 97.8 | audit | 95.8 | blast | 98.7 | classify | 95.0 |
| cli | 95.3 | config | 98.2 | engine | 99.5 | envdetect | 99.2 |
| facts | 100 | fsutil | 92.3 | gitro | 100 | grants | 97.7 |
| hooks | 99.0 | investigation | 100 | lock | 90.0 | observe | 100 |
| plan | 97.6 | policy | 91.8 | repos | 100 | schema | 92.5 |
| scrub | 97.0 | shellparse | 95.2 | state | 94.8 | fixtures/shop | 96.6 |
| fakes | 99.7 | | | | | | |

## 2. Eval runs

The living scoreboard, with the open target and the next improvements, is
`docs/plans/EVALS.md`.

Every run's raw answers are kept, so any run can be graded again under the current rules for
free: `ATHEL_EVALS_REGRADE=../history/<file> go test -run 'TestRegrade$' -v ./tests/evals/m2/`.
The scores below are all regraded under the final rules.

| Run | File | Cost | Time | Runs passing | Claims found | Exact labels | Traps |
|---|---|---|---|---|---|---|---|
| 1 | `m2-2026-11-09T134454Z.json` | $13.32 | 49 min | 48/60 | 90.5% | 66.7% | 29/30 |
| 2 | `m2-2026-11-09T162052Z.json` | $13.38 | 49 min | 53/60 | 97.6% | 81.0% | 30/30 |
| 3 | `m2-2026-11-10T060740Z.json` | $13.31 | 46 min | 55/60 | 100% | 90.5% | 29/30 |
| 4 | `m2-2026-11-10T070654Z.json` | $13.23 | 51 min | 53/60 | 100% | 85.7% | 29/30 |

- Total model cost, including a 2-case smoke run and three runs stopped early for harness bugs:
  about **$57**.
- Runs 1–2 used the Snabbit-all Team seat; runs 3–4 used Snabbit-2.
- In every run no secret value appeared, and no session tried to act on an injected
  instruction. Runs 2–4 listed the injected text as suspicious 3–5 times.

## 3. What the evals found, and what was done

Plumbing found before the first valid run (each fixed with tests):
- **Fakes answered unlike real tools.** An unknown argument was silently ignored, PromQL had
  only `=`, and several commands were missing.
- **The scout could not learn its own output format.** It now uses `--help` on every command
  and the new `athel investigation template`.
- **Claude Code refuses complex shell commands in headless runs.** The prompt now asks for one
  plain command per call.
- **The plugin's examples named the fixture's own traps**, which would have handed the scout
  the answers. They now use an unrelated example domain, and
  `TestPluginNeverHintsAtTheFixture` keeps it that way.
- **The new template tripped Athel's own injection check.**
- **CI caught a sandbox bug** that only shows in a fresh checkout, where the built plugin binary
  is absent (fixed in #57). Two CI failures on #44 and #45 were GitHub HTTP 504s and passed on
  rerun.

Product changes driven by the runs:
- **ADR 018: the CLI decides confidence labels.**
  - An evidence source may be marked `supports: false`. Any contradiction makes a claim
    `unverified`.
  - Save refuses any label other than the one the evidence gives.
  - Claims written backwards are refused (for example, `produced_by` needs an event as
    subject).
- **The scout prompt** lists each question type's mandatory sources; a test keeps that table
  equal to the CLI's own.
- **Non-blocking hints** point the answering claim at evidence already filed on another claim.
- **Grading follows the spec.** Three cases asked for more than doc 03 requires (a ticket
  comment, a branch besides its PR, traffic metrics for a service path). They were corrected,
  each with its doc 03 reference.

Remaining misses in run 4 (7 of 60):
- **Label one level low on the answering claim (6).** The scout gathered the second source but
  filed it on a side claim, or did not gather it. In 3 of these 6 (prod version) the repo
  manifest sat on an alias claim; the second hint, added after run 4, targets exactly this and
  has not been measured.
- **Trap missed (1).** "orders calls payments" was claimed at `likely`, alongside the correct
  gateway hop. Over the four runs this trap was missed 2 times in 12.

## 4. What M2 delivered

| Area | Where | Requirements |
|---|---|---|
| Schemas `fact`, `fact-access`, `investigation`, `repo-index`, `services`, `ask`, `blast`; `change@3`, `config@2` | #43 | FR-DISC-013 |
| Fact cache: add, find, show, use (access log), archive | `cli/internal/facts` (#44) | FR-DISC-004, NFR-DISC-02 |
| Suspicious-content flags and scout call capture | `cli/internal/observe` (#45) | doc 04 injection layer 3 |
| Recipe verification, supersede, flags on dependent changes | #46 | FR-DISC-005 |
| Investigation checks, confidence ceilings, exact labels, direction check, hints | `cli/internal/investigation` (#47, #57, ADR 018) | FR-DISC-001/002/003/006/009/014 |
| Repo index, human-only search roots (`/athel:roots`) | `cli/internal/repos` (#48) | FR-DISC-015 |
| Developer-clone protection, read-only scout, knowledge reads only through the CLI | `engine` (#49, ADR 015 follow-up) | FR-DISC-016, NFR-DISC-05 |
| Athel's own cache clones | `repos.Cache` (#50) | FR-DISC-017 |
| Praxis read allow-list, read-only MCP registry, nine playbooks | `tools.yaml`, `plugin/skills/integrations` (#51) | FR-DISC-007/008/012 |
| Hook profile; policy cache closed as not worth building | #52, ADR 016 follow-up | NFR |
| Scout agent, `/athel:ask`, `/athel:investigate`, `ask@1`, SubagentStop check | #53 | F-04, UC-01 |
| Ad-hoc blast radius, 14 deterministic rules | `cli/internal/blast` (#54) | F-07, UC-02, FR-BLAST-004 |
| `shop` fixture: 5 services, git history, fake CLIs and MCP servers, FR-DISC-008 contract test | `tests/fixtures/shop`, `tests/contract` (#55, #56) | FR-DISC-008, doc 08 fixture system |
| Eval cases, grader, gated runner, regrade, history | `tests/evals` (#57) | EC-1, EC-2 |

## 5. Deviations and interpretations (all recorded)

1. **Developer-clone protection applies in change mode only.** In guard mode it would block
   normal use (ADR 015 follow-up).
2. **No policy cache.** Measured: file opens dominate, so a cache gains nothing (ADR 016
   follow-up; owner delegation).
3. **The CLI decides confidence labels**, and `supports` was added to `investigation@1` in
   place, since that schema is unreleased (ADR 018; owner delegation).
4. **EC-2 is graded on expected claims as written.** All-claims ceiling match is reported only
   (owner delegation, `decisions-from-chat.md`).
5. **Eval harness changes from the plan (§4.9 amended):**
   - one `cases.yaml`;
   - a Go runner instead of `run.sh`, so its setup is tested in CI;
   - injected-text listing is not required where doc 03 makes the source optional.
6. **Linear and Slack ship as templates, not built-in MCP entries.** Their tool names do not
   separate reads from writes safely (#51).

## 6. Not built, or still open

- **FR-DISC-016 scenario check** (each developer clone's `.git` checksum unchanged after a run)
  was not built. The engine rule and its red-team cases are.
- **Eval cases with a tool removed** (`gaps[]`, NFR-DISC-06) are in the backlog for M6.
- **Negated claims** ("users no longer exposes X") cannot be expressed. In backlog for M3.
- **A one-off timeout in `TestConcurrentWritersSerialised`** while the machine ran at load
  average 160–230 from unrelated work. It passed on rerun and 5/5 alone; the test was not
  changed. Watch it in CI.
- FR-DISC-018 (writes only in worktrees) is M4 by plan.

## 7. Decision for the owner

**Decided 2026-11-10: option 1, accept M2 with the measured shortfall.** The owner's answer was
"Accept it (Recommended)". Label accuracy is carried into M3 as a tracked metric (backlog).

M2's code is complete and green. Three options:
1. **Accept M2 with the measured shortfall.** Exact labels 85.7–90.5%; trap misses 1 in 30
   per run, always at `likely`, never `confirmed`. Carry label accuracy into M3 as a tracked
   metric. *Recommended*: the misses are a label too cautious on a correct answer, safety held
   in every run, and the latest fix is unmeasured but cheap to check later.
2. **One more full run first** (about $13, 1 hour) to measure the last hint.
3. **Keep working until 100%.** No end date can be promised: the scout is a language model and
   run-to-run variation is about ±2 runs in 60.
