# HANDOFF

Last updated: 2026-11-10, session 4 (M2 merged), on the owner's personal machine. Tools
are installed system-wide with Homebrew (`docs/context/environment.md`).

## Where we are

- **M0 Skeleton, M1 Guardrails, M2 Understand: done and merged into `main`.**
  - M0 and M1: PRs #1–#42.
  - M2: PRs #43–#58, merged 2026-11-10 in order with merge commits; `main` CI green at
    `ec12150`.
  - The owner accepted M2 with a measured shortfall (`docs/plans/M2-report.md` §7).
- **AI eval target still open: `docs/plans/EVALS.md`.**
  - The confidence-label target is 100%; it measured 85.7–90.5%.
  - Traps: 29/30 against 30/30.
  - The owner: "We won't let it go." Re-run per `EVALS.md` §When to run again, with the
    owner's go-ahead (each run costs about $13).

## M2 PRs (all merged 2026-11-10)

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

M3 is approved (2026-11-10) and being built as stacked PRs on `docs/m3-plan` (#60):

| Task | Branch | PR | Status |
|---|---|---|---|
| plan | `docs/m3-plan` | #60 | open (base main) |
| 00 gateway-trap hint | `feat/m3-eval-carry` | #61 | done |
| 01 new schemas | `feat/m3-schemas` | #62 | done |
| 02 PRD intake | `feat/m3-prd` | #63 | done (also an M2 grader fix) |
| 03 decision cards | `feat/m3-decisions` | #64 | done |
| 04 /athel:decide (ADR 019) | `feat/m3-decide-human` | #65 | done |
| 05 specs | `feat/m3-spec` | #66 | done |
| 06 conventions | `feat/m3-conventions` | #67 | done |

Next:
1. Task 07, `feat/m3-plan-lints` on #67: `plan@2` with a task `kind` and migration from
   `plan@1` (ADR 002 bump), plus the coverage, ordering, size, rollback and env-action lints,
   `plan summary` and `plan diff`.
2. Then 08 trace, 08b fixture, 09 planned blast, 10 actual blast, 11a architect policy and
   templates, 11b skills, 13 evals, 14 report.

The M2 eval re-run (run 5) is in progress from a frozen worktree (`.worktrees/m2-eval`,
detached at `abfebf0`, locally excluded). When it finishes:
- copy `tests/evals/history/m2-*.json` from the worktree into the M3 stack;
- regrade it with the current grader;
- update `EVALS.md`;
- remove the worktree (`git worktree remove .worktrees/m2-eval`).

M3 eval runs: at most 2 full runs, each asked first (owner, 2026-11-10).

## Open questions for the owner

- M3 plan review, Q1 and Q2 (M3-plan §11). Each eval run needs a go-ahead (cost).

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
