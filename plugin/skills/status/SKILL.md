---
description: Show a one-screen summary of Athel changes in this workspace — state, current step, last checkpoint, budget and lock. Use when the user asks for Athel status or runs /athel:status.
argument-hint: "[CHG-NNN]"
allowed-tools: Bash(athel:*)
---

# /athel:status — where things stand

Read-only. Do not change any state. Never read or edit files under `.athel/` directly; use the
`athel` CLI, which prints one JSON object per command.

1. If `$ARGUMENTS` names a change (`CHG-NNN`), run `athel --json state show $ARGUMENTS`.
   Otherwise run `athel --json change list` and then `athel --json state show <ID>` for each
   change in the list.
2. If a command fails with `E-STATE-NOT-INITIALISED`, say "No Athel workspace here; run
   /athel:start with a PRD." and stop. Other failures: show `error.code` and `error.message`.
3. Render one screen, one block per change:
   - `CHG-NNN — <title>`: state (and `blocked_from` if blocked)
   - step: name, attempt, status (or "none")
   - last checkpoint: name and note (or "none")
   - budget: status and used/max for cost, time and tool calls
   - lock: holder session and whether it is stale (or "free")
   - next allowed states: `allowed_next`
4. Keep it short. Do not suggest actions beyond what the data says.
