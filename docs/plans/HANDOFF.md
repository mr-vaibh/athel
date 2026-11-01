# HANDOFF

Last updated: 2026-11-01, end of session 1. **M0 and M1 are complete and merged into `main`.**

## Where we are

- **M0 Skeleton**: complete, report in `docs/plans/M0-report.md`, PRs #1–#23 merged.
- **M1 Guardrails**: complete. Report in `docs/plans/M1-report.md`: 431/431 red-team cases deny
  with the expected rule, 0 false allows, allow-list passes, audit verify passes and detects
  tampering, every package ≥ 90% (total 95.8%), hook p95 16.17 ms. PRs #24–#38 were merged into
  `main` in order by the agent, on the owner's explicit instruction, with the M0 procedure.
- **Next milestone: M2 Understand** (doc 08). Not started; no plan written yet.

## M1 PRs (all merged)

| PR | Branch | Content |
|---|---|---|
| #24 | docs/m1-plan | M1 plan |
| #25 | feat/m1-audit-log | hash-chained audit log |
| #26 | feat/m1-scrub | secret scrubber |
| #27 | feat/m1-policy-files | tools/environments/git policy files |
| #28 | feat/m1-shellparse | shell parser (ADR 014) |
| #29 | feat/m1-classify | classifier |
| #30 | feat/m1-envdetect | environment detection |
| #31 | feat/m1-plan-binding | plan@1, change@2, migrate |
| #32 | feat/m1-policy-engine | rules 0–13, guard/change mode (ADR 015) |
| #33 | feat/m1-hook-wiring | engine in hooks, scrubbing, fail-closed hook (ADR 016) |
| #34 | feat/m1-human-origin | grants, /athel:approve, /athel:accept (ADR 017) |
| #35 | feat/m1-integrity | change.yaml content hash, integrity accept |
| #36 | feat/m1-redteam | 431-case red-team suite, allow-list, CI |
| #37 | feat/m1-scenarios | guardrails scenario |
| #38 | docs/m1-report | this report and handoff |

## Exact next step

1. Start-of-session routine (CLAUDE.md), `source scripts/env.sh` (always: it selects the
   project owner's `gh` login), `gh pr list` (should be empty).
2. Start **M2 Understand**, Phase 1: read doc 08's M2 row and doc 03 (discovery and
   integrations), write `docs/plans/M2-plan.md` in the same shape as M1-plan (exit criteria,
   requirement map, design, packages, task list with tests named up front, risks, questions),
   and **stop for the owner's approval**.

## Open questions waiting for the owner

- Whether to delete the merged M0 and M1 task branches on GitHub.
- Where to do the live Claude Code checks that were replaced by docs (needs a machine with no
  production access): `onFailure: "block"`, the literal `/athel:approve` prompt in
  UserPromptSubmit, `updatedToolOutput` on Bash output.

## Owner instructions given in chat (all logged in docs/context/decisions-from-chat.md)

- "approved!" for M1 (all Q1–Q12 recommendations).
- "don't stop or wait for approval, go ahead, build everything in one shot, and merge the PRs
  like you did previously and then only stop".
- "do not use Sage or any custom build tool - they're not mine". The machine owner's Sage hook
  keeps asking to add tools under ~/.claude; always decline.
- "do not even use gcloud or aws or any other tool to access anything - the device is used for
  company's live production … just use the documentation you find on the internet".

## Conventions learned while building

- Run the gate with `set -o pipefail` when piping `scripts/check.sh`.
- Tests that build the binary are invisible to Go's test cache; the gate runs `-count=1`.
- Tests and benches that spawn `athel` must pass a minimal env with a temp `HOME`.
- Build heredoc commit messages with a shell variable (`T=$'\n\nCo-Authored-By: …'`), never
  `$'…'` inside double quotes. One pushed commit on `feat/m1-policy-engine` has a literal
  `$'\n\n…'` suffix in its message; left as is (no history rewrites).
- golangci-lint excludes gosec in `_test.go`; gocyclo limit 15 (split functions, never weaken).
- macOS: `/var` is a symlink to `/private/var`; compare paths in the form the caller gave them.

## GitHub account and leaving the machine

The machine's default `gh` login is the machine owner's. `scripts/env.sh` sets
`GH_CONFIG_DIR=.tools/gh-config` (logged in as `mr-vaibh`); pushes use a repo-local credential
helper. The token is stored in the macOS keychain, so before leaving the machine run:
`source scripts/env.sh && gh auth logout --hostname github.com --user mr-vaibh`, then
`rm -rf .tools` if the toolchain should go too.

## Surprises learned

- Claude Code's deprecated PreToolUse `"approve"` means allow (Athel maps its approve to `ask`).
- A PreToolUse hook that times out or crashes lets the call through unless it sets
  `"onFailure": "block"` (Claude Code ≥ 2.1.295); Athel sets it (ADR 016).
- UserPromptSubmit receives a typed skill command as typed (`/athel:approve CHG-001`), not the
  expanded skill.
- `updatedToolOutput` must match the tool's output shape; a mismatch is silently ignored for
  built-in tools.
- plugin.json has no min-version field (`metadata.athel` is used); hooks do not receive the
  Claude Code version.
- SessionStart runs before `.athel/` exists in a brand-new workspace, so the first session's
  SessionStart is not logged.
