# HANDOFF

Last updated: 2026-11-10, session 4 (PR 14 eval runs), the first session on the owner's personal machine. Tools
are installed system-wide with Homebrew (`docs/context/environment.md`).

## Where we are

- **M0 Skeleton and M1 Guardrails: done.** PRs #1–#38, plus #40 (personal machine), #41 (live
  checks, 9/9) and #42 (M2 plan), all merged into `main`.
- **M2 Understand: plan approved 2026-11-09** (all Q1–Q12 recommendations). Phase 3 (build) is
  running task by task, one stacked PR per task (`docs/plans/M2-plan.md` §9).

## M2 build status (stacked PRs; merge in order only when the owner says so)

| # | Task | Branch | PR |
|---|---|---|---|
| 01 | Schemas, `change@3`, `config@2`, migrate, IDs | `feat/m2-schemas` | #43 (base `main`) |
| 02 | Fact store + `athel fact` commands | `feat/m2-facts-store` | #44 |
| 03 | Suspicious-content flags, scout capture | `feat/m2-observe` | #45 |
| 04 | Recipe verification, supersede, change flags | `feat/m2-fact-verify` | #46 |
| 05 | Investigation checks, confidence ceilings | `feat/m2-investigation` | #47 |
| 06 | Repo index, human-only roots (`/athel:roots`) | `feat/m2-repo-index` | #48 |
| 07 | Developer-clone protection, read-only scout, knowledge reads | `feat/m2-policy` | #49 |
| 08 | Repo cache (FR-DISC-017) | `feat/m2-repo-cache` | #50 |
| 09 | Praxis allow-list, read-only MCP registry, playbooks | `feat/m2-registry-playbooks` | #51 |
| 09b | Hook profile; policy cache closed as not worth building | `perf/m2-hook-profile` | #52 |
| 10 | Scout agent, skills `investigate`/`ask`, `ask.yaml`, SubagentStop check | `feat/m2-scout` | #53 |
| 11 | Blast radius (ad-hoc), deterministic rules | `feat/m2-blast-adhoc` | #54 |
| 12 | `shop` fixture: services and git history | `feat/m2-fixture-code` | #55 |
| 13 | `shop` fixture: fake CLIs and MCP servers, FR-DISC-008 contract test | `feat/m2-fixture-runtime` | #56 |
| 14 | Eval cases, grader, gated runner (paid run not yet done) | `feat/m2-evals` | #57 |
| 15 | M2 report, HANDOFF, ROADMAP | `docs/m2-report` | not started |

## Exact next step

M2 is built; `docs/plans/M2-report.md` (PR 15, `docs/m2-report`, stacked on #57) has the full
results. Exit-criteria status:

| Criterion | Status |
|---|---|
| Expected claims found | 100% (bar ≥ 95%) |
| Traps | 29/30 |
| Exact labels | 85.7–90.5% (bar 100%) |

The whole stack is green in CI. Four full eval runs (about $57 in all), history in
`tests/evals/history/`. The last fix (independent-source hint, re-save on hints) is not yet
measured.

1. Done: the owner accepted M2 with the measured shortfall (2026-11-10).
2. **Waiting for the owner:** whether to merge the stack #43 → #58, in order: retarget each to
   `main`, wait for CLEAN, merge commit, verify.
3. After merge, M3 planning (doc 08 M3 row): write `M3-plan.md` and stop for approval.

## Open questions for the owner

- Merge the M2 stack when it is complete, or earlier?
- Merge the M2 stack #43 → #58 now?

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
