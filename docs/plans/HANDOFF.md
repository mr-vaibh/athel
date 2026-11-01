# HANDOFF

Last updated: 2026-11-01, session 1, during Phase 3 (build).

## Where we are

- Milestone **M0 Skeleton**. Plan approved by the owner (all recommendations Q0–Q14 accepted;
  see `docs/context/decisions-from-chat.md`). Owner asked for everything "in one go" with no
  further approvals.
- **PRs are stacked** because merging by the agent is blocked by Claude Code's permission
  classifier. Each task branch is cut from the previous one; each PR targets the previous
  branch. The owner merges in order: #1 → #2 → #3 → … (after each merge GitHub retargets the
  next PR automatically when its base branch is deleted; otherwise retarget it to `main`).

## Task status (M0-plan §9)

| Task | Branch | PR | Status |
|---|---|---|---|
| Phase 1 plan + docs | `docs/m0-plan` | #1 | open |
| PR-00 scaffold | `feat/m0-scaffold` | #2 | open, CI green |
| PR-01 errors | `feat/m0-errors` | #3 | open |
| PR-02 fsutil/clock/ids/digest | `feat/m0-fsutil-clock-ids` | #4 | open |
| PR-03 OS lock | `feat/m0-oslock` | #5 | open |
| PR-04 schema + deps | `feat/m0-schema` | #6 | open |
| PR-05 change model/store | `feat/m0-change-model` | #7 | open |
| PR-06a event log | `feat/m0-eventlog` | #8 | open |
| PR-06 state machine | `feat/m0-state-machine` | #9 | open |
| PR-07 session lock | `feat/m0-session-lock` | – | next |
| PR-08 … PR-19 | – | – | not started |

## Exact next step

`source scripts/env.sh`, `git checkout feat/m0-state-machine && git pull`, then
`git checkout -b feat/m0-session-lock` and build PR-07 per M0-plan §6.5 / §9:
logical session lock in `change.yaml` (`lock: {session, since, heartbeat_at, ttl_min}`),
`Store.AcquireLock/RefreshLock/ReleaseLock/LockStatus`, stale = now − heartbeat_at > ttl,
take-over only when stale and with a reason (logged as `lock_takeover` state event), second
session gets `E-STATE-LOCK-HELD` (exit 13) naming the holder, heartbeat throttled to 5 min
(Q3). Then PR-08 (steps/checkpoints/budget), PR-09/10 (config), PR-11–13 (CLI), PR-14/15
(hooks), PR-16 (skills), PR-17 (bench), PR-18 (resume demo), PR-19 (M0 report).

Working loop per task: branch → failing tests → code → `scripts/check.sh` (build, vet,
golangci-lint, race tests, per-package ≥90% coverage) → self-review → commit
(feat|fix|refactor|test|docs|chore only) → push → `gh pr create --base <previous branch>`.

## Conventions learned while building

- golangci-lint excludes gosec in `_test.go`, so never put `//nolint:gosec` in tests
  (nolintlint flags it as unused).
- macOS `sed` has no `\b`; use Python for word-boundary renames.
- `go build ./...` in the repo root writes an `athel` binary (git-ignored as `/athel`).
- `errors.HasCode` walks `errors.Join` trees; `ExitCode` uses the first `*Error` found.
- Every error path is typed (`cli/internal/errors`); untyped errors exit 1 (internal).

## Open questions waiting for the owner

None. (Merging the stacked PRs is the owner's action.)

## GitHub account

The machine's default `gh` login is the machine owner's. Always `source scripts/env.sh`
first: it points `GH_CONFIG_DIR` at `.tools/gh-config`, logged in as `mr-vaibh`. Pushes use a
repo-local credential helper. The token sits in the macOS keychain: before leaving the
machine run `source scripts/env.sh && gh auth logout --hostname github.com --user mr-vaibh`.

## Surprises learned

- Claude Code's deprecated PreToolUse `"approve"` means **allow**; Athel's "approve" (ask the
  human) must map to `"ask"` (M0-plan D2).
- No plugin.json field for the min Claude Code version: stored in `metadata.athel`.
- The running Claude Code version is not exposed to hooks; SessionStart runs `claude --version`.
- Skills can use `${CLAUDE_SESSION_ID}`; SessionStart hooks can export env vars via
  `CLAUDE_ENV_FILE`. SessionEnd hooks default to a 1.5 s timeout.
- Installed Claude Code on this machine: 2.1.308 (used as the min tested version).
