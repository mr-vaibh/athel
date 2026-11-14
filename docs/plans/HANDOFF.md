# HANDOFF

Last updated: 2026-11-11, session 4 (M2 merged; M3 full run 1 recorded), on the owner's personal machine. Tools
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

| Task | Branch | PR |
|---|---|---|
| plan | `docs/m3-plan` | #60 (base main) |
| 00 gateway-trap hint | `feat/m3-eval-carry` | #61 |
| 01 new schemas | `feat/m3-schemas` | #62 |
| 02 PRD intake | `feat/m3-prd` | #63 |
| 03 decision cards | `feat/m3-decisions` | #64 |
| 04 /athel:decide (ADR 019) | `feat/m3-decide-human` | #65 |
| 05 specs | `feat/m3-spec` | #66 |
| 06 conventions | `feat/m3-conventions` | #67 |
| 07 plan@2 and lints (ADR 020) | `feat/m3-plan-lints` | #68 |
| 08 trace and step guards (ADR 021) | `feat/m3-trace` | #69 |
| 08b fixture: eval PRDs, table sizes, prepared branches | `feat/m3-fixture` | #70 |
| 09 planned blast, /athel:signoff | `feat/m3-blast-planned` | #71 |
| 10 actual blast from the diff (ADR 022) | `feat/m3-blast-actual` | #72 |
| 11a architect agent, read-only policy, templates | `feat/m3-architect-policy` | #73 |
| 11b skills spec, decide, plan, signoff; status cards | `feat/m3-skills` | #74 |
| 13 M3 eval harness, plus fixes the smoke runs found (review and blast templates, draft errors, sandbox SSH mapping, E.164 split, architect maxTurns 60) | `feat/m3-evals` | #75 |
| 14 M3 report, ROADMAP | `docs/m3-report` | #76 |

Paid runs so far (each approved by the owner; details in `EVALS.md`): smokes 1–3 not valid,
and each found a cause that was fixed; smoke 4 valid ($3.70). Smokes total $12.11. Full run 1
($28.27): 8 of 9 valid, so EC-1 is not met; blast recall 100%, so EC-2 is met. The failed run's
cause (a card option "re-investigate first", which no lifecycle step can follow) is fixed in
the architect's instructions at `0e7ee85`. Card precision is 20%, reported only. Total paid
for M3 so far: $40.38.

Next:
1. 13: **done.** Full run 2 met the M3 eval bar: 9/9 valid, blast recall 100%, $29.74
   (`EVALS.md`). Card over-asking is in the backlog for M4.
2. 14: **done**: `docs/plans/M3-report.md`, ROADMAP updated, branch `docs/m3-report` (PR #76, base `feat/m3-evals`).
3. **Done 2026-11-12:** the owner accepted M3; #60–#76 merged in order with merge commits; `main` CI green at `9339c36`. Next: the M4 plan (`docs/plans/M4-plan.md`, PR on `docs/m4-plan`) awaits the owner's review and answers to Q1–Q4. Build nothing in M4 before approval.

M2 eval run 5 is done and recorded in `EVALS.md`: 51/54 valid, traps 27/27, labels 90.5%. Its
last 6 sessions hit the usage limit and are excluded. Lesson: check the account's remaining
usage before a paid run.

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
