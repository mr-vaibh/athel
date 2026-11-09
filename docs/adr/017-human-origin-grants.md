# ADR 017: Human-origin grants for plan approval and acceptance

Status: Accepted (2026-11-01; owner decision Q4, M1 PR-10)

## Context
Doc 02 §Transition rules: `plan_approved` and `done` happen only from a human prompt, never a
subagent. In M0, any caller could pass `--by human`. The model runs the `athel` CLI, so the
claim needs proof that the model cannot forge.

## Decision
- **Grant creation.** Only the UserPromptSubmit hook creates a grant, and only when the prompt is
  exactly `/athel:approve CHG-NNN` (→ plan_approved) or `/athel:accept CHG-NNN` (→ done) and has no
  `agent_id`. Grants are appended to `.athel/state/grants.jsonl`, which the model cannot write
  (`.athel/` writes are tier X). Each creation is audited as `grant_created`.
- **Hook check (rule H).** PreToolUse denies `athel … --by human` with `E-POLICY-HUMAN-ORIGIN`
  unless the call is from the main agent and an unused grant younger than 10 minutes exists for
  the same session, change and target. This applies in guard mode and in change mode.
- **CLI check.** `state transition --to plan_approved|done --by human` itself consumes the grant,
  as the last step before the state changes, under the grant-book lock. Without a grant it fails
  with `E-POLICY-HUMAN-ORIGIN`. This is stricter than M1 plan §4.5 ("the CLI consumes the
  grant"): it also covers model-run processes the hook cannot see into, such as an interpreter
  script that calls `athel`. Consumption is audited as `grant_used`.
- **Skills.** `/athel:approve` and `/athel:accept` set `disable-model-invocation: true`
  (code.claude.com/docs/en/skills) and run the transition from the main conversation.

## Alternatives rejected
- Trusting `--by human`: the model can type it.
- Asking for approval with a PreToolUse `ask`: the human's click approves a tool call, not a
  specific plan hash, and does not work headless.

## Consequences
- A human can no longer approve from a terminal with `athel … --by human`. The path is the slash
  command inside Claude Code.
- A failed transition, for example a hand-edited plan, keeps the grant. A successful one uses it.
- **Not yet verified live:** that UserPromptSubmit receives the literal `/athel:approve CHG-NNN`
  text for a plugin skill command. This is checked in PR-13's `claude -p` spot check. If it does
  not, the owner decides a fallback.

## Follow-up (2026-11-09): verified live
`tests/live/claude-behaviours.sh` (check B) ran `claude -p "/athel:approve CHG-001"` with Athel's
plugin (Claude Code 2.1.311, model haiku, macOS arm64) on a fixture change in `planned` with a plan. Results:
- The UserPromptSubmit payload carried `"prompt":"/athel:approve CHG-001"` literally.
- Athel recorded `grant_created`; the skill's `state transition --by human` used the grant
  (`grant_used`), and the change reached `plan_approved`.
- `athel audit verify` passed.

The decision stands unchanged; no fallback is needed.

## Spec references
Doc 02 §Transition rules, §Security baseline; doc 04 §Audit log; M1 plan §4.5, decision Q4.
