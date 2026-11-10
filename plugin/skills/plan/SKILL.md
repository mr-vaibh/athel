---
description: The plan step of an Athel change - the architect writes the task graph, the CLI checks coverage, ordering and traceability, and the developer gets a one-screen summary to approve (doc 05). Use for /athel:plan CHG-NNN, or when a change's cards are answered.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*), Agent
---

# /athel:plan — the change plan (doc 05)

Never read or edit `.athel/` files; use the `athel` CLI.

1. If `$ARGUMENTS` is not one change ID, say "Usage: /athel:plan CHG-NNN" and stop.
2. Run `athel --json state show $ARGUMENTS`. The change must be in `awaiting_decisions` or
   `planned`; from `investigating` or `specified`, say the next step is `/athel:spec $ARGUMENTS`
   and stop.
3. Run `athel --json --session ${CLAUDE_SESSION_ID} decide tick $ARGUMENTS`. It applies due safe
   defaults and lists reminders; show the reminders. If a card that blocks the plan step is
   still open (`athel --json decide list $ARGUMENTS --open`), show it with the `/athel:decide`
   line and stop.
4. Start the step: `athel --json --session ${CLAUDE_SESSION_ID} state enter $ARGUMENTS plan`, and
   delegate to the `athel:architect` subagent with step `plan`. Never write the plan yourself.
5. When it returns, run `athel --json --session ${CLAUDE_SESSION_ID} trace $ARGUMENTS`, then,
   unless the change is already `planned`,
   `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to planned --by athel --reason "plan set"`.
   If the trace is incomplete or the guard refuses, hand the message to the architect once. If
   it still fails, end the step with
   
   `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS plan --result blocked --reason "<the message>"`, show it, and stop.
6. End the step: `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS plan --result ok`.
7. Run `athel --json plan summary $ARGUMENTS` and show it on one screen:
   - tasks per repo in order, with gates and env actions;
   - the risk level;
   - open assumptions, including defaults applied without an answer, which can still be
     reversed;
   - the estimate.
8. End with what the developer types next: `/athel:approve $ARGUMENTS`. At CRITICAL risk, first
   `/athel:signoff $ARGUMENTS`, after reading `.athel/changes/$ARGUMENTS/blast-radius.md`.
