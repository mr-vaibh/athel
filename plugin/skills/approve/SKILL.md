---
description: The developer approves the plan of an Athel change (moves it to plan_approved). Only the developer may run this by typing /athel:approve CHG-NNN; never invoke it yourself.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:approve — the developer approves the plan

The developer typed `/athel:approve $ARGUMENTS` themselves. Athel's prompt hook recorded that as
a one-time grant for this session, valid for 10 minutes. Never read or edit files under
`.athel/` directly; use the `athel` CLI, which prints one JSON object per command.

1. If `$ARGUMENTS` is not exactly one change ID (`CHG-NNN`), say "Usage: /athel:approve CHG-NNN"
   and stop.
2. Run `athel --json plan show $ARGUMENTS`. If it fails, show `error.code` and `error.message`
   and stop. Show the plan in at most ten lines: tasks (ID, repo, verbs, paths) and the plan hash.
3. Run:
   `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to plan_approved --by human --reason "plan approved by the developer with /athel:approve"`
4. On success say "Plan of $ARGUMENTS approved (hash <hash>)." On `E-POLICY-HUMAN-ORIGIN` say the
   approval must be typed by the developer in this session and expires after 10 minutes, and stop.
   On any other error show `error.code` and `error.message` and stop.
5. Do this from the main conversation only. Never delegate it to a subagent: a subagent can never
   act as the human, and Athel denies it.
