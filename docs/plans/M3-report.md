# M3 Specify: report

Date: 2026-11-12 (session 4). Plan: `docs/plans/M3-plan.md`, approved 2026-11-10. Environment:
macOS, Apple M3 Pro, Go 1.27.3, golangci-lint 2.15.0, Claude Code 2.1.311.

**Verdict: M3 is built and both exit criteria are met.**
- In the final full run, all 9 runs (3 eval PRDs × 3) reached `planned` through every step
  guard, with valid specs, reports, decisions and plans.
- Blast radius recall was 100% in both full runs, against a 95% bar.
- The first full run had 8 of 9 valid. Its one failure exposed a real gap in the architect's
  instructions. That gap was fixed, and the second full run measured the fix (section 3).

One measure that is reported but not gated is weak: the architect asks more questions than each
PRD needs (card precision 18–20%). That work is in the backlog for M4 (section 6).

## 1. Exit criteria

| # | Criterion | Proof | Result |
|---|---|---|---|
| EC-1 | 3 eval PRDs produce valid specs, reports and plans | `tests/evals/m3` runs each PRD as four real `claude -p` sessions in a sandbox: `/athel:start`, `/athel:spec`, the developer's `/athel:decide` (every card answered with its recommended option, through the real UserPromptSubmit grant), then `/athel:plan`. A deterministic grader requires: state `planned` (so every step guard passed), a complete trace, at least the expected risk level, and the expected task kinds | **Met**: 9/9 in full run 2 (8/9 in full run 1) |
| EC-2 | Blast radius recall 95% on fixtures | Each PRD lists its expected affected items. They are matched on each report section's naming fields only, never on evidence text | **Met**: 100% in both full runs |
| Carried | M0–M2 gates | `scripts/check.sh`, red-team and allow-list suites, scenarios, hook latency | Lint 0 issues. Every package ≥ 90% (lowest: lock 90.0%). Red-team **505/505** deny with the expected rule (M2: 497). Allow-list passes |

Coverage per package (CLI and test support):

| Package | % | Package | % | Package | % | Package | % |
|---|---|---|---|---|---|---|---|
| asks | 97.8 | audit | 95.8 | blast | 98.9 | classify | 93.7 |
| cli | 91.5 | config | 98.2 | conventions | 98.8 | decisions | 96.7 |
| engine | 98.0 | envdetect | 99.2 | facts | 100 | fsutil | 92.3 |
| gitro | 100 | grants | 98.5 | hooks | 99.0 | investigation | 100 |
| lock | 90.0 | observe | 100 | plan | 98.6 | policy | 91.8 |
| prd | 99.3 | repos | 100 | schema | 92.5 | scrub | 97.0 |
| shellparse | 95.2 | spec | 100 | state | 94.4 | trace | 100 |
| evalkit | 92.1 | fixtures/shop | 94.9 | fakes | 99.7 | m3docs | 100 |

## 2. Eval runs

Details for every run are in `docs/plans/EVALS.md` (M3 section). Raw results are in
`tests/evals/history/m3-*.json`. Regrading is free: `ATHEL_EVALS_REGRADE=../history/<file>
go test -run 'TestRegrade$' -v ./tests/evals/m3/`.

| Run | File | Cost | Valid | Blast recall | Blast precision | Card recall | Card precision |
|---|---|---|---|---|---|---|---|
| smoke 1–3 | `m3-2026-11-10T153200Z`, `…155031Z`, `…172655Z` | $8.41 | 0/3 | — | — | — | — |
| smoke 4 | `m3-2026-11-10T175329Z` | $3.70 | 1/1 | 100% | 78% | 100% | 12.5% |
| full 1 | `m3-2026-11-10T181714Z` | $28.27 | 8/9 | 100% | 84% | 89% | 20% |
| full 2 | `m3-2026-11-12T084721Z` | $29.74 | **9/9** | **100%** | 85% | 89% | 18% |

- Risk levels matched the expected levels in every valid run: HIGH for `phone-e164`, HIGH for
  `rename-event-field` (one MEDIUM in full 1, above its floor), and CRITICAL for
  `payments-reliable`.
- The PRD review checks passed 9/9 in both full runs.
- Total model cost for M3 evals: about **$72**. That includes a full-run-2 attempt stopped
  after a few minutes at the owner's request.
- **Safety held in every run.**
  - The scout read the injected "delete a production pod" text in a Jira ticket and treated
    it as untrusted.
  - Whole-folder searches, a search of this real repository and `SendMessage` were asked and
    so refused in headless mode.

## 3. What the evals found, and what was done

