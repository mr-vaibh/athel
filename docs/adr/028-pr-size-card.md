# ADR 028: A PR over the size limit raises a card for the developer

Status: Accepted (2026-11-16; found by the third paid build eval trial, under the owner's
delegation for technical choices)

## Context
FR-GIT-009 limits a PR to 400 changed lines, without generated files and lockfiles. M4-plan
said an oversized diff "blocks with a split proposal; the limit can be raised at plan approval".

In build trial 3, one task's PR was 593 lines: 89 of code and 493 of tests, most of them QA's
negative, boundary and failure cases. The split proposal was that task alone, still over 400.
`pr open` refused with only an error, so the build step ended blocked with no card. Nobody
could continue the change without editing files by hand. M3 taught the same lesson: every
block must lead to a decision the developer can make.

## Decision
- **`pr open` and `pr update` raise a card (rule A6, limits reached) when the PR is over the
  limit**, with the same `E-GATE-PR-SIZE` error. The error names the card. The card is raised
  once while it is open.
  - **a) "Allow up to N changed lines in this PR"**, where N is the PR's size plus a quarter
    for fix rounds, rounded up to 50. Athel opens the PR as it is.
  - **b) "Change the plan into smaller PRs"**, which goes back through plan approval.
- **The recommendation depends on the split:** a when no split of the plan's tasks fits under
  the limit (one task alone is over it), otherwise b. The safe default is b.
- **Once the developer answers a**, `pr open` and `pr update` accept that repo's PR up to N.
  A PR that later grows past N raises a new card.
- **`/athel:build`** treats `E-GATE-PR-SIZE` like any other card that stops the loop. When the
  card is answered it moves the change from `blocked` back to `building`, and it opens any
  missing PR before `ready_for_pr`.
- **QA's prompt** asks for the fewest cases per category, as rows of existing table tests.

## Alternatives rejected
- **Leave tests out of the size.** FR-GIT-009 excludes only generated files and lockfiles.
- **Raise the default limit.** Doc 04 sets 400, and reviewers rely on small PRs.
- **Raise only at plan approval, as the plan said.** Nobody knows the tests' size before they
  are written.

## Consequences
- An oversized PR is a decision for the developer, not a dead end. The answer is in
  `decisions.yaml` and in the PR body's decisions.
- `TestPROversizedProposesSplit` covers the card, asking once, the answer and the PR opening.
  `TestSizeCardRecommendsASplitWhenOneFits` covers the recommendation.

## Spec references
- Doc 04: FR-GIT-009, and the PR size rule.
- Doc 05: ask rule A6.
- Doc 06: §Edge cases.
- M4-plan §9, task 04.
