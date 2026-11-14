---
description: The developer waives a failing gate or a major review finding of a task, with a reason and an expiry. Only the developer may run this by typing /athel:waive CHG-NNN T-NN <gate|R1-02> until=YYYY-MM-DD 'reason'; never invoke it yourself.
argument-hint: "CHG-NNN T-NN <gate or finding> until=YYYY-MM-DD 'reason'"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:waive — the developer lets a gate or a finding through (doc 06 FR-BUILD-014)

The developer typed `/athel:waive $ARGUMENTS` themselves. Athel's prompt hook turned exactly this
waiver into a one-time grant, and named the command that records it.

Never read or edit `.athel/` files; use the `athel` CLI.

1. Run `athel --json --session ${CLAUDE_SESSION_ID} gate waive <CHG-NNN> <T-NN> <subject> --until <date> --reason '<reason>'`,
   with every value exactly as the hook's message names it. Never change the task, the
   subject, the expiry or the reason: any other waiver is denied.
2. On `E-POLICY-HUMAN-ORIGIN`, say the waiver must be typed by the developer in this session,
   and expires after 10 minutes, and stop. On `E-STATE-INVALID`, show the message and stop.
   Doc 06 never lets some gates or findings be waived: build, unit, types, format, contract,
   migration, review, ac-tests, and blocker findings. The security scan is waived only by the
   security owner.
3. Show the waiver: what it lets through, until when, and that it will be listed in the PR
   body. A waived gate counts as passing only until the end of its expiry day.
4. Do this from the main conversation only, never from a subagent.
