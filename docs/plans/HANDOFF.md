# HANDOFF

Last updated: 2026-11-09, session 3, the first session on the owner's personal machine. The
borrowed-machine setup (`./.tools/`, `scripts/env.sh`) has been removed; tools are installed
system-wide with Homebrew (`docs/context/environment.md`).

## Where we are

- **M0 Skeleton: done.** PRs #1–#23 merged. Report: `docs/plans/M0-report.md`.
- **M1 Guardrails: done.** PRs #24–#38 merged. Report: `docs/plans/M1-report.md`.
  - 431/431 red-team cases denied with the expected rule; 0 false allows.
  - The allow-list suite passes.
  - Audit verify passes, and tampering is detected.
  - Every package ≥ 90% coverage (95.8% total); hook p95 16.12 ms.
- **Next: M2 Understand, Phase 1 (plan).** Nothing started; no open PRs; `main` is green.
- What is left overall, milestone by milestone: **`docs/plans/ROADMAP.md`**.

## Setting up a machine

Follow `docs/context/environment.md` ("Setting up a fresh machine from zero"), then start Claude
Code in the repository. The start-of-session routine is in `CLAUDE.md` ("Session rhythm"). The
one-time prompt used to move from the borrowed machine is in git history (commit `83445f8`,
this file).

## First tasks on the personal machine (in this order)

1. **Remove the borrowed-machine setup** (the owner chose a normal system-wide setup on the
   personal machine). **Done in session 3**, in one PR (`docs/personal-machine`):
   - delete CLAUDE.md's "Environment: borrowed machine" section;
   - delete `scripts/env.sh` and the `.tools/` entry in `.gitignore`;
   - rewrite `docs/context/environment.md` for Homebrew-installed tools;
   - update the docs that say to source `scripts/env.sh`.
2. **Live-check three Claude Code behaviours** that M1 verified only from the docs. Use only the
   fixture workspace and harmless commands; never point anything at real systems.
   - With `claude --plugin-dir ./plugin` (never install the plugin into user settings), confirm
     the PreToolUse `onFailure: "block"` behaviour: a hook that fails or times out blocks the
     call.
   - Confirm that UserPromptSubmit receives a typed `/athel:approve CHG-001` literally.
   - Confirm that the scrubbed `updatedToolOutput` replaces Bash output.

   Record the results in a journal entry, in ADR 016/017 follow-up notes if anything differs,
   and in the M1 report.
3. Ask the owner whether to delete the merged M0/M1 task branches on GitHub.
4. **Start M2 Phase 1**: read doc 08 (M2 row, fixture system) and doc 03 in full, write
   `docs/plans/M2-plan.md` in the same shape as `M1-plan.md`, and stop for approval. M2 needs
   the `tests/fixtures/shop` fixture system (doc 08 §Fixture system).

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
- macOS: `/var` is a symlink to `/private/var`; compare paths in the form the caller gave them.
- Claude Code facts used by Athel:
  - Deprecated PreToolUse `"approve"` means allow, so Athel's approve maps to `ask`.
  - `onFailure: "block"` makes a failing PreToolUse hook block the call.
  - UserPromptSubmit gets a typed skill command as typed.
  - `updatedToolOutput` must match the tool's output shape.
  - plugin.json has no min-version field.
  - `--bare` skips plugin hooks.

## Open questions for the owner

- Delete the merged M0/M1 task branches on GitHub?

## Owner instructions given in chat (all also in docs/context/decisions-from-chat.md)

- 2026-11-02: On the personal PC, no `./.tools` or `scripts/env.sh`; Claude installs the tools
  with Homebrew. The handoff prompt was rewritten and committed straight to `main` on the
  owner's explicit instruction ("just fucking commit on main"). The borrowed machine was then
  cleaned up: the gh login was removed and the local clone deleted.

- 2026-11-02: "next phases of implementation i'll do on my personal PC … wrap up the work here,
  and write proper documentation … proper handoff … how to continue … commit and push
  everything on this machine and then destroy everything of mine - gh account linked on this,
  the repo, but not before my confirmation".
