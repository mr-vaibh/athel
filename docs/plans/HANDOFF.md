# HANDOFF

Last updated: 2026-11-02, end of work on the borrowed machine. **The project moves to the
owner's personal machine.** Everything needed to continue is in this repository on `main`.
Chat history from the borrowed machine does not come along, and nothing depends on it.

## Where we are

- **M0 Skeleton: done.** PRs #1–#23 merged. Report: `docs/plans/M0-report.md`.
- **M1 Guardrails: done.** PRs #24–#38 merged. Report: `docs/plans/M1-report.md`.
  - 431/431 red-team cases denied with the expected rule; 0 false allows.
  - The allow-list suite passes.
  - Audit verify passes, and tampering is detected.
  - Every package ≥ 90% coverage (95.8% total); hook p95 16.12 ms.
- **Next: M2 Understand, Phase 1 (plan).** Nothing started; no open PRs; `main` is green.
- What is left overall, milestone by milestone: **`docs/plans/ROADMAP.md`**.

## Moving to the personal machine: steps for the owner

1. Install Claude Code, git and the GitHub CLI (`gh`) the usual way for your machine.
2. Clone and enter the repo:
   ```sh
   git clone https://github.com/mr-vaibh/athel.git && cd athel
   ```
3. Start Claude Code in that folder (`claude`) and paste the prompt from the next section.
   Claude reads the docs, sets up Go and golangci-lint in `./.tools/` (CLAUDE.md, "Installing
   missing tools"), checks `gh`, and reports where things stand.
4. When it asks for GitHub access, log in to `gh` with your own account. On your own machine a
   normal `gh auth login` is fine. While the "borrowed machine" section of CLAUDE.md still
   exists, `scripts/env.sh` points `gh` at `./.tools/gh-config`, so run
   `source scripts/env.sh && gh auth login` once.
5. Approve the small first PR it proposes (see "First tasks on the personal machine"), then
   the M2 plan when it stops for approval.

## Prompt to paste into the first session on the new machine

```
This is Athel, my spec-driven agentic development framework (Claude Code plugin + Go CLI).
We are now on my personal machine; the work so far was done on a borrowed machine.
1. Read CLAUDE.md, docs/context/background.md, docs/plans/HANDOFF.md, docs/plans/ROADMAP.md,
   docs/plans/M1-report.md, the latest file in docs/journal/, docs/context/decisions-from-chat.md
   and docs/context/environment.md.
2. Run the start-of-session routine from CLAUDE.md (tool check; install Go and golangci-lint
   into ./.tools with checksum verification if missing; source scripts/env.sh; gh auth status
   must show mr-vaibh; gh pr list).
3. Run `set -o pipefail; scripts/check.sh` and the red-team suite to confirm main is green here.
4. Tell me in 5 lines where we are and what you will do next, then do the "First tasks on the
   personal machine" from HANDOFF.md, and then start M2 Phase 1 (write docs/plans/M2-plan.md and
   stop for my approval).
```

## First tasks on the personal machine (in this order)

1. **Remove the borrowed-machine rules.** CLAUDE.md says to delete its whole "Environment:
   borrowed machine" section once the project moves. Do that in a small PR
   (`docs/personal-machine`). Keep the `./.tools/` toolchain approach and `scripts/env.sh`: they
   work on any machine. Keep or drop the `GH_CONFIG_DIR` line in `scripts/env.sh` as the owner
   prefers (dropping it makes `gh` use the normal login), and update
   `docs/context/environment.md` to match.
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
- On the personal machine, keep the repo-local `gh` login (`GH_CONFIG_DIR`) or use the normal
  one?

## Owner instructions given in chat (all also in docs/context/decisions-from-chat.md)

- 2026-11-02: "next phases of implementation i'll do on my personal PC … wrap up the work here,
  and write proper documentation … proper handoff … how to continue … commit and push
  everything on this machine and then destroy everything of mine - gh account linked on this,
  the repo, but not before my confirmation".
