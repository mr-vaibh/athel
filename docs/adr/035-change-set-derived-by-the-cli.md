# ADR 035: The change set is derived by the CLI, and deferred tasks allow nothing

Status: Accepted (2026-11-24; M5 task 02, under the owner's delegation for technical choices)

## Context
Doc 04 §Multi-repo change sets shows a `change_set` record in `change.yaml`: each repo's role,
its merge order, the PRs it depends on, and the deferred removal of the old contract. It does not
say who writes it. If a model wrote it, the merge order (the one thing that keeps every
intermediate state working) would rest on a model's reading of the plan.

While building it, two gaps from M3 and M4 showed up. A deferred task (doc 04 rule 3: the removal
is "a separate, later change") was still offered by `build next`, and `AllDone` waited for it, so
a change with a deferred removal could never finish its build. Worse, a deferred task's paths
and env actions still matched rule 7, so an agent could have edited the files the later change
removes. The M4 evals had no deferred task, so neither showed.

## Decision
- **`change@5`** types `change_set` (strategy, repos, invariant, the last rehearsal). `change@4`
  files are read and upgraded in memory; `athel migrate` rewrites them (their set was always
  empty).
- **The CLI derives the set** (`internal/changeset`) when the human approves the plan, and clears
  it when a plan change voids the approval:
  - a repo's role comes from the kinds of tasks built in it: `expand` or `migrate` makes a
    provider, `consume` a consumer, otherwise internal; a repo's deferred `contract` tasks become
    one `provider_cleanup` entry after every merge of this change;
  - a repo waits for another when one of its tasks depends on a task there, through its own
    tasks; its order is one after the latest repo it waits for. Repos with the same order may
    merge in either order;
  - two repos that wait on each other cannot be ordered with one PR each: the approval is
    refused, naming the loop and the way out (split so one side expands first, or a
    coordinated release);
  - strategy: `single` for one repo and no cleanup, `coordinated` after a coordinated-release
    card (ADR 037), otherwise `expand_contract`.
- **`athel set show`** shows the set fixed at approval, or before approval a preview derived from
  the current plan.
- **A deferred task allows nothing in this change:** `Plan.Match` and `WritesIn` skip it (rule 7
  and worktrees), the engine skips its env actions and resources, `build next` never offers it,
  `build start` refuses it, and the build is done without it. The CLI's other per-repo task lists
  (commit trailers, owned paths, preflight paths, PR notes and split) use only the built tasks.

## Alternatives rejected
- **The architect writes the set into the plan:** the order would be a model's claim that the
  CLI could only check, not compute. Deriving it costs nothing and cannot drift from the plan.
- **Derive it on every read instead of storing it:** the order the human approved could then
  change silently with a plan edit; storing it at approval ties it to the approved hash.

## Consequences
- The set is always consistent with the approved plan; a plan change voids both together.
- A plan with a deferred removal can now finish its build, and the removal's files stay
  read-only until the follow-up change (M5-plan G3).

## Spec references
Doc 04 §Multi-repo change sets (change set record, ordering rules 1–3), FR-GIT-008; doc 05
FR-PLAN-003; M5-plan G1–G3, §5.1.
