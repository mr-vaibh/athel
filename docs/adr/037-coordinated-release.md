# ADR 037: The coordinated-release card

Status: Accepted (2026-11-24; M5 task 04, under the owner's delegation for technical choices)

## Context
Doc 04 ordering rule 5: "If backward compatibility is impossible, the plan says so in a decision
card with a coordinated-release proposal; Athel never silently ships a breaking change." Task 03
made `plan set` refuse a breaking change with consumers in the change set unless the plan has an
expand path or a coordinated release. Nothing yet could raise that card, and nothing read its
answer.

## Decision
- **`athel set coordinate CHG --reason "<why no expand path>"`** raises the card. The CLI, not
  the model, names the breaking contracts and their consumers in the set from the planned blast
  radius; with none, it refuses ("nothing to coordinate"). The card:
  - rule A2 (irreversible), step `plan`, blocks the plan;
  - question "Backward compatibility is not possible for <contracts>. How should Athel ship it?";
  - (a) coordinated release: merge every PR together, in the stated order, in one deploy window;
    in-between states unsupported, each PR carries its rollback, Athel rehearses the final state;
  - (b) re-plan with an expand path;
  - recommends (a), as doc 04 asks ("a coordinated-release proposal"), with the architect's
    reason; the safe default if unanswered is (b).
  It is raised once: a second call while it is open returns the same card.
- **The answer is read by the CLI** (the card is found by its question, as the size card is):
  with (a), the plan lint accepts the breaking change without an expand path, and plan approval
  derives the change set with strategy `coordinated` (a `TransitionRequest` field, read only for
  `plan_approved`). `set show` and `plan summary` preview the same.
- PR wording for a coordinated set and the final-state rehearsal land with linked PRs (task 07)
  and the rehearsal (task 08, ADR 036).

## Alternatives rejected
- **The architect writes the card by hand:** the CLI could not tell it from any other card, and
  the list of breaking contracts would be the model's.
- **A plan field `strategy: coordinated`:** the plan would grant itself the exception; rule 5
  wants a human decision, and cards are how Athel asks.

## Consequences
- A breaking change ships either the safe way or by an explicit human choice that is recorded,
  cited in every PR, and part of the approved change set.

## Spec references
Doc 04 §Ordering rules, rule 5; doc 05 §Decision cards (A2); M5-plan G9, §5.6.
