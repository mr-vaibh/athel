# ADR 034: Identical failures count fix attempts; new files need create at plan time; checkers do not re-report

Status: Accepted (2026-11-23; found by build eval 6, under the owner's delegation for technical
choices)

## Context
Build eval 6 ran after ADR 033. `typed-client-errors` and `event-name-field` were valid in all
6 runs, with the planted bug filed each time. All three `saved-addresses` runs failed, each
for a different reason:
1. **Run 1:** the gates ran three times on the same commit. No fix was attempted between those
   runs, yet the CLI raised the "failed the same way three times in a row" card (FR-BUILD-010).
2. **Run 2:** task T-02 listed `main_test.go`, a file the repo did not have, without the `create`
   verb. The scope hook refused the file mid-build, and a card sent the change back to planning.
3. **Run 3:** the security checker reported the same authorization concern in rounds 1, 2 and 4.
   The approved spec's assumption accepts this risk. Each report was folded into the review as a
   new finding with a new ID, so neither the developer's dispute nor the reviewer's verification
   stuck. A dispute card and then the round limit stopped the task.

## Decision
1. **Fix attempts.** "3 identical failures in a row" counts fix attempts, meaning distinct heads.
   Runs again on the same commit count once, as the latest of them.
2. **New files need create.** `plan set` checks each literal path (not a glob) of a task without
   `create` against the base branch in Athel's cache clone, without fetching. A file the base
   does not have is a lint problem: "add create and edit". A repo with no cache clone yet is not
   checked. The architect's prompt states the rule.
3. **No re-reporting.** The security and QA prompts restate doc 06 round rule 1 for later rounds:
   check the fixes and the new diff only. Never report again a problem the review round already
   holds, whatever its status. A risk that the approved spec, one of its assumptions or an
   answered decision accepts is not a finding.

## Alternatives rejected
- **De-duplicate folded report findings by location and rule in the CLI:** line ranges move
  between rounds and the same rule covers different problems, so a heuristic would drop real
  findings.
- **Let the scope hook allow test files next to a planned file:** it widens the approved plan
  without the developer's approval.

## Consequences
- The identical-failure card means three failed attempts, as FR-BUILD-010 intends.
- A plan that names new files without `create` is refused before approval, not mid-build.
- The checkers' later rounds stay on the new work. The reviewer remains the only judge of a
  finding already raised.

## Spec references
Doc 06 FR-BUILD-010, round rules 1 and 3; doc 05 planning rules (task verbs and paths); ADR 030,
ADR 031, ADR 033.
