# ADR 027: A plan task that changes no code is done at once, and its shared actions go to the developer

Status: Accepted (2026-11-15; found by the first paid build eval trial, under the owner's
delegation for technical choices)

## Context
The first build eval trial (typed-client-errors) planned a "verify first" task, T-01:
- it had no `paths`, only `env_actions`: measure a latency in prod, check the prod build, and
  comment on a ticket;
- it covered AC-7, which T-02's code and test also covered.

The build loop assumes every task changes code. T-01's `ac-tests` gate found no test for AC-7
in its own (empty) diff, QA had nothing to test, and the change blocked.

The spec allows such a task:
- FR-PLAN-001 lets a task have "paths or resources";
- planning rule 4 lists every shared action (a ticket comment) in `env_actions`, approved with
  the plan;
- FR-PLAN-002 makes every task cover at least one criterion.

Doc 06's build loop does not say how such a task is built, and doc 06's quality standard says
every requirement has a test.

## Decision
- **The plan lint** adds a rule: every acceptance criterion must be covered by at least one task
  with `paths`, which holds its test. A task with no paths may still cover a criterion, so
  FR-PLAN-002 holds, but never alone.
- **`build done` for a task with no `paths`** needs only that no card blocks it: it has no gates,
  review or QA and security reports, since there is no code to check.
- **`/athel:build` marks such a task done at once.**
- **Athel never performs the task's `env_actions` in M4.** They appear in every PR body under
  "Shared actions for the developer", with all the plan's other `env_actions` for that repo. This
  keeps to "humans deploy and act on environments", and the developer approved them with the
  plan.
- **The architect's prompt says the same**, and the lint message names the fix.

## Alternatives rejected
- **Refuse tasks without paths.** That contradicts FR-PLAN-001 and planning rule 4, and loses
  real pre-rollout checks the developer should see.
- **Run the env actions from the build loop.** Shared writes need the tools policy, approval
  and the right MCP tools. That is more than M4's scope and is in the backlog. Read-only checks
  belong to M6's verify step.
- **Skip only the `ac-tests` gate for such tasks.** QA's report would still be impossible, and
  the task would still block.

## Consequences
- Pre-rollout checks are visible to the human in the PR instead of blocking the build.
- A plan whose only coverage of a criterion is a non-code task is refused at plan time, before
  approval.
- `TestLintTaskWithoutCode`, the new lint row and `TestTaskWithoutCodeIsDoneAtOnce` cover this.

## Spec references
- Doc 05: FR-PLAN-001 and 002, planning rule 4.
- Doc 06: §The per-task build loop and §Code quality standard (rule 1).
- Doc 04: the PR body (a new section).
