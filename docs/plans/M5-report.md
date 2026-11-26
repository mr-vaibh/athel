# M5 Multi-service: report

Date: 2026-11-25, updated 2026-11-26 (session 5). Plan: `docs/plans/M5-plan.md`, approved 2026-11-23. Environment:
macOS, Apple M3 Pro, Go 1.27.3, golangci-lint 2.15.0, Node 26.8.1, Claude Code 2.1.311.

**Verdict: not yet accepted. EC-1 is not met: full eval run 4 had 5 of 9 runs valid
(`rename-now` 3 of 3, `users-v2-profile` 2 of 3, `phone-e164` 0 of 3). ADR 039's fixes all held.
Every failure is explained, and all four came from three new Athel bugs, now fixed with tests
(ADR 040). EC-2 is met. The owner decides on the next run (§7).**

> **What M5 delivers, in plain words.** A change across several services is now one piece of
> work. Athel works out which service must ship first, refuses plans that would break
> production between merges, lets the developer agent of a consuming service see the exact new
> API the providing service built, opens linked PRs that each say where they stand in the merge
> order, **rehearses the merges in order on throwaway copies and checks the running system after
> each step** before any PR is marked ready, watches the real merges and raises an alarm if
> someone merges out of order, and drafts the later removal of the old API when the developer
> says every consumer is deployed. When safe ordering is impossible, it asks for a coordinated
> release instead of shipping a breaking change silently.

## 1. Exit criteria

| # | Criterion | Proof | Result |
|---|---|---|---|
| EC-1 (doc 08) | A 3-service fixture change merges in order with every intermediate state compatible | `tests/evals/m5/set`: 3 PRDs (a 3-service v2 profile with a deferred removal; an E.164 phone field with a migration; a rename with no transition, a coordinated release) through real sessions; the grader re-runs every branch's gates, checks the linked PRs and the rehearsal, then merges in the stated order and runs the system check from main after each merge. Deterministically, `TestThreeRepoJourney` does the same with no model, in every `scripts/check.sh` and CI run | **Not met.** Full run 4: 5 of 9 valid (`rename-now` 3 of 3; `users-v2-profile` 2 of 3; `phone-e164` 0 of 3), every failure explained (§3), all fixed (ADR 040). Full run 3: 3 of 9. `rename-now` is valid 6 of 6 across runs 3 and 4. Smoke run 2: valid. `TestThreeRepoJourney`: passes locally and in CI |
| EC-2 (owner) | Multi-service work costs about the same per repo as one-repo work | Median cost and time per changed repo ≤ 1.25 × M4's one-repo baseline ($7.69, 20 min); no question asked twice | **Met on cost and time**: full run 4 median $6.85 and 16.1 min per repo over 9 runs; full run 3 $6.28 / 14.0 min. Repeated questions: in run 3 the PR-size card per repo (fixed, ADR 039, and asked once in run 4); in run 4 a round-limit card asked again after a "stop" answer, twice (fixed, ADR 040) |
| Carried | M0–M4 gates | `scripts/check.sh`, red-team and allow-list suites, scenarios, hook latency | Green on every PR #114–#134; lint 0 issues; every package ≥ 90% (`cli` 90.3%) |

## 2. Eval runs

Details in `docs/plans/EVALS.md` (M5 section); raw results in `tests/evals/history/m5-set-*.json`.

| Run | Result | Cost |
|---|---|---|
| Smoke 1 (`users-v2-profile`) | not valid: an Athel bug (test artifact held a baseline), fixed in #131 | $15.57 |
| Smoke 2 (`users-v2-profile`) | **valid** | $18.50 |
| Full 1 (3 × 3) | 7 runs lost to the network and the usage limit; 2 found the never-cloned repo bug (fixed, ADR 038) and the TypeScript gap | $32.91 |
| Full 2 (3 × 3) | void: the account's session limit | $4.62 |
| Full 3 (3 × 3) | 3 of 9 valid: `rename-now` 3 of 3; 4 runs hit Athel bugs (fixed, ADR 039), 1 lost to the network, 1 missed the planted bug | $108.84 |
| Full 4 (3 × 3) | 5 of 9 valid: `rename-now` 3 of 3, `users-v2-profile` 2 of 3; 4 runs hit Athel bugs (fixed, ADR 040) | $146.50 |
| **Total so far** | | **$326.94** of about $550 approved |

## 3. What the trials found, and what was done

