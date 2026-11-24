---
description: The developer confirms that every consumer of an Athel change set is deployed, and Athel drafts the follow-up change that removes the old contract. Only the developer may run this by typing /athel:followup CHG-NNN; never invoke it yourself.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:followup — draft the deferred cleanup (doc 04 rule 3)

The developer typed `/athel:followup $ARGUMENTS` themselves: it says every consumer of the
change set is deployed everywhere. Athel's prompt hook recorded a one-time grant.

Never read or edit `.athel/` files; use the `athel` CLI.

1. If `$ARGUMENTS` is not one change ID, say "Usage: /athel:followup CHG-NNN" and stop.
2. Run `athel --json --session ${CLAUDE_SESSION_ID} followup draft $ARGUMENTS`.
3. On success, show the drafted PRD's path and its requirements in a few lines, and say the next
   step is the developer's: `next` (`/athel:start <path>`). Remind them to check that traffic on
   the old path is zero before the removal is merged.
4. On `E-POLICY-HUMAN-ORIGIN`, say it must be typed by the developer in this session, and stop.
   When the CLI says PRs are not merged yet, show which, and that `/athel:status $ARGUMENTS`
   shows the merge order. Do this from the main conversation only, never from a subagent.
