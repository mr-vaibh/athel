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

1. Install Claude Code, then run:
   ```sh
   git clone https://github.com/mr-vaibh/athel.git && cd athel && claude
   ```
2. Paste the prompt below. Approve Claude's commands as it asks. When it asks for the GitHub
   login, type `! gh auth login` in the Claude prompt and log in as `mr-vaibh`.

## Prompt to paste into the first session on the new machine

```
This is Athel, my spec-driven agentic development framework (a Claude Code plugin plus a Go
helper CLI named athel). M0 and M1 are done and merged. We were on a borrowed machine; this is
now my own PC.

1. Read CLAUDE.md, docs/context/background.md, docs/plans/HANDOFF.md, docs/plans/ROADMAP.md,
   docs/plans/M1-report.md, the latest file in docs/journal/, docs/context/decisions-from-chat.md
   and docs/context/environment.md. They hold the full context; there is no other chat history.
2. This is my own machine. Ignore the "Environment: borrowed machine" section of CLAUDE.md and
   do NOT source scripts/env.sh or use ./.tools. Install the tools system-wide with Homebrew
   (ask me before running): `brew install go golangci-lint gh`. Go must be the latest stable
   release; golangci-lint must be v2 (CI pins v2.15.0). Then ask me to run `! gh auth login`
   and check that `gh auth status` shows mr-vaibh. Set the repo's git identity to mr-vaibh /
   shuklavaibhav336@gmail.com (repo-local, not --global).
3. Run `set -o pipefail; scripts/check.sh` and the red-team suite
   (`go test -count=1 ./tests/redteam/`) to confirm main is green on this machine.
4. First PR (branch docs/personal-machine):
   - delete the borrowed-machine section from CLAUDE.md;
   - delete scripts/env.sh and the .tools/ entry in .gitignore;
   - rewrite docs/context/environment.md for a normal setup (Homebrew tools, plain go/gh);
   - update every doc that says to source scripts/env.sh (HANDOFF.md, README.md, ROADMAP.md,
     M0/M1 docs only where they give instructions).
   Gate it and open the PR.
5. Then do the remaining "First tasks on the personal machine" in HANDOFF.md:
   - live-check onFailure:block, the literal /athel:approve prompt and updatedToolOutput, using
     fixtures only;
   - ask me about deleting the old merged branches.
6. Then start M2 Phase 1: read doc 08 (M2 row and fixture system) and doc 03 in full, write
   docs/plans/M2-plan.md in the same shape as M1-plan.md, and stop for my approval.

Follow CLAUDE.md's engineering standards and git conventions throughout (feature branches,
stacked PRs, conventional commits, journal and decision log kept current).
Tell me in 5 lines where we are before you start step 4.
```

## First tasks on the personal machine (in this order)

1. **Remove the borrowed-machine setup** (the owner chose a normal system-wide setup on the
   personal machine). In one PR (`docs/personal-machine`):
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
