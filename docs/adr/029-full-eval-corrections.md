# ADR 029: Corrections from the first full M4 evals

Status: Accepted (2026-11-20; found by the full paid evals of 2026-11-19, under the owner's
delegation for technical choices)

## Context
The full build eval (3 PRDs × 3 runs) reached `in_review` in 2 of 9 runs. Reading the sessions
showed five places where Athel itself left a change with no way on:

1. **A repo whose tasks change no code.** ADR 027 lets a check or a ticket comment be a task
   with no paths. The move to `building` still asked for a git preflight in its repo, and the
   preflight refused one (no task writes there), so the change could never start. `build done`
   for such a task also looked for a worktree. Its shared actions went to its own repo's PR,
   which never exists.
2. **The round limit.** Doc 06 says a task leaves the loop "as a decision card when the round
   limit is reached". The card was raised only when a sixth round was saved, which never
   happens: the build skill stopped after round 5 with nothing for the developer to answer. The
   card's option "Allow more rounds" did nothing, and `review@1` capped a round at 5.
3. **A long dispute.** The dispute card's context joins the finding, the reviewer's note and the
   developer's reason. Past `decision@1`'s 2,000 characters the dispute failed with
   `E-STATE-SCHEMA`.
4. **A mutant that loops forever.** Each mutant's tests had only the 10-minute gate limit, and up
   to 50 mutants run. One hang held the gates past the session's time.
5. **A spec for a service a decision took out of the change.** The architect writes specs
   before the developer answers the cards. When an answer narrowed the change to one repo, the
   other repo's spec stayed. The plan could not cover its criteria with code (ADR 027), and
   nothing could remove the spec.

## Decision
1. **Repos with no code.** `plan.WritesIn` counts only tasks with paths. Preflight, worktrees and
   the `building` guard skip a repo the plan writes nothing in. `plan summary` marks each repo
   with `writes`. `build done` finishes a no-code task without a worktree. Its shared actions
   appear in every PR of the change, named with their repo.
2. **Round limit.**
   - Saving the last allowed round, when it is not done, raises the round-limit card at once.
   - The card names the open blocking findings and says that a major one can be waived
     (`/athel:waive`).
   - Each answer "Allow more rounds" (b) raises the task's limit by 5, up to 50. `review@1`
     allows rounds up to 50.
3. **Card text is clipped** to the schema's limit. The finding, the reviewer's note and the
   developer's reason each get a share of the 2,000 characters, so both sides of a dispute
   survive.
4. **Each mutant's tests get one minute** (`go test -timeout`). A mutant whose tests time out
   counts as killed, as mutation tools count a timeout: the tests noticed it.
5. **`athel spec remove CHG-NNN <service> --reason "…"`** deletes a service's spec while the
   change is `investigating`, `specified`, `awaiting_decisions` or `planned`. It never removes
   the change's last spec. The architect uses it in the plan step when an answer takes a
   service out of the change, then saves the planned blast radius again. After approval, specs
   are fixed.

The plugin's skills that delegate to agents (spec, plan, investigate, blast radius, ask) keep
them in the foreground, as `/athel:build` already does. `/athel:plan` resumes a change blocked
by its own earlier card once that card is answered.

## Alternatives rejected
- **Let a plan's criteria be covered by a no-code task again:** ADR 027 removed this for a
  reason: such a task never proves its criterion.
- **Count a hung mutant as not run:** the gate would pass a change whose tests merely hang on a
  real mutant, and the score would hide it.
- **An unbounded round count:** the fix loop exists to stop spending; 50 rounds is far past any
  useful loop.

## Consequences
- Every block in the eval's failures now ends on a card the developer can answer, or does not
  happen.
- A change still stops at the round limit when the developer chooses to stop. The eval's
  scripted developer takes the recommended "stop", so such a run stays invalid; that is the
  reviewer and the spec disagreeing, not Athel being stuck.
- `spec remove` is one more writing command. The build roles' command tables (ADR 024) do not
  list it, so no build agent can run it.

## Spec references
Doc 06 FR-BUILD-010 and §The per-task build loop, round rules 3 and 5; doc 05 §Plan; ADR 025,
ADR 027.
