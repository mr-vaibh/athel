---
description: The developer chooses what happens to each side finding of a change - a separate fix PR, a ticket, or ignore. Only the developer may run this by typing /athel:triage CHG-NNN SF-1=fix_pr SF-2=ticket … (or all=<choice>); never invoke it yourself.
argument-hint: "CHG-NNN SF-1=fix_pr|ticket|ignore … | all=fix_pr|ticket|ignore"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:triage — the developer decides side findings (doc 07 FR-FIND-003)

The developer typed `/athel:triage $ARGUMENTS` themselves. Athel's prompt hook turned exactly these
choices into a one-time grant, and named the command that records them.

Never read or edit `.athel/` files; use the `athel` CLI.

1. Run `athel --json --session ${CLAUDE_SESSION_ID} finding triage <CHG-NNN> <choices>`, with the
   change and choices exactly as the hook's message names them. Never change, add or drop a
   choice: any other set is denied.
2. On `E-POLICY-HUMAN-ORIGIN`, say the choices must be typed by the developer in this session,
   and expire after 10 minutes, and stop. On `E-STATE-NOT-FOUND`, show which finding is unknown
   and stop.
3. Show the result: each finding with its choice. For each `fix_pr`, say it becomes its own small
   change with its own plan, gates, review and PR, linked to this one. For each `ticket`, say
   Athel drafts the ticket for the developer to approve; it is never created without approval.
   Findings left undecided appear in the PR body as "not decided yet".
4. Do this from the main conversation only, never from a subagent.
