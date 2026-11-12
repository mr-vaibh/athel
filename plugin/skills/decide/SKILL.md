---
description: The developer answers Athel's decision cards. Only the developer may run this by typing /athel:decide CHG-NNN D-1=b …; never invoke it yourself.
argument-hint: "CHG-NNN D-1=<a-d> [D-2='free text'] [skip=D-3]"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:decide — the developer answers cards (ADR 019)

The developer typed `/athel:decide $ARGUMENTS` themselves. Athel's prompt hook turned exactly
these answers into a one-time grant, and named the command that records them.

Never read or edit `.athel/` files; use the `athel` CLI.

1. Run `athel --json --session ${CLAUDE_SESSION_ID} decide answer <CHG-NNN> <answers>`, with the
   change and answers exactly as the hook's message names them. Never change, add, drop or
   reword an answer: any other answer set is denied.
2. On `E-POLICY-HUMAN-ORIGIN`, say the answers must be typed by the developer in this session,
   and expire after 10 minutes, and stop. On `E-STATE-INVALID`, show the message (an unknown
   card, an option a card does not offer, a card already answered) and stop.
3. Run `athel --json --session ${CLAUDE_SESSION_ID} decide tick <CHG-NNN>` (after the answers,
   so a due default never overrides what the developer just typed), and show its reminders.
   Then run `athel --json decide list <CHG-NNN> --open`. If cards remain, show them. If none remain
   and the change is in `awaiting_decisions`, say the next step is `/athel:plan <CHG-NNN>`.
4. Do this from the main conversation only, never from a subagent.