| Found by | Problem | Fix (tested) |
|---|---|---|
| Building task 02 | A deferred task was offered by `build next`, held the build open, and its paths and env actions matched rule 7 | Deferred tasks allow nothing in the change (ADR 035) |
| Building task 07 | A repo with only no-code tasks has no PR and would have held "ready together" forever | Left out of the set; dependencies walk through it |
| Testing task 08 | Upstream patch files (task 06) were not files the CLI may write | Allowed; tested through the real store |
| Building task 11 | The classifier named one-word commands with their argument and cut three-word ones, so `prd review template` counted as a write | The CLI's own names; kept in sync by a test |
| Preparing task 14 | The rehearsal could not run the fixture's system check from Athel's repo (rightly refused) | The check lives in the shop's gateway repo |
| Smoke 1 | QA left `cover.out` in a worktree; the next baseline refused it and no agent may delete it | Test artifacts ignored in Athel's worktrees; agents never write them |
| Full 1 | A repo the developer never cloned (`notifications`) could not be resolved, so the build never started | ADR 038: found under the one org |
| Full 1 | Athel cannot finish a task in a TypeScript repo (no Node coverage or security scan) | Recorded; M5.5, before any pilot (owner) |
| Full 3 | The "same failure three times" card never came: a green quick-gate run between red full-gate runs reset the count, so a stuck task could neither finish nor stop cleanly | Failures are compared within one gate stage (ADR 039) |
| Full 3 | The plan gave a criterion to a task with no test file in its paths (a migration; a system-check script), so no build round could pass | `plan set` refuses it and names the fix (ADR 039) |
| Full 3 | The plan named a single-file migration; the migration gate needs an up and a down file | `plan set` requires the pair in the same task (ADR 039) |
| Full 3 | The PR-size question was asked again for a second repo within the size already allowed | One answer covers every PR of the change (ADR 039) |
| Full 3 | The eval sandbox did not link `govulncheck` (harness, not Athel) | Found in Go's install folder too |
| Full 4 | The security gate charged a flaw in this machine's Go (1.27.3, fixed in 1.27.4) to a task that added a library, though the service had it before; its message named no module or fix, so five rounds went nowhere (`phone-e164` 1–3) | Only vulnerabilities not on base fail; each names where it is and what fixes it (ADR 040) |
| Full 4 | After the developer answered "stop the task", Athel offered the task again and asked the same question twice (`users-v2-profile` 1, `phone-e164` 1) | A "stop" holds the task until the developer answers that card again (ADR 040) |
| Full 4 | At the round limit the card recommended "stop" though every earlier finding was fixed and the last one was new and already fixed (`users-v2-profile` 1) | It recommends more rounds while the loop makes progress (ADR 040) |

## 4. What M5 delivered

PRs #114–#134. Change sets derived by the CLI at approval (`change@5`, ADR 035); plan checks for
consumers, provider ordering, breaking changes and two-way repos; the coordinated-release card
(ADR 037); every repo preflighted first and work going on past a held task (`build park`); the
provider's patch in the consumer's manifests; linked PRs with position, change label and ready
together; the merge-order rehearsal with the system check (`athel set verify`, ADR 036);
`athel set status` with the out-of-order alarm; `/athel:followup`; per-repo test locations and
JVM, .NET and Ruby test naming; command tables for the scout and the architect; the scripted
3-repo journey; the M5 eval harness.

## 5. Deviations and interpretations (all recorded)

- ADR 035 (the CLI derives the set), 036 (rehearsal; the system check in its own config file,
  not `config@3`), 037 (coordinated release), 038 (repos never cloned), 039 (failures compared
  per gate stage; plans checked for testable criteria and migration pairs; one PR-size answer
  per change), 040 (only new vulnerabilities fail the security gate; a "stop" answer holds its
  task; round-limit advice follows progress).
- `athel set derive` was not built: the set is derived at approval (plan updated).
- `changeset/status.json` was not needed: the set records each PR's last state.
- The `phone-e164` eval case has two services, not three; the 3-service exit criterion is
  `users-v2-profile`. The `rename-now` case renames `name` (Go services only) after full run 1.

## 6. Not built, or still open

- **TypeScript and Node repos (M5.5, owner):** coverage and security scan for Node.
- Several PRs per repo for one change (backlog, ADR 032); parallel worktrees (doc 08 "Later").
- The traffic check before the deferred removal (doc 07, M6).

## 7. Decision for the owner

M5 is built and the deterministic 3-repo journey proves the mechanics on every check. Each paid
run has found fewer and narrower bugs: run 4's failures came from three Athel bugs, none in the
multi-service mechanics (ordering, rehearsal, linked PRs, merges), and all are fixed with tests.
`rename-now`, the coordinated release, is valid 6 of 6. EC-1 asks for every run valid, so M5
cannot be accepted yet. About $223 of the approved $550 is left.

| Option | Cost | What it proves |
|---|---|---|
| **a. Re-run `users-v2-profile` and `phone-e164`, 2 × 3, after the fix PR merges (recommended)** | about $95–125 | The two cases that failed, on the fixed code; with `rename-now`'s 6 of 6, every case passes 3 runs in a row. Leaves about $100–130 |
| b. Full run 5, 3 × 3 | about $120–150 | The same bar as runs 1–4 on one commit; leaves about $75–100 |
| c. Accept M5 now on the evidence so far | $0 | Leaves `phone-e164` (the migration case) unproven end to end |
