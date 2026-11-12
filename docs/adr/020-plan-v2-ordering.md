# ADR 020: plan@2 with task kinds for expand-and-contract ordering

Status: Accepted (2026-11-10; M3 task 07, under the owner's delegation for technical choices)

## Context
FR-PLAN-003 requires multi-repo plans to follow doc 04's expand-and-contract ordering, checked
by an "ordering lint". `plan@1` (M1) does not say what a task does to a contract, so no lint
can check the order. ADR 002 asks for a version bump when a schema changes.

## Decision
- **`plan@2`** is `plan@1` plus, per task:
  - a required `kind`, one of `expand` (a provider adds backward-compatible fields, endpoints
    or event versions), `migrate` (a data step), `consume` (a consumer moves to the new
    contract), `contract` (removal of the old contract) or `internal` (none of these);
  - an optional `size_reason`, required for size L;
  - an optional `deferred` flag.
- **Reading old plans.** `plan@1` stays readable with its original hash: the new fields are
  omitted when empty, so an existing approval stays bound. Every plan is written as
  `plan@2`. There is no rewrite migration, because rewriting an approved plan would change its
  hash and void the developer's approval without their knowledge.
- **When plans are linted.** Once a change has specs, `plan set` accepts only `plan@2` and runs
  every check. A change without specs (the M1 flow) still accepts `plan@1`; the state machine
  (M3 task 08) requires specs before `planned`, so this is not a way round the checks.
- **Checks** (doc 05 planning rules; FR-PLAN-002/003):
  - **coverage both ways:** every acceptance criterion is covered by a task, every task covers
    one, and none is unknown;
  - **ordering:** a `consume` task depends on an `expand` task; a `contract` task is deferred
    and comes after every `consume` and `migrate` task;
  - **size:** L needs a reason;
  - **tests:** a task that edits or creates code carries the `test` verb;
  - **no deploys:** env actions never deploy;
  - **rollback:** at HIGH or CRITICAL risk (from the planned blast report), `migrate`,
    `expand` and `contract` tasks need a rollback.

## Alternatives rejected
- **Infer kinds from paths or titles:** guesses, and a wrong guess silently passes a bad order.
- **Rewrite `plan@1` files in `athel migrate`:** voids approvals.

## Spec references
Doc 05 FR-PLAN-001…004 and §Planning rules; doc 04 §Ordering rules; ADR 002.
