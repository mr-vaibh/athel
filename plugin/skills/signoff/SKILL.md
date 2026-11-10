---
description: The developer signs off a CRITICAL blast radius report of an Athel change. Only the developer may run this by typing /athel:signoff CHG-NNN; never invoke it yourself.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:signoff — the developer signs off the blast radius (doc 05)

The developer typed `/athel:signoff $ARGUMENTS` themselves; Athel's prompt hook recorded a
one-time grant.

Never read or edit `.athel/` files; use the `athel` CLI.

1. If `$ARGUMENTS` is not one change ID, say "Usage: /athel:signoff CHG-NNN" and stop.
2. Run `athel --json --session ${CLAUDE_SESSION_ID} blast signoff $ARGUMENTS`.
3. On success, say the report (hash shown) is signed off, and that editing it voids the sign-off.
   On `E-POLICY-HUMAN-ORIGIN`, say it must be typed by the developer in this session, and stop.
4. Do this from the main conversation only, never from a subagent.