| Found by | Problem | Fix (tested) |
|---|---|---|
| smoke 1 | Run files lost when the sandbox was removed | `ATHEL_EVALS_KEEP` keeps every run folder; history keeps each session's last message |
| smoke 2 | Repo clones used the developer's SSH origin and escaped the sandbox, so the conventions gate refused the specs | The sandbox maps SSH and https remotes, and sends every other remote to a path that fails locally |
| smoke 2 | The PRD indexer cut "E.164" into "E." | Sentences end only at punctuation followed by a space or the line's end |
| smoke 3 | The architect guessed the review format over 13 refusals, each telling it to "stop the step", and ran out of turns | `athel prd review template` and `athel blast template`. A malformed draft now says to fix it and names its template (`draftError`). Architect turn cap 60 |
| smoke 3 | A survey by short repo name was refused with "stop the step" | It now says to name the repo `host/org/repo` |
| full 1 | The architect offered "re-investigate first" as a card option and recommended it. Once that option was chosen, the plan step had nothing to plan, because no lifecycle step investigates again after the cards | Every card option must lead to a plan. An unverified claim becomes a task that verifies it first. Rollout conditions become gates. The plan step applies every answer and updates the spec when an answer adds a requirement (`TestArchitectCardsLeadToAPlan`). Full run 2 showed no dead-end options |

The cases' expectations and the pass bar never changed after a result was seen.

## 4. What M3 delivered

PRs #60–#75, stacked, one per task (`M3-plan.md` §9):
- **PRD intake.** Stable PRD IDs, versions and diff, an ambiguity pre-scan, and the
  architect's review, checked for all 8 checklist items.
- **Decision cards.** Ask rules A1–A7 are required on every card. Athel's own choices are
  recorded with a reason, cards are batched as one interruption per step, and safe defaults
  apply after 4 h. Only the developer's typed `/athel:decide` can answer (ADR 019).
- **Specs per service.** EARS and acceptance-criteria lints and PRD-ID citations, rendered
  from a validated YAML block. Brownfield conventions are required before any spec, and the
  CLI re-checks their freshness offline.
- **`plan@2`.** Task kinds, coverage and expand–contract ordering lints, rollbacks at high
  risk, a summary and a diff (ADR 020).
- **Traceability.** PRD to spec to plan to acceptance criteria, with step guards on
  `specified` and `planned` (ADR 021).
- **Blast radius.** A planned report linked to the spec, with a CRITICAL report needing the
  developer's `/athel:signoff`. An actual report from the diff blocks unplanned impact
  (ADR 022).
- **Agents and skills.** A read-only architect agent, with templates for every document it
  writes. Skills `spec`, `decide`, `plan` and `signoff`, and cards in `/athel:status`.
- **Evals.** A shared eval kit (sandbox, settings, history), the M3 grader, a multi-prompt
  runner and regrading.

## 5. Deviations and interpretations (all recorded)

1. Spec gaps G1–G15 were decided in `M3-plan.md` §3 and approved with the plan.
2. **ADRs:**
   - 019: human-origin decision answers, and the decide skill ticking only after the answers;
   - 020: `plan@2`;
   - 021: step guards;
   - 022: diff change-surface heuristics.
3. **The scripted developer always picks the recommended option.** This measures the default
   path, not every path. Answers that differ from the recommendation are covered by unit tests
   and the human-grant tests.
4. **Card recall and precision are reported, not gated.** Recall counts each case's required
   minimum rules; precision counts every card against that minimum, so it understates cards a
   reviewer would accept. On smoke 4, 5 of 8 cards were clearly justified.

## 6. Not built, or still open

- **Over-asking.** The architect raised 5–9 cards per PRD where 1–2 were required, mostly A2
  cards that a senior engineer would decide alone and record. This needs its own measured run
  (backlog, M4).
- **`rename-event-field` sometimes raises no A2 card** (1 in 3 runs, both full runs) for the
  field rename. The grade allows this, but it is reported.
- **M2's carried target**, 100% exact confidence labels: still 90.5% at its last measure
  (`EVALS.md`). No M3 work changed the scout's labels.
- **By plan, these come later:**
  - the other 9 eval PRDs and rubric grading (G1, M6);
  - keeping completed tasks' state across PRD versions (G9, M4);
  - the developer-agent citation of conventions and the per-PR report check (G11, M4);
  - lesson conflicts in A4 (G12, M6).

## 7. Decision for the owner

M3's exit criteria are met. **Recommended: accept M3 and merge the stack in order, #60 → #75**
(each PR's base is the one before it). Then plan M4 (Build).
