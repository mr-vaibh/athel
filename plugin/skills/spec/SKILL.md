---
description: The spec step of an Athel change - investigate, review the PRD, onboard the repos, raise decision cards, write a spec per service and the planned blast radius, before any code (doc 05). Use for /athel:spec CHG-NNN, or when a change is in draft or investigating and the next step is the spec.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*), Agent, Read
---

# /athel:spec — from PRD to specs (doc 05)

Never read or edit `.athel/` files except the ones named here; use the `athel` CLI, which
prints one JSON object per command.

1. If `$ARGUMENTS` is not one change ID, say "Usage: /athel:spec CHG-NNN" and stop.
2. Run `athel --json state show $ARGUMENTS`. From `draft`, investigate first (doc 03):
   - `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to investigating --by athel --reason "investigation starts" --evidence prd/v1.md`;
   - `athel --json --session ${CLAUDE_SESSION_ID} state enter $ARGUMENTS investigate`;
   - list the questions about the running system the PRD depends on (who owns the data, who
     calls or consumes what will change, what is deployed, health, owners, active work). For
     each, delegate to the `athel:scout` subagent with the question and `--change $ARGUMENTS`;
   - `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS investigate --result ok`.

   The change must now be in `investigating` or `specified`.
3. Start the step: `athel --json --session ${CLAUDE_SESSION_ID} state enter $ARGUMENTS spec`.
4. Delegate to the `athel:architect` subagent with step `spec`, the change, the repos in scope,
   and `.athel/changes/$ARGUMENTS/investigation.md`. Never write the review, cards, specs or
   report yourself.
5. When it returns, unless the change is already `specified`, run
   `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to specified --by athel --reason "specs saved"`.
   If the step guard refuses, hand its message to the architect once. If it still refuses, end
   the step with 
   `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS spec --result blocked --reason "<the guard's message>"`,
   show what is missing, and stop.
6. Run `athel --json --session ${CLAUDE_SESSION_ID} decide present $ARGUMENTS --step spec`, then
   end the step with `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS spec --result ok`
   and move on:
   `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to awaiting_decisions --by athel --reason "cards presented"`.
7. Show every presented card at once: its question, the options with their consequences, the
   recommendation and the default. Give the risk level from the blast radius.
8. End with what the developer types next. With cards: one line they can copy, for example
   `/athel:decide $ARGUMENTS D-1=a D-2=b` (`skip=D-n` means "should not have asked"). Without
   cards: `/athel:plan $ARGUMENTS`.
