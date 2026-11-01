# HANDOFF

Last updated: 2026-11-01, session 1, M1 Phase 3 (build) in progress: PR-12 of 14.

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
- **M1 Guardrails**: plan approved ("approved!", all Q1–Q12 recommendations). Building in
  stacked PRs; each PR's base is the previous branch (all open, none merged yet):

  | PR | Branch | Content |
  |---|---|---|
  | #24 | docs/m1-plan | M1 plan |
  | #25 | feat/m1-audit-log | hash-chained audit log, `audit verify/tail` |
  | #26 | feat/m1-scrub | secret scrubber |
  | #27 | feat/m1-policy-files | tools/environments/git policy files |
  | #28 | feat/m1-shellparse | shell parser (mvdan.cc/sh) |
  | #29 | feat/m1-classify | classifier (+ handoff commit) |
  | #30 | feat/m1-envdetect | environment detection |
  | #31 | feat/m1-plan-binding | plan@1, change@2, migrate |
  | #32 | feat/m1-policy-engine | rules 0–13, guard/change mode (ADR 015), `policy decide` |
  | #33 | feat/m1-hook-wiring | engine in PreToolUse, PostToolUse scrub, `onFailure: block` (ADR 016) |
  | #34 | feat/m1-human-origin | grants, `/athel:approve`, `/athel:accept` (ADR 017) |
  | #35 | feat/m1-integrity | change.yaml content hash, `integrity accept` |

  In progress: PR-12 `feat/m1-redteam` (this branch). Then PR-13 `feat/m1-scenarios` (with a
  real `claude -p` spot check: confirm `onFailure: block`, `updatedToolOutput` on Bash output,
  and that UserPromptSubmit receives the literal `/athel:approve CHG-NNN`), PR-14
  `docs/m1-report`. Owner merges stacks (the agent may merge only when explicitly told; GitHub
  does not retarget stacked PRs automatically: retarget each to main before merging, as done
  for M0).
- Deviations/interpretations to report to the owner: engine package is `engine` not `policy`
  (import cycle, ADR 015); `policy decide` takes flags plus `--cwd`; policy cache deferred
  (p95 15.9 ms, ADR 016); CLI *requires* grants for `--by human` (stricter than plan, ADR 017);
  integrity is sealed on every CLI write, not only transitions (#35).
- One pushed commit on `feat/m1-policy-engine` has a broken message suffix
  (`docs(journal): … $'\n\nCo-Authored-By…'`); left as is (no history rewrites).

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
2. Continue M1-plan §9 from the first task without an open PR (`gh pr list`), branching from
   the last M1 branch, one PR each. Same loop: failing tests → code → `scripts/check.sh` (with pipefail) →
   self-review → commit → push → PR.

## Open questions waiting for the owner

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
