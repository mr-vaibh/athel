# HANDOFF

Last updated: 2026-11-01, end of session 1 (Phase 1 stop).

## Where we are

- Milestone **M0 Skeleton**, **Phase 1 (understand and plan) complete**, waiting for the
  owner's approval of `docs/plans/M0-plan.md` and answers to its questions (§11, Q0–Q14).
- No code exists yet. Phase 2 (scaffold PR) has not started.

## Task status (M0-plan §9)

| Task | Status |
|---|---|
| Phase 1 plan + context docs | Done, committed locally on `docs/m0-plan`, **not pushed** |
| PR-00 scaffold | Not started (Phase 2, needs approval) |
| PR-01 … PR-19 | Not started |

## Done this session

- Tools: Go 1.27.3 and golangci-lint 2.15.0 installed in `.tools/` with verified checksums
  (`docs/context/environment.md`). `scripts/env.sh` and `.gitignore` created.
- All 8 spec PDFs read in full; Claude Code docs checked.
- Docs written: `docs/plans/M0-plan.md`, `docs/context/background.md`,
  `docs/context/environment.md`, `docs/context/decisions-from-chat.md`,
  `docs/plans/backlog.md`, `docs/journal/2026-11-01-session-1.md`, this file.
- Commit: see `git log docs/m0-plan` (local only).

## BLOCKER: GitHub account

`gh auth status` on this machine shows the **machine owner's** account, not `mr-vaibh`.
Per CLAUDE.md nothing was pushed and no `gh` command was used. To unblock (recommended, Q0):

```sh
source scripts/env.sh        # sets GH_CONFIG_DIR=./.tools/gh-config
gh auth login                # log in as mr-vaibh; stored inside the repo folder only
git config --local credential.helper ""        # ignore the machine's global helper
git config --local --add credential.helper "!gh auth git-credential"
gh auth status               # must show mr-vaibh
git push -u origin docs/m0-plan
```

## Exact next step

1. Run the start-of-session routine (CLAUDE.md), then `source scripts/env.sh`.
2. If the owner has logged in as `mr-vaibh`: push `docs/m0-plan`, open a PR with
   `gh pr create`.
3. Apply the owner's answers to Q0–Q14: log each in `docs/context/decisions-from-chat.md`,
   update `docs/plans/M0-plan.md`.
4. Once the plan is approved, start Phase 2: branch `feat/m0-scaffold` from `main` (after the
   plan PR is merged), build PR-00 exactly as described in M0-plan §9.

## Open questions waiting for the owner

Q0–Q14 in `docs/plans/M0-plan.md` §11 (GitHub login; step vs transition model; return-to-
planned states; heartbeat throttling; draft blockable; in_review→building; skipping
awaiting_decisions; overridable format; platforms; malformed-payload stub decision;
E-INTEGRITY timing; spec folder path; resume demo via `claude -p`; the two dependencies).

## Chat instructions not written elsewhere

All logged in `docs/context/decisions-from-chat.md` (kickoff instructions). The full kickoff
prompt is committed as `KICKOFF-PROMPT.md`.

## Surprises learned

- Claude Code's deprecated PreToolUse value `"approve"` means **allow**; the spec's "approve"
  means *ask the human*. The adapter must map Athel `approve` → `ask` (M0-plan D2).
- No plugin.json field for the minimum Claude Code version; use free-form `metadata`.
- The Claude Code version is not exposed to hooks; SessionStart must run `claude --version`.
- Skills can use `${CLAUDE_SESSION_ID}`; SessionStart hooks can export env vars via
  `CLAUDE_ENV_FILE`.
- SessionEnd hooks default to a 1.5 s timeout.
- `pdftoppm` is not installed here; the Read tool still renders whole PDFs.
- Spec folder is `docs/specs/` (plural); CLAUDE.md says `docs/spec/`.
