---
description: The investigate step of an Athel change - the scout answers the questions the PRD raises about the running system before any spec is written. Use when a change is in draft or investigating and the next step is investigation.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*), Agent, Read
---

# Investigate step of a change (doc 03)

Never read or edit `.athel/` files except the ones named here; use the `athel` CLI.

1. Run `athel --json resume` (or `athel --json change show $ARGUMENTS`). The change must be in
   `draft` or `investigating`. If `features.discovery` is false in `athel --json config show`,
   say so and stop.
2. If it is in `draft`, move it on:
   `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to investigating --by athel --reason "investigation starts" --evidence prd/v1.md`.
3. Start the step: `athel --json --session ${CLAUDE_SESSION_ID} state enter $ARGUMENTS investigate`.
4. Read the current PRD (`.athel/changes/$ARGUMENTS/prd/`). List the questions about the running
   system it depends on: which service owns the data, who calls or consumes what will change, how
   services reach each other, what is deployed, health, owners, and active work by others. Ask
   the developer only if the PRD names no system at all.
5. For each question, in turn, delegate to the `athel:scout` subagent with the question,
   `--change $ARGUMENTS`, the services in scope and the question budget from config. After each
   one, record a checkpoint:
   `athel --json --session ${CLAUDE_SESSION_ID} checkpoint set $ARGUMENTS investigate.q<N> --note "<question>: <one-line answer>"`.
   Each scout run replaces `investigation.md`. Keep its summary in the checkpoint note and in
   your reply.
6. End the step with `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS investigate --result ok`,
   or `--result blocked` if a question could not be answered and the change depends on it.
7. Report the answers with their confidence, drift, gaps and active work, in at most 2,000 words.
