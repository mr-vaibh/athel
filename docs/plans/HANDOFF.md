# HANDOFF

Last updated: 2026-11-01, session 1, M1 Phase 1 (plan written, waiting for approval).

## Where we are

- **Milestone M0 Skeleton is complete.** All three exit criteria are proven in
  `docs/plans/M0-report.md` (real fresh Claude Code sessions resume a change with nothing lost;
  every package ≥ 90% coverage, 95.2% total; hook p95 12.22 ms locally, 26.20 ms on CI, budget
  50 ms).
- **All 22 PRs (#1–#22) are merged into `main`** (merge commits, in order; done by the agent on
  the owner's explicit instruction). Before each merge the PR was retargeted to `main` (GitHub did
  not do it automatically), checked MERGEABLE/CLEAN with exactly its own commits, and after each
  merge its branch head was verified to be contained in `main`. `scripts/check.sh` and the resume
  demo pass on `main` at c23c033. The task branches still exist on GitHub (all contained in
  `main`); delete them when convenient.
- **M1 Guardrails** started on the owner's "go M1". Phase 1 is done: `docs/plans/M1-plan.md`
  (branch `docs/m1-plan`, PR open) with 12 questions (Q1–Q12). **Stop: waiting for the owner's
  approval and answers.** Nothing of M1 is built yet.

## Merged PRs

| PR | Branch | Content |
|---|---|---|
| #1 | docs/m0-plan | plan, context docs, toolchain env |
| #2 | feat/m0-scaffold | layout, CI, plugin skeleton, ADRs 001/002/003/010 |
| #3 | feat/m0-errors | typed errors, exit codes |
| #4 | feat/m0-fsutil-clock-ids | atomic writes, clock, IDs, digests |
| #5 | feat/m0-oslock | OS file locks |
| #6 | feat/m0-schema | schemas, validator, ADRs 012/013 |
| #7 | feat/m0-change-model | change model, codec, store |
| #8 | feat/m0-eventlog | JSONL event log |
| #9 | feat/m0-state-machine | transition table |
| #10 | feat/m0-session-lock | session lock |
| #11 | feat/m0-steps-checkpoints-budget | steps, checkpoints, budgets, ADR 011 |
| #12 | feat/m0-config-merge | merge engine |
| #13 | feat/m0-config-layers | config layers |
| #14 | feat/m0-cli-core | CLI core |
| #15 | feat/m0-cli-state | state/lock/budget/resume commands |
| #16 | feat/m0-cli-config-schema | config/schema commands |
| #17 | feat/m0-hook-adapter | Claude Code adapter |
| #18 | feat/m0-hook-handlers | hook handlers, stub PreToolUse |
| #19 | feat/m0-plugin-skills | skills, hooks.json |
| #20 | feat/m0-hook-bench | latency benchmarks |
| #21 | feat/m0-resume-demo | resume demo + scenario test |
| #22 | docs/m0-report | M0 report, handoff, journal |

## Exact next step

1. Start-of-session routine (CLAUDE.md), `source scripts/env.sh` (always: it selects the
   project owner's `gh` login), `gh pr list`.
2. If the owner approved the M1 plan: log every answer to Q1–Q12 in
   `docs/context/decisions-from-chat.md`, fold them into `docs/plans/M1-plan.md`, then build the
   tasks in M1-plan §9 in order (PR-01 audit log first), one PR each, stacked like M0 unless the
   owner merges as we go. Same loop: failing tests → code → `scripts/check.sh` (with pipefail) →
   self-review → commit → push → PR.

## Open questions waiting for the owner

- Approve `docs/plans/M1-plan.md` and answer Q1–Q12 (§11 of the plan).
- Whether to delete the 23 merged M0 task branches on GitHub.

## Owner instructions given in chat (all logged in docs/context/decisions-from-chat.md)

- Plan approved; all Q0–Q14 recommendations accepted.
- "start implementing, all in one go, no approval from now all, you have all the permission
  from my side"; implement directly or with agents/workflows; if a limit is reached, leave a
  prompt that revives the work.

## Conventions learned while building

- Run the gate with `set -o pipefail` when piping `scripts/check.sh` through `grep`/`tail`
  (once a failing check was masked and committed; fixed in #20).
- golangci-lint excludes gosec in `_test.go`, so no `//nolint:gosec` in tests.
- macOS `sed` has no `\b`; GNU vs BSD tools differ (e.g. `echo --version`), so tests must not
  depend on either.
- Human (non-JSON) CLI output sorts keys alphabetically and indents YAML by 4.
- `python3` was killed by the OS once when given large argv in a script; keep scripts in Go/sh.
- `go build ./...` at the repo root writes `./athel` (git-ignored).

## GitHub account and leaving the machine

The machine's default `gh` login is the machine owner's. `scripts/env.sh` sets
`GH_CONFIG_DIR=.tools/gh-config` (logged in as `mr-vaibh`); pushes use a repo-local credential
helper. The token is stored in the macOS keychain, so before leaving the machine run:
`source scripts/env.sh && gh auth logout --hostname github.com --user mr-vaibh`, then
`rm -rf .tools` if the toolchain should go too.

## Surprises learned

- Claude Code's deprecated PreToolUse `"approve"` means allow (Athel maps its approve → `ask`).
- plugin.json has no min-version field (`metadata.athel` used); hooks do not receive the Claude
  Code version (`claude --version` probe, ~0.3 s, SessionStart only).
- `${CLAUDE_SESSION_ID}` works in skill content; SessionStart's `CLAUDE_ENV_FILE` export reaches
  later Bash commands in real sessions (verified in the M0 demo).
- SessionStart runs before `.athel/` exists in a brand-new workspace, so the first session's
  SessionStart is not logged (nothing to log into yet).
