# HANDOFF

Last updated: 2026-11-01, end of session 1.

## Where we are

- **Milestone M0 Skeleton is complete.** All three exit criteria are proven in
  `docs/plans/M0-report.md` (real fresh Claude Code sessions resume a change with nothing lost;
  every package ≥ 90% coverage, 95.2% total; hook p95 12.22 ms locally, 26.20 ms on CI, budget
  50 ms).
- Everything is in **22 stacked PRs (#1–#22)**, all pushed, all green on CI. Merging by the agent
  is blocked by Claude Code's permission classifier, so **the owner merges them in order**:
  #1 (base `main`) → #2 → … → #22. When a merged PR's branch is deleted, GitHub retargets the next
  PR to `main` automatically; if not, retarget it by hand. Use merge commits (not squash) to keep
  the per-task commits, or squash per PR if preferred; history is linear either way.
- The next milestone is **M1 Guardrails** (doc 08). Do not start it before the owner has merged
  M0 and said to begin.

## PR stack

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
2. If the owner has merged the stack: `git checkout main && git pull`, run `scripts/check.sh`.
3. When the owner says to start M1: read docs 02 and 04 again, then write
   `docs/plans/M1-plan.md` the same way as M0 (Phase 1, stop for approval). Report §5 lists
   what M1 needs first.

## Open questions waiting for the owner

- Merge the 22 stacked PRs (owner action).
- Confirm M0 is accepted and when to begin M1.

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
