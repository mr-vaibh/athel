---
description: The developer confirms the folders Athel may search for their local git clones (repo search roots). Only the developer may run this by typing /athel:roots followed by the folders; never invoke it yourself.
argument-hint: "~/Developer [~/company …]"
allowed-tools: Bash(athel:*)
disable-model-invocation: true
---

# /athel:roots — the developer confirms repo search roots

The developer typed `/athel:roots $ARGUMENTS` themselves. Athel's prompt hook recorded that as a
one-time grant for exactly these folders, valid for 10 minutes in this session. Inside confirmed
roots Athel may read the developer's clones without asking (read-only; doc 04 rule 12), so only
the developer can set them. Never read or edit files under `.athel/` or `~/.athel/` directly; use
the `athel` CLI, which prints one JSON object per command.

1. If `$ARGUMENTS` is empty, run `athel --json repos roots propose`, show the candidate folders,
   and say: "Type /athel:roots followed by the folders to confirm, for example
   /athel:roots ~/Developer". Stop.
2. Run the exact command from Athel's prompt-hook message, each folder in single quotes so the
   shell does not expand `~`, for example:
   `athel --json --session ${CLAUDE_SESSION_ID} repos roots set '~/Developer' '~/company'`
3. On success run `athel --json repos index` and report how many clones were found and whether
   the walk finished. On `E-POLICY-HUMAN-ORIGIN` say the folders must be typed by the developer
   in this session and expire after 10 minutes, and stop. On any other error show `error.code`
   and `error.message` and stop.
4. Do this from the main conversation only. Never delegate it to a subagent.
