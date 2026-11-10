# HANDOFF

Last updated: 2026-11-09, session 3, the first session on the owner's personal machine. Tools
are installed system-wide with Homebrew (`docs/context/environment.md`).

## Where we are

- **M0 Skeleton and M1 Guardrails: done.** PRs #1–#38, plus #40 (personal machine), #41 (live
  checks, 9/9) and #42 (M2 plan), all merged into `main`.
- **M2 Understand: plan approved 2026-11-09** (all Q1–Q12 recommendations). Phase 3 (build) is
  running task by task, one stacked PR per task (`docs/plans/M2-plan.md` §9).

## M2 build status (stacked PRs; merge in order only when the owner says so)

| # | Task | Branch | PR | Status |
|---|---|---|---|---|
| 01 | Schemas, `change@3`, `config@2`, migrate, IDs | `feat/m2-schemas` | #43 | open, base `main` |
| 02 | Fact store + `athel fact` commands | `feat/m2-facts-store` | #44 | open, base #43 |
| 03 | Suspicious-content flags, scout capture | `feat/m2-observe` | #45 | open, base #44 |
| 04 | Recipe verification, supersede, change flags | `feat/m2-fact-verify` | #46 | open, base #45 |
| 05 | Investigation validation, confidence ceilings | `feat/m2-investigation` | — | in progress |
| 06–15 | See M2-plan §9 | — | — | not started |

## Exact next step

Finish PR 05 on `feat/m2-investigation` (stacked on #46):
- `internal/investigation`: validate `investigation@1`, compute confidence ceilings (M2-plan
  §4.3, G8, G9), enforce mandatory sources per question type, render `investigation.md`, apply
  the 2,000-word limit.
- `athel investigation save` writes facts. It must **refuse cited facts that have no access
  record** for that change or ask (`fact find/show` do not log uses).

Then continue with PR 06 (repo index) per M2-plan §9. Two things to confirm live in PR 10: the
scout's real `agent_type` string (`observe.ScoutAgentType`, assumed `athel:scout`) and PostToolUse
`additionalContext`.

## Setting up a machine

Follow `docs/context/environment.md` ("Setting up a fresh machine from zero"). The
start-of-session routine is in `CLAUDE.md` ("Session rhythm").

## GitHub account on the personal machine

`gh` runs as **vs-snabbit**, through a `GITHUB_TOKEN` environment variable. The owner chose this
on 2026-11-09 and added vs-snabbit as a collaborator with write access. Commits use the repo-local
identity mr-vaibh / shuklavaibhav336@gmail.com.

## How the work is run (the owner's standing instructions)

- Four phases per milestone: plan (stop for approval) → scaffold → build task by task (one PR
  each) → prove the exit criteria in `Mn-report.md`.
- Ask questions in batches, each with options and a recommendation; log every owner decision in
  `docs/context/decisions-from-chat.md`.
- After a plan is approved the owner has said "build everything in one shot" without stopping.
  The agent merged the M0 and M1 stacks into `main` only when the owner explicitly asked. Ask
  again for each new milestone.
- Stacked PRs: each task branch is cut from the previous one, and each PR targets the previous
  branch. **GitHub does not retarget stacked PRs automatically.** To merge, take the PRs in
  order, and for each one:
  - retarget it to `main`;
  - wait for MERGEABLE/CLEAN;
  - check that the commits new to `main` equal the PR's own commit count;
  - merge with a merge commit;
  - verify the branch head is now in `main`.
- Never run cloud CLIs (gcloud, aws, kubectl against real clusters) for testing. Athel's tests
  only *classify* commands, and every test that spawns `athel` uses a temporary HOME.
- Commits: conventional (`feat|fix|refactor|test|docs|chore(scope): summary`), ending with the
  Claude co-author trailer. PR bodies: what and why, spec/requirement IDs, how tested, deferred.

## Conventions and lessons learned

- Run the gate as `set -o pipefail; scripts/check.sh` when piping it; it runs with `-count=1`,
  because tests that build the binary are invisible to Go's test cache.
- Tests and benches that spawn `athel` pass a minimal env with a temp `HOME`.
- gocyclo limit 15: split functions, never weaken a lint.
- Build commit messages with a variable (`T=$'\n\nCo-Authored-By: …'`); `$'…'` inside double
  quotes does not expand. One pushed commit on `feat/m1-policy-engine` has that literal suffix;
  left as is (no history rewrites).
- The CLI needs a session ID (`--session` or `ATHEL_SESSION_ID`) for transitions; a fixture
  that sets one must release its lock before a Claude session can take the change.
- CI runs the Go scenario tests, not the shell demos; run `tests/scenarios/m0-resume/demo.sh`
  by hand when touching state output.
- macOS: `/var` is a symlink to `/private/var`; compare paths in the form the caller gave them.
- Claude Code facts used by Athel (the `onFailure`, UserPromptSubmit and `updatedToolOutput`
  facts were confirmed live on 2026-11-09 by `tests/live/claude-behaviours.sh`):
  - Deprecated PreToolUse `"approve"` means allow, so Athel's approve maps to `ask`.
  - `onFailure: "block"` makes a failing PreToolUse hook block the call.
  - UserPromptSubmit gets a typed skill command as typed.
  - `updatedToolOutput` must match the tool's output shape.
  - plugin.json has no min-version field.
  - `--bare` skips plugin hooks.

## Open questions for the owner

- Merge the M2 stack when it is complete, or earlier?
- Q11 (plan): the paid eval runs need a go-ahead when PR 14 is ready.

## Owner instructions given in chat (all also in docs/context/decisions-from-chat.md)

- 2026-11-02: On the personal PC, no `./.tools` or `scripts/env.sh`; Claude installs the tools
  with Homebrew. The handoff prompt was rewritten and committed straight to `main` on the
  owner's explicit instruction ("just fucking commit on main"). The borrowed machine was then
  cleaned up: the gh login was removed and the local clone deleted.

- 2026-11-02: "next phases of implementation i'll do on my personal PC … wrap up the work here,
  and write proper documentation … proper handoff … how to continue … commit and push
  everything on this machine and then destroy everything of mine - gh account linked on this,
  the repo, but not before my confirmation".
- 2026-11-09: tools via Homebrew (approved); gh account vs-snabbit (owner's choice, against the
  recommendation to log in as mr-vaibh); vs-snabbit added as a collaborator; keep the merged
  M0/M1 branches. The owner asked for the work to be done "properly … production enterprise
  level … accurate and precise".
