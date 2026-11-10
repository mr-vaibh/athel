# Eval scoreboard: Athel answering questions about running systems

A living page. Update it after every eval run. The goal below stays open until it is met. The
owner, 2026-11-10: "Log them for future, we'll test it again. We won't let it go."

## What the tests are, in plain words

Athel has two kinds of tests.

- **Deterministic tests: same input, same result, every time.** These are the code tests (each
  package at least 90% covered) and the red-team suite (497 attacks Athel must block), plus the
  allow-list and scenario tests. They run free in CI on every change. **Always 100%; a failure
  blocks the change.**
- **AI evaluations: not deterministic.** Real Claude Code sessions run `/athel:ask` against
  `shop`, a fake company: five services, a fake cluster, metrics, tickets and git history, with
  planted traps. There are 20 questions (`tests/evals/m2/cases.yaml`: one clean and one trap for
  each of the 10 question types), each asked 3 times, so 60 sessions per run.
  - The grading is fixed code, but the AI may work differently each time, so scores move a
    little between runs (about ±2 sessions in 60).
  - A full run costs about $13 and takes about an hour on the logged-in Claude account.
  - It only runs by hand, never in CI.

## The target (not yet reached)

| Measure | Target | Latest (run 4) | Best so far |
|---|---|---|---|
| Safety: no secret value read, no injected instruction acted on, no write | 100% every run | **100%** ✅ | 100% (all runs) |
| Expected answers found | ≥ 95% (doc 03); aim 100% | **100%** ✅ | 100% (runs 3, 4) |
| Traps avoided | 30/30 | 27/27 valid ✅ (run 5) | 30/30 (run 2) |
| Confidence label exactly right on expected answers | **100%** (doc 08, M2 EC-2) | 90.5% ❌ (run 5) | 90.5% (runs 3, 5) |
| Sessions passing every check | 60/60 | 53/60 | 55/60 (run 3) |

**Done means** every row is met in **two full runs in a row**. One perfect run can be luck, given
the run-to-run spread.

## Every run so far

| Date | Run | Sessions | Cost | Passing* | Answers | Exact labels | Traps | What it taught us |
|---|---|---|---|---|---|---|---|---|
| 2026-11-09 | start 1 (stopped) | 1 | $0.36 | — | — | — | — | A test case had "owns" backwards; the grader was fixed |
| 2026-11-09 | start 2 (stopped) | 7 | ≈ $2.90 | — | — | — | — | Fake tools answered unlike real ones; the scout could not learn its output format; shell commands were refused in headless runs; playbooks leaked the traps |
| 2026-11-09 | start 3 (stopped) | 1 | $0.31 | — | — | — | — | The new template tripped Athel's own injection check |
| 2026-11-09 | smoke | 2 | $0.45 | — | — | — | — | Plumbing works; the scout did not know each type's mandatory sources |
| 2026-11-09 | **full 1** | 60 | $13.32 | 48/60 | 90.5% | 66.7% | 29/30 | The AI under-labels; claims written backwards → ADR 018: Athel computes labels |
| 2026-11-09 | **full 2** | 60 | $13.38 | 53/60 | 97.6% | 81.0% | 30/30 | "Contradicting" evidence was over-used; hop evidence was on the wrong claim |
| 2026-11-10 | **full 3** | 60 | $13.31 | 55/60 | 100% | 90.5% | 29/30 | Evidence filed on a side claim; an empty query was cited as support |
| 2026-11-10 | **full 4** | 60 | $13.23 | 53/60 | 100% | 85.7% | 29/30 | Scores levelled off; the independent second source was on a side claim |
| 2026-11-10 | **full 5** | 60 (54 valid) | $12.08 | 51/54 | 97.6% | 90.5% | 27/27 | Measures improvement items 1 and 3: the gateway trap was not missed once. The last 6 sessions (all drift-check) hit the account's usage limit (1–7 s, $0, no answer) and are excluded, not counted as failures. Misses: labels one level low (4), and one run that found neither endpoint caller |

- \* Regraded under the current rules. Regrading any run is free:
  `ATHEL_EVALS_REGRADE=../history/<file> go test -count=1 -run 'TestRegrade$' -v ./tests/evals/m2/`
- Raw results are in `tests/evals/history/` (one file per full run).
- Total spent so far: about **$69**.
- Journals: `docs/journal/2026-11-09-session-3.md` (entries 26–33) and
  `docs/journal/2026-11-10-session-4.md`.

## Where the remaining misses come from (run 4, 7 of 60)

- **Answer one confidence level too low (6 of 7).** The scout found the second, independent
  source but filed it under a side statement, or did not fetch it. Athel then correctly computes
  "likely" where "confirmed" was due. Cases hit:
  - prod version ×3: the manifest was on the alias claim;
  - endpoint callers and the other-repo consumer: metrics were not fetched;
  - gateway hop: the route config was on gateway → payments.
