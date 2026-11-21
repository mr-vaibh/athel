# ADR 032: The PR size card recommends raising the limit while Athel has one PR per repo

Status: Accepted (2026-11-21; the owner chose "Recommend (a) for now")

## Context
FR-GIT-009 says a big PRD "becomes several ordered PRs". M4 planned only the split proposal:
`athel pr open` counts the lines and groups the tasks into PRs under the limit. Athel opens one
branch and one PR per repo per change, so it cannot carry out such a split.

ADR 028's size card recommended (b) "Change the plan into smaller PRs" whenever the tasks would
fit into several PRs. In full build eval 3, `saved-addresses` run 1, the scripted developer took
(b). Nothing could go on: the change stayed in `building`, and no plan can give one repo two PRs.

## Decision
- The card recommends (a) "Allow up to N changed lines in this PR" in every case; its reason says
  that Athel opens one PR per repo, or that one task alone is over the limit.
- (b) becomes "Re-plan this change with a smaller scope": the change goes back to `planned`, and
  the plan moves part of the work to a follow-up change, approved again. It stays the safe
  default if the card is left unanswered.
- `/athel:build` moves an answered (b) back to `planned` and points the developer to
  `/athel:plan`.
- Several ordered PRs per repo are in the backlog for a later milestone.

## Alternatives rejected
- **Build several PRs per repo now:** outside the approved M4 plan; the owner deferred it.
- **(b) creates the follow-up change by itself:** the architect and the developer decide what
  moves; Athel does not guess a scope.

## Consequences
- An oversized PR opens once the developer allows it, which they are told is recommended.
- A developer who prefers small PRs gets them through a follow-up change.

## Spec references
Doc 04 FR-GIT-009 and the PR size rule; ADR 028.
