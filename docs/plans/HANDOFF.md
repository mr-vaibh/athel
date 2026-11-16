# HANDOFF

Last updated: 2026-11-14, session 4 (M3 merged; M4 approved and started), on the owner's personal machine. Tools
are installed system-wide with Homebrew (`docs/context/environment.md`).

## Where we are

- **M3 Specify: done and merged** (PRs #60–#76, 2026-11-12; `main` CI green at `9339c36`). Exit
  criteria met: 9/9 eval runs valid, blast recall 100% (`docs/plans/M3-report.md`).
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

M4 Build is **approved** (2026-11-14, `docs/plans/M4-plan.md`, with Q1–Q4 answered: budget OK,
PostgreSQL yes, `govulncheck` limited to `vuln.go.dev`, and the live GitHub check and dogfooding
yes in principle, asked again before each). It is built as stacked PRs, one per task of
M4-plan §9:

| Task | Branch | PR | Status |
|---|---|---|---|
| plan | `docs/m4-plan` | #77 (base main) | open |
| 00a buildable Go fixture repos | `feat/m4-fixture-build` | #78 | open, gate green |
| 00b fake GitHub PR store and call log | `feat/m4-fixture-prs` | #79 | open, gate green |
| 01 worktrees, `change@4`, `OwnBranches` | `feat/m4-worktrees` | #80 | open, gate green |
| 02 git preflight, overlap card, `building` guard | `feat/m4-preflight` | #81 | open, gate green |
| 03 commit, branch and title lint; pre-commit checks | `feat/m4-git-lint` | #82 | open, gate green |
| 04a `athel git push` | `feat/m4-push-pr` | #83 | open, gate green |
| 04b `athel pr body|open|update`, ADR 026 | `feat/m4-pr` | #84 | open, gate green (`pr ready` moves to task 10) |
| 05 gates: defaults, classifier-checked runner (ADR 023), baseline, flaky, evidence | `feat/m4-gates` | #85 | open, gate green |
| 06a changed-lines coverage and complexity gates (ADR 025) | `feat/m4-gates-quality` | #86 | open, gate green |
| 06b contract and `ac-tests` gates, per-repo `gates.yaml` overrides | `feat/m4-gates-contract` | #87 | open, gate green |
| 07a built-in Go mutation gate (HIGH and CRITICAL risk) | `feat/m4-gates-mutation` | #88 | open, gate green |
| 07b security-scan gate | `feat/m4-gates-security` | #89 | open, gate green |
| 07c performance gate (latency budget in `risk_notes`) | `feat/m4-gates-perf` | #90 | open, gate green |
| 07d migration gate (static checks; up, down, up on a throwaway PostgreSQL) | `feat/m4-gates-migration` | #91 | open, gate green |
| 08 review rounds: `review@1`, carry-forward, limits, disputes, `review` gate | `feat/m4-review` | #92 | open, gate green |
| 09 waivers: `/athel:waive`, reason, expiry (security-scan refused, G18) | `feat/m4-waivers` | #93 | open, gate green |
| 10a `cli` coverage push: 90.2% → 92.0%; every command refuses cleanly | `feat/m4-cli-coverage` | #94 | open, gate green |
| 10b build engine: task order, manifests, `build done` | `feat/m4-build-engine` | #95 | open, gate green |
| 10c `ready_for_pr` guard, `pr ready`, Stop hook on unfinished tasks | `feat/m4-ship-guard` | #96 | open, gate green |
| 11a side findings: `finding@1`, ranking, `/athel:triage`, PR-body section | `feat/m4-findings` | #97 | open, gate green |
| 11b `finding fix`: a child change `CHG-NNN-F<n>` on `side_finding_branch` | `feat/m4-findings-fix` | #98 | open, gate green |
| 12a build, QA and security reports; their findings join the review round | `feat/m4-agents-policy` | #99 | open, gate green |
| 12b per-agent write scopes in the engine (ADR 024) | `feat/m4-agents-scopes` | #100 | open, gate green |
| 12c developer, QA, security and reviewer agents; rule IDs; prompt sync | `feat/m4-agents-prompts` | #101 | open, gate green |
| 13a `build done` needs both reports per round; `ready_for_pr` checks blast drift; resume names the task | `feat/m4-skills-cli` | #102 | open, gate green |
| 13b skills `/athel:build`, `/athel:ship`; status and resume updated | `feat/m4-skills` | #103 | open, gate green |
| 14 network-deny sandbox test | `feat/m4-sandbox-net` | #104 | open, gate green |
| 15 reviewer defect eval: corpus (24+6), setup, runner, grader; **paid run not made** | `feat/m4-evals-defects` | #105 | open, gate green |
| 16 build eval: 3 PRDs, full-journey runner, grader re-runs gates; **paid run not made** | `feat/m4-evals-build` | #106 | open, gate green |
| trial fixes: finding locations, spec-valid corpus, tasks without code (ADR 027), gate timeout 20m | `feat/m4-eval-trial-fixes` | #107 | open, gate green |
| 17 M4 report | `docs/m4-report` | — | after the paid runs |

CI passed on #77–#100 (checked 2026-11-15).

Next:
1. **Paid runs, each only with the owner's go-ahead.** Trials so far cost $29.58 (results in
   `EVALS.md`):
   - defects trial 2: 2/2 caught;
   - build trial 4: every task done and pushed, the size card and the side finding worked, but
     no PR, because `gh` lost its own settings in the CLI's scrubbed environment. Fixed in #107.

   Ask the owner for the next build trial (about $10–12), then the full runs.
2. Task 17: the M4 report (EC-1 and EC-2 results, carried gates), HANDOFF, ROADMAP; then the
   owner's merge OK for #77 to #106 in order.

The `cli` package's tests take about 7 minutes.

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