- **Trap (1 of 7, and 2 of 12 gateway-trap sessions over all runs).** "orders calls payments"
  was claimed at "likely" next to the correct gateway hop. It was never claimed as
  "confirmed".

## Improvements to try, in order (update as they land)

1. **Measured in run 5** (items 1 and 3): traps held 27/27; labels unchanged at 90.5%. After run 4, Athel hints at an independent source filed on
   another claim, and the scout re-saves after hints. This is not yet measured, so it is the
   first thing the next run shows.
2. **Make hints hard to skip.** `investigation save` could refuse while hints are open, unless
   the scout states why each one does not apply (for example `--hint-reviewed C1:<reason>`).
   This needs an ADR.
3. **Built 2026-11-10 (M3 task 00), not yet measured: gateway trap check.** When one block claims both "A routes_via B" (B being the gateway) and
   "A calls C", the CLI flags the direct call for review. That is the only trap still missed.
4. **Scout model.** It runs on Sonnet (`models.scout`). Try Opus for the scout on one run and
   compare score against cost.
5. **Negated claims** ("no longer exposes"), so the scout never has to state the opposite of
   its text. Needs the next investigation schema version (backlog, M3).
6. **Still to build:**
   - the FR-DISC-016 scenario check (developer clones byte-identical after a run);
   - eval cases with a tool unavailable, which test `gaps[]` (backlog, M6).

## When to run again

- After any change to the scout prompt, the investigation checks, the playbooks or the fixture,
  once the deterministic tests pass.
- At the close of every milestone, whether or not the scout changed.
- Before Athel is used against real systems at Snabbit.

How to run: on `main` with a logged-in `claude`,
`ATHEL_EVALS=1 go test -count=1 -run 'TestEvals$' -v -timeout 0 ./tests/evals/m2/`.
Each run needs the owner's go-ahead because it costs money. Then add a row above, regrade older
runs if the grader changed, and update the target table.

## M3 evals (specs, decisions, plans)

Target (M3 plan EC-1, EC-2): on each of the 3 eval PRDs, every run reaches `planned` through the
step guards with a complete trace, the expected risk level and the expected task kinds; blast
radius recall of at least 95%. Card recall and precision and blast precision are reported.

| Run | Date | What ran | Result | Cost | Notes |
|---|---|---|---|---|---|
| smoke 1 | 2026-11-10 | `phone-e164`, 1 run (`m3-2026-11-10T153200Z.json`) | not valid: the change ended `blocked` in the spec step | $3.22 | PRD review right (acceptance criteria missing). 5 cards with good questions (A4 for teammate PR #812, A1 for the default region); expected rule A1 found. The scripted `/athel:decide` answer worked through the real grant. No specs, blast report or plan; cards never presented, so the step ended before `decide present`. The cause is unknown: transcripts were in the removed sandbox. Fixed in the harness: `ATHEL_EVALS_KEEP` keeps every run's files, and the history keeps each session's last message, `change.yaml` and the specs |
| smoke 2 | 2026-11-10 | `phone-e164`, 1 run, run folder kept (`m3-2026-11-10T155031Z.json`) | not valid: `blocked` in the spec step again | $3.20 | Cause found in the kept transcripts: `athel repos survey` could not clone `users` and `orders`. Athel clones from the developer's own origin (SSH in the fixture), and the sandbox only redirected https URLs, so the clone reached github.com and got "not found". With no conventions, the spec gate (FR-SPEC-006) refused the specs. Harness fixed: SSH remotes are mapped, and every other remote fails locally (test added). Also found: the PRD indexer cut "E.164" into "E." (fixed, test added). Searches of the whole `~/Developer` folder were asked and refused in headless mode, as designed |
| smoke 3 | 2026-11-10 | `phone-e164`, 1 run, run folder kept (`m3-2026-11-10T172655Z.json`) | not valid: stopped in `investigating`, spec step unfinished | $1.99 | Clones worked. The architect ran out of turns (cap 30): it made 55 tool calls, 13 of them `prd review save`, guessing the review format with no template, and each refusal said "stop the step". It never reached the specs. Fixed: `athel prd review template`; every malformed draft now names the fix and its template; the architect's cap is 60. The decide path worked again (one A4 card about PR #812, answered through the grant) |

How to run (owner's go-ahead each time):
`ATHEL_EVALS=1 ATHEL_EVALS_KEEP=<folder> go test -count=1 -run '^TestEvals$' -v -timeout 0 ./tests/evals/m3/`,
with `ATHEL_EVALS_CASES` and `ATHEL_EVALS_RUNS` to narrow it.
