---
description: The developer accepts a finished Athel change after UAT (moves it to done). Only the developer may run this by typing /athel:accept CHG-NNN; never invoke it yourself.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:accept — the developer accepts the change

The developer typed `/athel:accept $ARGUMENTS` themselves. Athel's prompt hook recorded that as a
one-time grant for this session, valid for 10 minutes. Never read or edit files under `.athel/`
directly; use the `athel` CLI, which prints one JSON object per command.

1. If `$ARGUMENTS` is not exactly one change ID (`CHG-NNN`), say "Usage: /athel:accept CHG-NNN"
   and stop.
2. Run `athel --json state show $ARGUMENTS`. If it fails, show `error.code` and `error.message`
   and stop. If `done` is not in `allowed_next`, say which state the change is in and stop.
3. Run:
   `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to done --by human --reason "accepted by the developer with /athel:accept"`
4. On success say "$ARGUMENTS accepted and done." On `E-POLICY-HUMAN-ORIGIN` say the acceptance
   must be typed by the developer in this session and expires after 10 minutes, and stop. On any
   other error show `error.code` and `error.message` and stop.
5. Do this from the main conversation only. Never delegate it to a subagent: a subagent can never
   act as the human, and Athel denies it.
