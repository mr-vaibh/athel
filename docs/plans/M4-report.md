# M4 Build: report

Date: 2026-11-23 (session 5). Plan: `docs/plans/M4-plan.md`, approved 2026-11-14. Environment:
macOS, Apple M3 Pro, Go 1.27.3, golangci-lint 2.15.0, Claude Code 2.1.311.

**Verdict: M4 is built and both exit criteria are met: EC-1 with 9 of 9 build runs valid on one commit (PR #113), EC-2 with 91.7% of seeded defects caught (bar: 90%). The owner decides whether to accept M4 (§7).**

## 1. Exit criteria

| # | Criterion | Proof | Result |
|---|---|---|---|
| EC-1 | One-repo eval changes reach a PR with all gates green | `tests/evals/m4/build` takes 3 PRDs through real `claude -p` sessions: start, spec, the developer's `/athel:decide` (recommended options), plan, `/athel:signoff` at CRITICAL, `/athel:approve`, `/athel:build`, `/athel:ship` and `/athel:triage`. A run is valid only if every deterministic check passes: `in_review`; a PR opened as a draft, then marked ready; the grader re-runs gofmt, vet, build, tests and lint (new errors only) on a fresh clone of the pushed branch; commit lint is clean; the PR is within the size limit in force; no merge anywhere; the developer's clones are unchanged; the planted nearby bug is a side finding and is not edited | **Met.** 9 of 9 valid on PR #113's code (ADR 033 and 034): `saved-addresses` 1–3 from build full 7; `typed-client-errors` 1–3 and `event-name-field` 1–3 from build full 8. Build full 7's other 5 runs were lost to a network outage (no API access) and were re-run on the same code with the owner's OK. Every valid run filed the planted nearby bug as a side finding |
| EC-2 | The reviewer catches 90% of seeded defects | `tests/evals/m4/defects`: 24 diffs with one planted defect each, plus 6 clean diffs. The reviewer runs from the build loop's manifest, with the security agent too for security defects. A defect is caught by a blocker or major finding on its planted lines that cites one of its rules | **Met: 91.7%** (66/72 over 3 runs; bar 90%), full run 2 on 2026-11-21. Full run 1 measured 87.5% and stays on record; two cases were corrected between the runs with the owner's OK (§3) |
| Carried | M0–M3 gates | `scripts/check.sh` (vet, lint, race tests, coverage ≥ 90% per package), red-team and allow-list suites, scenarios, hook latency, the network-deny sandbox test (new) | Lint 0 issues. Every package ≥ 90% (lowest: `lock` 90.0%, `defects` 90.2%, `evalkit` 90.3%, `cli` 90.9%). Red-team **572/572** deny with the expected rule (M3: 505). The allow-list passes. CI is green on every PR |

## 2. Eval runs

Details for every run are in `docs/plans/EVALS.md` (M4 section). Raw results are in
`tests/evals/history/m4-*.json`.

| Run | Result | Cost |
|---|---|---|
| Trials (2 defects, 9 build), 2026-11-15 to 19 | each found one gap, fixed with a test (§3); build trial 9 valid | $61.95 |
| Defects full 1 | 87.5% (63/72), not met | $32.12 |
| Build full 1 | 2 of 9 valid | $78.19 |
| Defects full 2 | **91.7% (66/72), met** | $31.93 |
| Build full 2 | 2 of 9 valid (4 runs lost to the account's usage limit) | $45.44 |
| Build full 3 | 2 of 9 valid (6 runs lost to the usage limit) | $35.70 |
| Build full 4 | 4 of 9 valid (3 runs lost to the usage limit) | $66.80 |
| Build full 5 (`event-name-field` only) | 2 of 3 valid | $14.79 |
| Build full 6 (after ADR 033) | 6 of 9 valid | $75.85 |
| Build full 7 (after ADR 034) | 4 of 9 valid (5 runs lost to a network outage) | $43.28 |
| Build full 8 (the 2 PRDs hit by the outage, same code) | **6 of 6 valid; 9 of 9 with build full 7's `saved-addresses`: met** | $38.14 |
| **Total** | | **$524.19** |

Runs lost to the usage limit (HTTP 429) or the network outage count as not valid in the raw
results; they say nothing about Athel and are reported as such.

## 3. What the trials found, and what was done

Every trial was run only with the owner's go-ahead. Each found one gap a real user would hit.
Each gap was fixed with a test before the next trial.

| Found by | Problem | Fix (tested) |
|---|---|---|
| defects trial 1 | The reviewer cited diff positions as file lines, so a correct finding pointed at lines that do not exist | `review save`, `qa save` and `security save` refuse locations outside the file. The prompts say to use the file's own lines |
| defects trial 1 | A case's AC was not observable, so setup failed | Every case's spec is validated offline by the CLI |
| defects trial 1 | False alarms on a clean, package-level diff | The eval's conventions state that new packages are wired later and that the gateway authenticates |
| build trial 1 | A plan task with no code (a check or a ticket comment) covered an AC, and could never pass its gates | ADR 027: every AC needs a code task; a no-code task is done at once; its actions go to the developer in the PR body |
| build trial 2 | The session ran the checkers in the background and ended its turn to wait, 3 times, so the Stop hook blocked the change | `/athel:build` runs every agent in the foreground |
| build trial 3 | A PR over 400 lines (mostly QA tests) was refused with no way on | ADR 028: an oversized PR raises a card (allow up to N lines, or change the plan). The build resumes after the answer. QA writes lean, table-row cases |
| build trial 4 | `gh` ran with a scrubbed environment that dropped its own settings, so `pr open` could not reach GitHub. A real `GH_TOKEN` login fails the same way | gh keeps `GH_TOKEN`, `GITHUB_TOKEN`, `GH_HOST`, `GH_CONFIG_DIR` and the enterprise tokens, passed to gh only |
| build trial 5 | A build session hit the runner's 20-minute cap, and its lock then held off later sessions (harness) | Build sessions get 3× the cap. A killed session runs its SessionEnd hook |
| build trial 6 | `pr update` was building-only, so `/athel:ship` could not refresh the body | `pr update` is allowed until review; pushes stay building-only |
| build trial 7 | Subagent shells cannot `cd`, so QA's `go test` aimed at no planned repo and rule 7 denied it | The classifier treats `go -C`, `npm --prefix` and `yarn --cwd` as the command's folder. The prompts use `go -C <worktree>` |
| build trial 8 | At CRITICAL risk, approval needs `/athel:signoff`, which the scripted developer never typed (harness) | The runner signs off at CRITICAL, as the plan step tells a developer to |
| build trial 9 | The grader held the PR to 400 lines although the developer had allowed 600 | The grader uses the limit in force (ADR 028, FR-GIT-009) |
| defects full 1 | D11 planted a hard-coded key that Athel's own pre-commit check refused, so it never reached the reviewer; D04's missing test could only be found in the test file | With the owner's OK: D11 logs the request headers instead; D04 counts a finding in `refunds_test.go`. An offline check now commits every case through Athel's hooks before any paid run |
| build full 1 | A repo with no code tasks still got a worktree and guards; the round-limit card came too late and said too little; long card text broke the schema; a slow mutant hung the gate; a service dropped by an answer kept its spec | ADR 029: repos without code are skipped; the card is raised when the last round is saved, names the open findings, and "allow more rounds" adds 5 (up to 50); card text is clipped fairly; 1-minute mutant timeout; `athel spec remove`; all delegating skills run agents in the foreground |
| build full 2 | A task allowed `create` but not `edit`, so review fixes to its own new files were denied | ADR 030: plan lint requires `edit` with `create`; the template has it |
| build full 2 | A build session ended blocked on a reason with no card, so the developer had nothing to answer | ADR 031: the build step ends blocked only on an open card; the developer agent disputes findings the spec rules out |
| build full 3 | The size card recommended "split into smaller PRs", which Athel (one PR per repo) cannot do | ADR 032: it recommends raising the limit; "re-plan" goes back to planning. Several PRs per repo is in the backlog |
| build full 4 and 5 | The reviewer held tasks open over minor points until the round limit; agents read the planted bug but nobody filed it | ADR 033: `review save` refuses a verdict that disagrees with the findings (minors and nits never block; doc 06), open minors are listed in the PR body, and the scout and architect record side findings (doc 07) |
| build full 6 | The identical-failure card counted 3 gate runs on one commit; a new test file was planned without `create`; the security checker re-reported a risk the spec accepted, each round | ADR 034: attempts are counted per commit; `plan set` checks new files against the base branch; later rounds of security and QA check only the fixes and the new diff |

The cases' expectations and the pass bars never changed after a result was seen. The only
grader change that affects a verdict is the size limit above. It follows the spec's "the limit
can be raised", and it is reported, not hidden.

## 4. What M4 delivered

PRs #77–#111, stacked, one per task (`M4-plan.md` §9), plus the trial and full-run fixes (#107–#110) and the eval records (#111), merged in order on 2026-11-23 with the owner's OK:
- **Git:**
  - worktrees in Athel's own cache clones (the developer's clones are never touched);
  - preflight with overlap cards;
  - branch, commit and PR-title lint with trailers;
  - pre-commit secret and size checks;
  - pushes and PRs only through the CLI (ADR 026);
  - draft PRs until every gate is green;
  - the PR size limit with a split proposal and a card (ADR 028). Athel never merges.
- **Gates (ADR 023, 025):**
  - format, lint, build, types and unit, with a baseline so failures already on base never
    count, and flaky tests detected;
  - changed-lines coverage, complexity, contract and AC tests;
  - a built-in Go mutator;
  - security scan (gosec, govulncheck), performance budgets;
  - migrations up, down and up on a throwaway PostgreSQL;
  - the review gate.
  - Every gate command goes through the policy engine.
- **Review:** rounds with carry-forward, a reviewer-only verify that needs the fix commit to
  touch the finding, disputes, the 5-round limit, identical-failure cards, and waivers that
  only the developer can grant, with reason and expiry.
- **Roles (ADR 024):**
  - developer, QA, security and reviewer agents with their own prompts, templates and models;
  - per-role write rules and CLI command tables, enforced in the engine, with red-team cases;
  - QA and security reports join the review round;
  - 39 rule IDs from doc 06's quality standard, synced between prompt and CLI.
- **Build engine:** task order, manifests (the reviewer never sees the developer's notes),
  `build done`, the ready guard (every task done, pushed, PR open, no blast drift), the Stop
  hook on unfinished tasks, and resume inside a task.
- **Side findings:** ranked, triaged only by the developer, listed in the PR body, and a chosen
  fix becomes its own child change.
- **Skills:** `/athel:build` and `/athel:ship`; status shows tasks, gates and findings.
- **Network:** the CLI links no network code and runs a whole change under a network-deny
  sandbox.
- **Evals:** the build eval (3 PRDs, gates re-run by the grader) and the reviewer eval (24
  defects, 6 clean diffs).

## 5. Deviations and interpretations (all recorded)

1. **Spec gaps G1–G18** were decided in `M4-plan.md` §3 and approved with the plan.
2. **ADRs:**
   - 023: gate commands through the classifier;
   - 024: per-agent write scopes;
   - 025: changed-lines coverage and the built-in Go mutator;
   - 026: pushes and PRs only through the CLI;
   - 027: tasks without code;
   - 028: the PR size card;
   - 029: corrections from full eval 1;
   - 030: `create` needs `edit`;
   - 031: the build blocks only on a card;
   - 032: the size card recommends raising the limit;
   - 033: minors and nits never hold a task (doc 06 severity table and round rule 4);
   - 034: attempts per commit, new files need `create`, settled risks are not findings.
3. **Rule IDs:** doc 06 names one rule ID (`errors.no-silent-catch`). The other 38 follow its
   form, from the quality standard and the review checklist.
4. **The scripted developer always picks the recommended option**, as in M3.
5. **The build eval's grader re-runs** format, vet, build, unit and lint (new errors only). The
   deeper gates (coverage, mutation, contract) are trusted from Athel's own gate run on the
   pushed head; the CLI's tests cover them.

## 6. Not built, or still open

- **Checker scope creep** (seen in build full 4): ADR 033 and 034 bound it; the backlog note
  stays for a structural fix.
- **The usage limit** cut off 13 build runs across rounds 2–4, and a network outage 5 in round 7. Long evals need an account with
  room, or a smaller case set per day.
- **By plan, these come later:**
  - performing a plan's `env_actions` (backlog, M5/M6);
  - per-repo test patterns for QA;
  - command tables for the scout and the architect;
  - security-owner waivers with a verified identity (G18).

## 7. Decision for the owner

Both exit criteria are met. The owner chose option (b) of the earlier draft (one fix pass, then
a full run); it took two fix passes (ADR 033, ADR 034) and the re-run of 6 runs lost to the
outage. To close M4:

1. Merge PR #113 (the fixes ADR 033 and 034, with their eval records).
2. Merge this PR (#112, the report and ROADMAP).
3. Accept M4; M5 planning starts next.
