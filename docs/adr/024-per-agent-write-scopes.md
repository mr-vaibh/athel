# ADR 024: Each build agent writes only what its role allows

Status: Accepted (2026-11-15; M4 task 12b, under the owner's delegation for technical choices;
M4-plan G3)

## Context
Doc 06 gives each role agent its own tool list and says independence between the author and
the checkers is the point:

| Agent | Doc 06 tools |
|---|---|
| Developer | read, edit, write in the worktree; build, test and lint commands; git commit |
| QA | read; write only under test paths; test commands |
| Security | read; scanners; no edit |
| Reviewer | read only |

It does not say how this is enforced. A prompt alone is not enough: QA that "fixes" the bug its
test found, or a reviewer that edits code and then approves it, is exactly the self-grading the
roles exist to prevent. The agents also reach Athel's state through the `athel` CLI, and some
CLI calls belong to one role only. Only the reviewer moves a finding to verified (doc 06 round
rule 2), and only the developer marks a fix.

Claude Code reports each plugin subagent's `agent_type` (`athel:<name>`) on every hook call.
The scout and the architect already have engine rules keyed on it (M2, M3).

## Decision
The engine applies rule 3 per role, before any other change-mode rule, with
`E-POLICY-AGENT-SCOPE`, or `E-POLICY-AGENT-READONLY` for an agent that may not write at all.

**Developer (`athel:developer`)**
- Its file and shell writes follow the normal change rules: inside its task's planned paths in
  the worktree (rules 7 and 10). Pushes and PRs go through the CLI, which only the skill runs
  (ADR 026).

**QA (`athel:qa`)**
- It may write only test files inside the change's worktrees. The worktree-relative path must
  match the test-file patterns of the gates package: `_test.go`, `.test.` and `.spec.` for
  JS and TS, `test_*.py` and `*_test.py`, and `test/`, `tests/` and `__tests__/` folders.
- A shell write with no file target counts only when it is a test run (verb `test`, such as
  `go test` or `npm test`). Anything else is denied, including `git commit` and `git add`. The
  build skill commits QA's tests.

**Security (`athel:security`) and reviewer (`athel:reviewer`)**
- No file or shell write at all, like the architect.
- The scanners run as the `security-scan` gate, and the security agent reads the results with
  `athel gate show`.

**CLI commands per role**
Every `athel` read is allowed. Of the commands that write, each role may run only these:

| Role | Writing `athel` commands |
|---|---|
| developer | `build save`, `review fix`, `review dispute`, `finding add` |
| qa | `qa save`, `finding add` |
| security | `security save`, `finding add` |
| reviewer | `review save`, `finding add` |

Everything else is the build skill's, in the main session: state transitions, manifests, gates,
pushes, PRs and `build done`. The human-only commands stay under rule H for every subagent.

A test keeps this table equal to the CLI's registered commands, and the agent prompts name
only commands their role may run.

## Alternatives rejected
- **Prompts only.** The model can ignore them, and an injected instruction in a diff can
  override them. Doc 04 puts enforcement in hooks, not in prompts.
- **Claude Code's `tools` and `disallowedTools` only.** They cannot tell a test file from a
  product file, nor one `athel` command from another. They are still set in each agent's
  front matter, as a second layer.
- **Test paths from `conventions.md`.** It is free text written by the architect, so the engine
  cannot read it reliably. The language defaults cover the fixture and eval repos. A per-repo
  pattern list is in the backlog.

## Consequences
- A QA agent that tries to fix product code is denied, and its prompt tells it to file a
  finding instead.
- A checker cannot change what it judges, and a developer cannot verify its own fix.
- A new role or a new CLI command must be added to the table, or the test fails.
- The rules key on `agent_type`. A subagent not named `athel:*` gets the main agent's rules.
  The build skill only delegates to the four named agents.

## Spec references
- Doc 06: §Role agents, §Independence rules, round rules 2 and 3.
- Doc 04: rule 3 (always denied), §Injection defence layer 4.
- FR-BUILD-004, FR-BUILD-005, FR-BUILD-006.
- M4-plan G3.
