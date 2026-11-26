# HANDOFF

> **2026-11-26: M5 Multi-service is built (tasks 00–15 merged); its exit criterion EC-1 is not
> yet met. Full eval 4: 5 of 9 runs valid (`rename-now` 3 of 3, `users-v2-profile` 2 of 3,
> `phone-e164` 0 of 3); the three Athel bugs behind the failures are fixed in the PR from
> `docs/m5-full-4` (ADR 040). The M5 report §7 asks the owner: re-run the two failed cases
> (recommended), a full run 5, or accept now.** See "M5 progress" at the end.

Last updated: 2026-11-20, session 5 (M4 built; first full evals run and missed; fixes in a new PR), on the owner's personal machine. Tools
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
| full-run fixes: no-code repos, round-limit card, dispute card length, mutant timeout, `spec remove`, foreground agents (ADR 029) | `feat/m4-full-run-fixes` | see `gh pr list` | open |
| 17 M4 report | `docs/m4-report` | — | after the owner decides on a re-run |

CI passed on #77–#107.

**Full evals, 2026-11-19 (owner's "Both full runs"; $110.31, M4 eval total $172.26):** both
missed. Reviewer 87.5% caught (bar 90%); build 2 of 9 runs valid. Details and causes are in
`EVALS.md` and journal entry 17. Five Athel gaps found in the failed build sessions were fixed
with tests on `feat/m4-full-run-fixes` (ADR 029).

Owner, 2026-11-21: "Keep 87.5%, fix cases"; "Yes, re-run both"; merges "Wait for the re-run".
The fixes are PR #108 (on #107). D11 and D04 are changed for the next run on
`feat/m4-defect-cases` (PR on #108), with an offline check that every case commits through
Athel's hooks.

Next:
1. When CI is green on #108 and the cases PR, run full evals 2 (both, on the cases branch):
   with the commands in `EVALS.md` (M4 section). Keep the workspaces (`ATHEL_EVALS_KEEP`) of
   the build runs only: full 1 kept all 99 and filled the disk. Record the rows in `EVALS.md`
   and the history files.
2. Then ask the owner for the merge OK for #77 → the cases PR, in order.
3. Task 17: the M4 report from the draft (it states every run plainly), ROADMAP.
4. Not fixed and reported: in `saved-addresses` the reviewer and security keep a major on
   ownership checks the approved spec allowed, until the round limit. With the scripted
   developer taking the recommended "stop", those runs stay invalid.

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

## Update 2026-11-21 (late): full evals round 2
- Build round 2: 2 of 9 valid, $45.44 (`tests/evals/history/m4-build-2026-11-21T103945Z.json`).
  - saved-addresses 1: create without edit, fixed in #110 (ADR 030). Run 3: build blocked with
    no card, fixed in #110 (ADR 031). Run 2: model missed the side finding.
  - typed-client-errors 3 and event-name-field 1–3: the account's usage limit (HTTP 429,
    "usage_limit_reached") stopped the sessions at start; not Athel. They must be re-run.
- Reviewer round 2 was still running at 70/90 (1 miss) when the session hit its limit; check
  `tests/evals/history/` for its file, or re-run it.
- Owner: up to 2 more eval rounds without asking (decisions-from-chat). Round 2 counts as the
  approved re-run; rounds 3 and 4 remain. Next: run both evals on #110 once the usage limit resets.
- Reviewer round 2 finished: **91.7% caught (66/72; bar 90%)**, false alarms on 5 of 18 clean
  runs, $31.93 (`m4-defects-2026-11-21T103945Z.json`). Late runs that hit the usage limit
  saved no round and count as misses, so the rate is a lower bound. EC-2 met on this run.

## Round 4 (2026-11-22)
Build full 4: 4 of 9 valid, $66.80 (see EVALS.md). No Athel bug found in the failed runs; the
3 `event-name-field` runs hit the usage limit. Both pre-approved rounds are used: ask the owner
before any more paid runs. Next: report to the owner, ask for the merge OK for #77–#110 in
order, then task 17 (M4 report, draft in the session scratchpad).

## Round 5 (2026-11-23)
Owner approved re-running only `event-name-field` (3 runs): 2 of 3 valid, $14.79. Combined with
round 4: 6 of 9 valid; EC-1 (every run valid) not met. The 3 failures are 2 missed planted side
bugs (model) and 1 round-limit stop (checker scope creep, backlog). Owner's merge OK: merge
#77–#111 in order after this re-run, checks green.

## Fix pass (2026-11-23)
- #77–#111 merged into main in order (owner's OK). M4 report: PR #112 (open).
- Owner: "One fix pass + 9 runs". Fix pass: PR #113 `feat/m4-review-minors` (ADR 033), check.sh
  green. Build full 6 (all 9 runs, $50–70 approved) running on that branch.
- Next: record build full 6 in EVALS.md; if every run is valid, update the M4 report (#112) to
  EC-1 met and ask the owner to accept M4 and merge #112 and #113; if not, read the failed runs
  (free), report, and ask before any more paid runs.

## Build eval 7 (2026-11-23)
Build full 7 on #113 (ADR 034): 4 of 9 valid, $43.28. Every run that finished before a network
outage (from about 11:00 local, DNS `ENOTFOUND` on the API) was valid, including
`saved-addresses` 1–3, which failed in every earlier round. The 5 failed runs never got far
enough to test Athel. Next: ask the owner whether to re-run `typed-client-errors` and
`event-name-field` (6 runs, about $45–50) on the same code, to complete 9 runs on one commit.
If all 6 are valid, update the M4 report (#112) to EC-1 met and ask to accept M4 and merge #112
and #113.

## Build eval 8 (2026-11-23)
Owner: "Re-run the 6". Build full 8 (`typed-client-errors` and `event-name-field` × 3, same code
as eval 7): 6 of 6 valid, $38.14. With eval 7's `saved-addresses` 1–3: **9 of 9 valid on #113's
code; EC-1 met.** M4 eval spend: $524.19. M4 report (#112) updated to both criteria met. Next:
the owner accepts M4 and merges #113, then #112 (or says what else is needed). After the merges,
update ROADMAP and HANDOFF on main and plan M5.

## M4 closed (2026-11-23)
Owner: "you decide, just merge all needs to be merged". M4 accepted. #113 (ADR 033, 034, evals
6–8) and #112 (M4 report, ROADMAP) merged into `main` with merge commits. Owner standing
instruction: merging finished work with green checks is Claude's call. Next: M5 plan.

## M5 plan drafted (2026-11-23)
`docs/plans/M5-plan.md` on `docs/m5-plan`, **approved 2026-11-23** with Q1–Q3 as
recommended (budget up to about $550, each run asked first; create the change label; live check
and dogfooding asked again before each). The owner's bar: multi-service is Athel's main selling point;
it must be as efficient per repo as one repo (EC-2) and "game changing". Next: task 01 (`feat/m5-fixture-system`).

## M5 progress (2026-11-24)
Plan approved (#114). Tasks, one PR each, merged with merge commits once CI is green (owner:
merging finished green work is Claude's call):

| Task | PR | Status |
|---|---|---|
| 00 plan | #114 | merged |
| 01 fixture system layer, system check, fake GitHub labels and human merge | #115 | merged |
| 02 change@5, change set derived at approval (ADR 035) | #116 | merged |
| 03 multi-repo plan checks, merge order in the summary | #117 | merged |
| 04 coordinated-release card (ADR 037) | #118 | merged |
| 05 every repo preflighted first; build park | #119 | merged |
| 06 upstream patch in manifests | #120 | merged |
| 07 linked PRs, change label, ready together | #121 | merged |
| 08 merge-order rehearsal, athel set verify (ADR 036) | #122 | merged |
| 09 athel set status, out-of-order alarm | #123 | merged |
| 10 per-repo test patterns; JVM, .NET, Ruby test naming | #124 | merged |
| 11 scout and architect command tables; classifier naming fixes | #125 | merged |
| 12 /athel:followup, the deferred cleanup draft | #126 | merged |
| 13 skills and agent prompts for sets | #127 | merged |
| 14a system check in the gateway repo; rehearsal caches untouched repos | #128 | merged |
| 14b scripted 3-repo journey (TestThreeRepoJourney) | #129 | merged |
| 15 M5 eval harness (tests/evals/m5/set) | #130 | merged |
| smoke 1 fixes (test artifacts; deferred removal in the plan) | #131 | merged |
| smoke 2: valid, $18.50 | #132 | merged |
| full 1 fixes: repos never cloned (ADR 038); rename case on Go services | #133 | merged |
| full 2 (owner: re-run 3 × 3) | #134 | void: session limit, $4.62 |
| full 3 (owner: start it now) | `fix/m5-full-3` | 3 of 9 valid, $108.84; four Athel bugs fixed (ADR 039), harness `govulncheck` fix |
| 16 report | `fix/m5-full-3` | merged (#135) |
| full 3 fixes (ADR 039) | #135 | merged, 46f2e85 |
| full 4 (owner: "a") | `docs/m5-full-4` | 5 of 9 valid, $146.50; three Athel bugs fixed (ADR 040); report updated |

Next: merge the `docs/m5-full-4` PR once CI is green (fixes, ADR 040, the full-4 history, the
report). Then ask the owner the M5 report's §7 question: re-run `users-v2-profile` and
`phone-e164` 2 × 3 (about $95–125, recommended), a full run 5 (about $120–150), or accept now.
M5 eval spend: $326.94 of about $550. Kept workspaces of full 4 are in the session scratchpad
(not in the repo). After M5: M5.5 Node and TypeScript gates (owner: before any pilot).

Learned in full 4: this machine's Go 1.27.3 has GO-2026-6859 (`net/http`, fixed in 1.27.4).
Upgrading Go is the owner's call (CLAUDE.md: ask before upgrading tools); Athel no longer
charges it to changes.
