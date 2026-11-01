# ADR 011: State model interpretations made in M0

Status: Accepted (2026-11-01; questions Q1–Q6, Q10 answered by the owner in chat, see
`docs/context/decisions-from-chat.md`)

## Context
Doc 02 defines the lifecycle, steps, locks and budgets, but leaves some mechanics open. M0 had
to fix them in code. None of these deviates from a spec statement; each fills a gap.

## Decisions
1. **Steps are decoupled from lifecycle transitions (Q1).** `athel state enter <step>` checks
   that the step may run in the current state (fixed step→states table in
   `cli/internal/state/steps.go`), takes the change lock, increments `step_attempts` and records
   the start. `athel state exit <step> --result ok|blocked|failed` records the end. Only
   `athel state transition` moves the lifecycle, with one exception: a step that ends `blocked`
   moves the change to `blocked` (doc 02 execution rule 4 + "blocked … returns to the state it
   came from"). Rationale: doc 02's steps and states do not map one-to-one ("specified" is the
   result of the spec step, "building" is where several steps run), and a mapping invented now
   for twelve skills would be hard to undo.
2. **Approval voided returns to `planned` from `plan_approved`, `building` and `ready_for_pr`
   (Q2)**, because docs 05 and 06 raise plan changes during the build and before the PR.
3. **Heartbeat throttling (Q3).** "Every tool call refreshes [the lock]" is implemented as: every
   tool call may refresh it, but `change.yaml` is rewritten only when the last heartbeat is older
   than 5 minutes. With a 30-minute TTL an active session's lock can never go stale.
4. **`draft` is a working state (Q4)** and can be blocked.
5. **No `in_review → building` (Q5)** and **no skip of `awaiting_decisions` (Q6)** in M0.
6. **Every mutating command needs a session ID** and claims the change lock for it; a second
   live session gets `E-STATE-LOCK-HELD` and read-only access. Reaching `done`/`cancelled`
   releases the lock.
7. **`E-INTEGRITY` state-file hashing is deferred to M1 (Q10)**, together with the hash-chained
   audit. M0 already hashes evidence files per transition.
8. **Human-only transitions** need `--by human` in M0. Each history entry records
   `origin: {session, prompt_id}` and the UserPromptSubmit hook logs prompt IDs (never prompt
   text), so M1's hook layer can reject `--by human` that did not come from a human prompt.
9. Reserved `change.yaml` fields (`waivers`, `repos`, `change_set`, `parent`, `flags`,
   `approvals.plan`) must be empty in `athel/change@1`; the milestone that fills them bumps the
   schema version with a migration.

## Consequences
Later milestones extend the step table and transition table only through ADRs and tests.

## Spec references
doc 02 §Runtime model (execution rules), §State model, §Workflow engine; doc 05 FR-PLAN-004;
doc 06 §The per-task build loop.
