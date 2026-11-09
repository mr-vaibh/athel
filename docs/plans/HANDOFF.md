# HANDOFF

Last updated: 2026-11-09, session 3, the first session on the owner's personal machine. Tools
are installed system-wide with Homebrew (`docs/context/environment.md`).

## Where we are

- **M0 Skeleton: done** (PRs #1–#23). **M1 Guardrails: done** (PRs #24–#38). `main` was
  green on this machine at `83445f8`:
  - gate passes; red-team and allow-list suites pass;
  - hook latency p95 27.62 ms (budget 50 ms).
- **M2 Understand: Phase 1 (plan) written, waiting for the owner's approval.**
  `docs/plans/M2-plan.md` has 15 PRs and questions Q1–Q12, each with a recommendation.
- What is left overall: `docs/plans/ROADMAP.md`.

## Open PRs (stacked; merge in order, only when the owner says so)

| PR | Branch | Base | What |
|---|---|---|---|
| #40 | `docs/personal-machine` | `main` | Removes the borrowed-machine setup; rewrites environment.md; fixes `tests/scenarios/m0-resume/demo.sh` (it had failed since #35; it now isolates HOME). CI green |
| #41 | `test/m1-live-checks` | `docs/personal-machine` | `tests/live/claude-behaviours.sh`: 9/9 live checks pass (onFailure block, literal `/athel:approve`, `updatedToolOutput`); results in ADR 016/017 follow-ups and M1-report §6 |
| — | `docs/m2-plan` | `test/m1-live-checks` | M2 plan, journal, decision log, backlog, this handoff |

## Exact next step

1. Get the owner's answer on `docs/plans/M2-plan.md` (approve, or answer Q1–Q12) and log it in
   `docs/context/decisions-from-chat.md`.
2. Ask whether to merge #40, #41 and the plan PR into `main` (same procedure as M0/M1, below).
3. Then M2 Phase 2/3: PR 01 `feat/m2-schemas` from the plan's task list. Ask how to run M2
   (one shot or with stops), as for M0 and M1.

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

- Approve the M2 plan (Q1–Q12 in `docs/plans/M2-plan.md`).
- Merge #40, #41 and the M2 plan PR into `main`?
- How to run M2 after approval (one shot, as M1, or with stops)?

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
