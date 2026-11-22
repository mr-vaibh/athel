# M4 Build: plan

Status: **Approved by the owner on 2026-11-14** ("Approve, budget OK"), with Q2 PostgreSQL yes, Q3 `govulncheck` limited to `vuln.go.dev` yes, and Q4 yes in principle, asked again before each live action.

Sources:
- **Doc 06** (Build Loop, Review and Quality) governs M4 and was read in full.
- **Doc 04 §Git and multi-repo**, §Git policy, preflight and conventions: FR-GIT-001 to 007,
  009 and 010 (008 is M5).
- **Doc 07 §Side findings**: FR-FIND-001 to 004.
- **Doc 02**: the change folder layout (`preflight/`, `reviews/`, `gates/`, `findings.yaml`),
  the failure policy table, and the Stop and SubagentStop hooks.
- **Doc 08**: the M4 row, the fixture system ("Teammate PR overlapping a planned path"), the eval
  harness ("defects that reviewers must catch (seeded)"), and dogfooding with writes from M4.
- **Doc 01**: features F-11 to F-14 and F-16.

"§" means a section of the named doc. "G" items are spec gaps I resolved (§3). "Q" items are
questions for the owner (§11).

> **What M4 ships, in plain words.** After M3, Athel turns a feature request into an approved
> plan. After M4, it **writes the code**, one task at a time, the way a careful senior engineer
> would:
> - before touching anything, it checks whether a teammate is already working on the same
>   files;
> - it works only in its own copy of the repo, on its own branch, never in yours;
> - a developer agent writes the code and its tests in the repo's own style;
> - three other agents that did not write it check it: QA adds the nasty test cases, security
>   looks for vulnerabilities, and a reviewer judges it like a strict senior reviewer;
> - automatic checks (build, tests, lint, coverage and more) must all pass, and every review
>   finding is tracked until it is fixed or a human waives it;
> - if it gets stuck, it stops and asks instead of burning money;
> - it opens a **draft** pull request, marks it ready only when everything is green, and
>   **never merges**;
> - problems it notices outside the task are written down, never quietly fixed.

## 1. Exit criteria (doc 08, Milestones table, M4 row)

| # | Criterion | How it is proven |
|---|---|---|
| EC-1 | One-repo eval changes reach a PR with all gates green | `tests/evals/m4/build`: 3 one-repo PRDs on buildable fixture repos (§4.10), each run as real Claude Code sessions: start, spec, decide, plan, the developer's `/athel:approve`, `/athel:build`, `/athel:ship`. A run is **valid** only if every check passes, all deterministic: the change reaches `in_review`; the fake GitHub holds a PR that was opened as a draft and then marked ready; the grader **re-runs every gate itself** on the pushed head and all are green; every commit passes the commit lint; the PR is within the size limit; no merge call appears in the audit log or the fake GitHub's log; and the developer's own clones are byte-for-byte unchanged (FR-GIT-001, also closing M2's unbuilt FR-DISC-016 check). Bar: every run valid |
| EC-2 | Reviewer catches 90% of seeded defects | `tests/evals/m4/defects`: about 24 prepared diffs on the fixture repos, each with one planted defect from doc 06's review checklist and code quality standard (silent catch, missing error-path test, wrong status code, broken contract, SQL injection, PII in logs, race, N+1, missing authz, and others), plus 6 clean diffs. The reviewer runs in a fresh context (and the security agent too, for security defects), exactly as in the build loop. A defect is **caught** when a blocker or major finding's location overlaps the planted lines and its rule is in the defect's category. Catch rate = caught ÷ planted, over all runs; bar ≥ 90%. Blocker or major findings on clean diffs are reported (false alarm rate), not gated |
| Carried | M0–M3 gates | `scripts/check.sh`, red-team and allow-list suites (new git, build and agent cases), resume and guardrails scenarios, hook latency in CI |

## 2. Deliverables mapped to requirements

| Req | Requirement (short) | How M4 meets it |
|---|---|---|
| FR-GIT-001 | All work in Athel's own worktrees and branches | Worktrees `workspace/repos/<repo>@<CHG>` are cut from Athel's cache clone (M2), never from the developer's clone. Engine rules 7 and 10 already expect this layout. A scenario test checksums every developer clone before and after |
| FR-GIT-002 | Preflight lists others' active work on planned paths | `athel git preflight`: fetch, base head, then open PRs, branches pushed in the last 14 days, commits to base in the last 7 days, and tickets mentioning the service, overlapped with the plan's paths at file level. The report goes to `preflight/<repo>.md` |
| FR-GIT-003 | Overlap with someone else's work → card, no commit | An overlap raises an A4 card (wait, coordinate with a drafted message, or narrow the plan). `building` cannot be entered while it is open |
| FR-GIT-004 | Never push protected or base branches, never force-push, rewrite or delete foreign branches | Exists since M1 (rule 9, red-team git set). M4 fills `OwnBranches` from `change.yaml`, and pushes go only through the CLI (G15) |
| FR-GIT-005 | Branch names, commits and PR titles follow `git.yaml` | `athel git lint`: conventional format, the `Athel-Change` and `Athel-Task` trailers, at most 15 files per commit. It is checked when the agent runs `git commit` (PreToolUse) and again before every push |
| FR-GIT-006 | Before a push: foreign commits block; rebase only if never pushed | `athel git push`: re-fetch; stop and raise a card if anyone else pushed to the branch; rebase on base if never pushed, otherwise merge base in; run the pre-commit secret and size check; then push |
| FR-GIT-007 | Draft PR until gates pass; Athel never merges | `athel pr open --draft` after the first task is done, `athel pr update` after each task, and `athel pr ready` only when every gate is green. The merge commands stay denied (red-team) |
| FR-GIT-009 | Size limit per PR (400 lines, without generated files and lockfiles) | `athel pr open` and `athel pr update` count changed lines with `git.yaml`'s exclusions. An oversized diff blocks with a split proposal: the plan tasks grouped into PRs under the limit. The limit can be raised at plan approval |
| FR-GIT-010 | Base conflicts resolved only in files the change owns | `athel git push` merges or rebases. A conflict in a file outside every task's paths aborts the merge and raises a card |
| FR-BUILD-001 | Tasks in dependency order | `athel build next` returns only tasks whose dependencies are done. A scenario log shows topological order |
| FR-BUILD-002 | Developer gets exactly its inputs | `athel build context CHG T-n` writes a manifest: the task, its REQs and ACs, `conventions.md`, the decisions it cites, lessons (empty until M6, G10), the files in its paths, and open findings. The developer agent is given only this manifest |
| FR-BUILD-003 | AC tests in the repo's own style, carrying the AC ID | The developer's instructions require the AC ID in the test name or a tag. The `ac-tests` gate checks that every AC the task covers appears in at least one changed test. The reviewer checks the style |
| FR-BUILD-004 | QA adds negative, boundary and failure tests | The QA agent writes only under test paths (G3). `qa-report@1` lists the cases added per category |
| FR-BUILD-005 | Security review on every diff and dependency change | The security agent writes `security-report@1` every round; the `security-scan` gate runs the scanners (G6) |
| FR-BUILD-006 | Reviewer in a fresh context, without the developer's reasoning | `athel review context` builds the reviewer's manifest from the diff, the spec slice, the conventions and the previous rounds only. A test checks that no developer transcript or build note is in it |
| FR-BUILD-007 | Findings with ID, severity, location, rule and fix; closed only by a verified fix or a waiver | `review@1` round files `reviews/<task>/round-N.yaml`. Only the reviewer can move a finding to `verified`. `done` is refused while a blocker or major finding is open |
| FR-BUILD-008 | Re-review builds on the last round | Round N+1 must reference every open finding of round N. The CLI refuses a round that drops one |
| FR-BUILD-009 | Every gate in `gates.yaml` passes before `done` | `athel gate run` evaluates deterministically. The Stop hook blocks the build step while the current task is neither done nor blocked |
| FR-BUILD-010 | 5 rounds, or 3 identical failures, then a card | Counters in the task's build record. The limit raises an A6 card. A test proves it never loops |
| FR-BUILD-011 | Edits stay in the task's paths; elsewhere → plan change request | Out-of-path edits are denied (rule 7, exists). `athel plan change-request` records the need and returns the change to `planned`; approval is voided as today |
| FR-BUILD-012 | Small commits with trailers | The commit lint (FR-GIT-005) |
| FR-BUILD-013 | Side findings recorded, never fixed in the task | `athel finding add` (`finding@1`). A fixture with a nearby planted bug must yield a finding and no edit |
| FR-BUILD-014 | Waivers need a human, a reason and an expiry, and appear in the PR body | Only the developer's typed `/athel:waive` can create one (human-origin grant, ADR 019 pattern). The CLI refuses a waiver without a reason or expiry, and refuses one for a never-waivable gate |
| FR-FIND-001 | Finding fields | `finding@1`: location, evidence, category, severity, effort, fix risk, suggested fix, disposition |
| FR-FIND-002 | Never fixed inside the change | Rule 7 plus the reviewer's spec-fit check. The grader checks that no diff touches paths outside the plan |
| FR-FIND-003 | Ranked at PR time; the human chooses | `/athel:ship` presents them ranked (security first, then severity, then quick wins). Only the developer's typed `/athel:triage CHG SF-1=fix_pr SF-2=ticket …` sets dispositions |
| FR-FIND-004 | A chosen fix is its own mini-change | `athel finding fix` creates a child change linked to the parent (M3's `--parent`), with a one-task plan, the same gates and review, and its own branch from `side_finding_branch` |

## 3. Spec gaps and interpretations (my decisions; tell me if you disagree)

| # | Gap | Decision |
|---|---|---|
| G1 | The fixture's repos have no tests, build files or lint config, so nothing can be built or gated | Task 01 makes the Go repos (`users`, `orders`, `gateway`) real small services with tests, `gofmt`, `go vet` and a `golangci-lint` config. The TS repos get tests on Node's built-in runner only, because installing npm packages would need network access that CLAUDE.md does not allow. **Build evals use the Go repos.** TS gates work through the repo's own `package.json` scripts and are unit-tested with fake commands |
| G2 | Gates are "commands run by the helper CLI", but the CLI must never become a way around the policy engine | Every gate command is classified and decided by the **same engine** as an agent's Bash call, under the change's mode and plan, before it runs. A denied gate command fails as `E-POLICY`. Gates run in the task's worktree with a scrubbed environment and a timeout. ADR 023 |
| G3 | Doc 06 gives each role a tool list ("QA: write only under test paths; security and reviewer: no edit") but not how to enforce it | Per-agent rules in the engine, beside the existing scout and architect rules. Developer: writes inside its task's paths (rule 7). QA: writes only to test files inside those paths (the test patterns come from `conventions.md` and the language defaults). Security and reviewer: no writes. Red-team cases for each. ADR 024 |
| G4 | The migration gate runs "up and down on a disposable database", is never waivable, and the fixture has no database (M2 Q9) | The migration gate runs static online-safety checks always, and the up/down run against a throwaway local PostgreSQL. Without PostgreSQL the gate is **unavailable**, so a migration task cannot finish. The build evals avoid migrations until Q2 is answered |
| G5 | Mutation testing on HIGH-risk tasks needs a mutation tool | A small built-in Go mutator (standard library `go/ast`) mutates only changed lines (`==`/`!=`, `<`/`<=`, `+`/`-`, `true`/`false`, dropped `if err != nil` returns) and runs the package's tests. For other languages the gate is **unavailable**, so a human must waive it, as doc 06 says for a missing tool. ADR 025, with changed-lines coverage |
| G6 | `security-scan` needs SAST, a dependency audit and a secret scan | SAST: `gosec` inside the installed `golangci-lint`. Secrets: M1's scrubber patterns over the diff. Dependency audit: `govulncheck` (Q3) runs only when a dependency changes; without it, such a task needs the security owner's waiver |
| G7 | PRs need a Git host; evals cannot open real PRs | Evals use the fixture's local bare repos as remotes, and the fake `gh` gains a PR store (`pr create`, `ready`, `view`, `list`, `diff`) with a log of every call. A live check on a real throwaway GitHub repo is Q4, since it is outward-facing |
| G8 | Doc 07: tickets for side findings are "drafted, then created after approval (W2)" | M4 drafts the ticket into the finding. Creating it goes through `tools.yaml` as a W2 call, so it asks the human. In headless mode it is never created |
| G9 | Doc 02 says `plan.yaml` holds "status per task", but any change to `plan.yaml` voids its hash-bound approval (M1) | Task status lives in `change.yaml` (`build.tasks[]`: status, round, attempts, commits). The approved plan file never changes during the build |
| G10 | The developer's inputs include lessons (FR-BUILD-002); lessons are M6 | The manifest has a lessons section, empty until M6 |
| G11 | M3 carried three items here: completed tasks keep their state when the PRD changes (M3 G9); "conventions.md is cited by the developer agent" and "both blast reports exist for every change that reached PR" (M3 G11) | All three land in M4: the plan diff keeps unaffected done tasks; `build-result@1` requires the conventions it followed; `/athel:ship` refuses a PR without a planned and an actual report, and runs `athel blast check` (FR-BLAST-005) |
| G12 | Human review comments on an open PR: the lifecycle has no `in_review → building` (backlog since M0) | Still out of M4 (doc 02 has no such arrow). It stays in the backlog for M6, where UAT rounds return a change to `specified` |
| G13 | Doc 02: "after 3 blocked stops the step moves to blocked and raises a decision card" | `ShouldBlockStop` (M0) gets the real gate status. The third block moves the step to `blocked` and raises an A6 card |
| G14 | The cyclomatic complexity and function length gate needs a tool | Go: computed with `go/ast` in the CLI (no dependency). Other languages: the repo's own linter where configured, otherwise unavailable |
| G15 | FR-GIT-006 needs checks before every push (re-fetch, foreign commits, rebase vs merge), which a raw `git push` by an agent cannot guarantee | In change mode, agents' `git push`, `gh pr create`, `gh pr ready` and `gh pr edit` are denied with a next action naming the `athel` command that does it safely. This narrows M1's rule 9 and never widens anything. ADR 026 |
| G17 | Doc 04 preflight step 3 also lists "in-progress tickets mentioning the service", and step 4 asks for symbol-level overlap "when the language tooling allows" | *As built (task 02):* the CLI collects what it can read without a model: open PRs (`gh`), branches pushed in the last 14 days and commits to base in the last 7 days (cache clone). Ticket search needs the scout's tools and judgement, so the `/athel:build` skill asks the scout for in-progress tickets before the preflight, and a ticket on planned paths becomes the same A4 card. Overlap is at file level; symbol level is in the backlog. Other Athel changes' branches are excluded from the cache fetch (task 01), so they are seen through their open PRs only |
| G16 | "Changed-lines coverage ≥ 90%" needs per-line data | Go: `go test -coverprofile` joined with `git diff -U0`. Other languages: an lcov file if the repo's scripts produce one, otherwise unavailable |

## 4. Design

### 4.1 Flow, who does what

1. **`/athel:approve`** (exists): hash-bound plan approval → `plan_approved`.
2. **`/athel:build CHG`** (new skill):
   - **Preflight**, once per repo: `athel git preflight`. Overlaps raise a card and stop. Then
     the worktree and branch → `building`.
   - **Per task**, in order (`athel build next`):
     1. `athel state enter build --task T-n` (task lock, checkpoint).
     2. Round 1 only: `athel gate baseline` on the untouched worktree.
     3. The **developer** subagent gets the manifest, writes the tests and the code, and
        commits in small steps.
     4. `athel gate run --stage cheap` (format, lint, build, types, unit, `ac-tests`). If any
        is red, the findings go back to the developer, and the round counter moves.
     5. The **QA** subagent adds tests; the **security** and **reviewer** subagents review,
        each from its own manifest, in parallel. They save `qa-report@1`, `security-report@1`
        and the review round.
     6. `athel gate run --stage full` (QA tests, security scan, contract, coverage,
        complexity, mutation and performance when they apply, review).
     7. All green: the task is done and its commits are pushed (`athel git push`); the draft
        PR is opened or updated. Otherwise the next round, up to the limits.
   - The **Stop hook** refuses to end the step while the current task is neither done nor
     blocked (with M0's loop protection).
3. **`/athel:ship CHG`** (new skill): the actual blast radius from the diff, `athel blast check`
   against the plan, side findings presented ranked, and `athel pr ready` when everything is
   green → `ready_for_pr` → `in_review`.
4. **Developer-only commands**, each a human-origin grant like `/athel:decide`:
   - `/athel:waive CHG <gate> --reason … --until …`;
   - `/athel:triage CHG SF-1=…`;
   - `/athel:decide` for the build cards (round limit, dispute, overlap, conflict).

### 4.2 Git: worktrees, preflight, commits, pushes and PRs (FR-GIT)

- **New package `internal/gitw`**, writes only inside Athel's workspace. `gitro` (M2) stays
  read-only. It covers worktree add and remove, branch creation by `branch_name`, fetch,
  merge, rebase, push and conflict detection.
- **`change@4`**: `repos[]` with name, base, base head, branch, worktree, pushed (yes/no), PR,
  and `build.tasks[]` (G9). `athel migrate` upgrades `change@3`.
- **`OwnBranches`** comes from `change.yaml` (`repos[].branch`).
- **A branch that already exists remotely** is reused only if every commit on it carries this
  change's trailer; otherwise a suffix is added (doc 04 edge case).
- **The pre-commit check** blocks files over 1 MB and secret patterns (M1 scrubber).
- **The PR body** follows doc 04's sections, generated from the change's files: spec link, PRD
  lines, file groups, blast summary, decisions, gate evidence, security notes, side findings,
  linked PRs, and rollback.

### 4.3 Gates (FR-BUILD-009, doc 06 §Quality gates)

- **`gates@1`** defaults are embedded in the CLI and can be overridden per repo in
  `.athel/config/gates.yaml`. Each gate has its command (from `conventions.md` recipes or the
  language default), pass criterion, waivable-by value, and stage (cheap or full).
- **Evidence:** `gates/<task>.yaml` (`gate-result@1`) holds each gate's command, exit code,
  duration, parsed counts, the baseline it was compared with, and an output file under
  `artifacts/`.
- **Baseline rules** (doc 06 edge cases): tests already failing on base never fail the task
  but become side findings. Skipped or deleted tests compared with the baseline fail `unit`. A
  test that flips is re-run up to 3 times and marked flaky only if it also flakes on base.
- **Unavailable gate:** the tool is missing and no repo script exists. The gate is recorded as
  unavailable and blocks unless a human waives it, when it is waivable.

### 4.4 Review rounds and findings (FR-BUILD-007/008/010)

- **`review@1`** follows doc 06's finding format: IDs `R<round>-<n>` given by the CLI; source
  reviewer, security, qa or gate; status open, fixed, verified, waived or disputed; and
  history.
- **The developer** marks a finding fixed with its commit, or disputed with a reason. **Only
  the reviewer** can mark it verified.
- **The CLI refuses:**
  - a round that does not carry forward every open finding;
  - `done` with an open blocker or major finding;
  - a sixth round (it raises an A6 card instead);
  - a third identical gate failure in a row (card);
  - a second round of dispute on the same finding (a card with both arguments).

### 4.5 Role agents (doc 06 §Role agents)

- **New agents:** `developer`, `qa`, `security`, `reviewer`. The `architect` exists and is
  called for a change request or a design question.
- **Model tiers come from config:**
  - the developer uses the strongest model;
  - the reviewer and security agents use the strongest too, with a different system prompt;
  - `models.reviewer` can name a different model (doc 06 independence rule).
- **Templates** for `build-result@1`, `qa-report@1`, `security-report@1`, the review round and
  `finding@1`. Each uses an unrelated example domain and is tested against its schema, the
  injection check and the leak guard (M3 lessons).
- **The developer's rules** are doc 06's eight, verbatim in substance. The reviewer's checklist
  and the code quality standard's rule IDs (for example `errors.no-silent-catch`) are a table
  kept equal to the CLI's rule list by a prompt-sync test.

### 4.6 Side findings (FR-FIND, FR-BUILD-013)

- **`finding@1`** goes in `findings.yaml`, with IDs `SF-n` from the CLI.
- **Ranking:** security first, then severity, then small effort and low fix risk. "Quick win"
  means effort S and fix risk low.
- **Presented once, at ship.** Dispositions are set only through `/athel:triage`.
- **`fix_pr`** creates a child change through `athel finding fix`, with ID `CHG-NNN-F<n>`, a
  one-task plan, the normal flow and `side_finding_branch`.

### 4.7 Waivers (FR-BUILD-014)

- **`/athel:waive`** creates a grant; `athel gate waive` records the waiver in `change.yaml`
  (`waivers[]`: gate, task, by, reason, until, grant).
- **Refused:**
  - a never-waivable gate (build, unit, types, format, contract, migration, review);
  - a missing reason;
  - an expiry in the past or more than 30 days ahead;
  - `security-scan` waived by anyone other than the security owner named in
    `config` (`security_owner`, default the CODEOWNERS entry for `/`).
- **Expired waivers** stop counting.
- **The PR body** lists every waiver.

### 4.8 Edge cases (doc 06 and doc 04 tables)

Each case is a scenario or unit test:
- failing tests on base;
- a flaky test;
- a repo with no test command (the plan needs a harness task, otherwise a card);
- a missing gate tool;
- a reviewer–developer dispute;
- a fix that breaks another gate;
- a hand-edited generated file (refused when a generator config exists; the generator input is
  reviewed);
- a large refactor (plan change request);
- coverage dropping because dead code was deleted (changed-lines coverage);
- a latency budget in `risk_notes` (performance gate);
- a dependency upgrade (separate task, security review);
- a teammate pushes to Athel's branch (card, never overwrite);
- base moves during the build;
- a remote branch name collision;
- a file over 1 MB or a secret in a commit;
- lockfiles inflating the PR size.

### 4.9 Lessons from M2 and M3 built in from the start

- Templates for every document an agent writes, with errors that name the fix (M3
  `draftError`).
- The leak guard covers the new agents, skills and templates.
- Prompt-sync tests keep the rule tables equal to the CLI's.
- Agents never get "investigate more" or "wait" as dead-end options (M3 full run 1).
- Run folders are kept, smoke runs come first, and a full run starts only from a frozen
  worktree.
- Every agent's turn cap is sized from a measured smoke run.

### 4.10 Eval design

- **Build evals (EC-1):** 3 one-repo PRDs on the Go fixture repos:
  - a small endpoint feature;
  - a validation fix with error paths;
  - an event-field addition (an expand task).

  Each has an expected outcome: the expected task kinds, the ACs that must pass, and one nearby
  planted bug that must become a side finding and not be edited.

  The runner scripts the developer's prompts as in M3: `/athel:decide` with the recommended
  options, `/athel:approve`, and `/athel:triage` with every finding set to ticket. It never
  answers anything else.

  The grader re-runs the gates itself, so a self-reported green is never trusted.
- **Defect evals (EC-2):** the defect list is a YAML file of diffs on the fixture repos, each
  with the planted line range and the category's rule IDs.

  The reviewer and security prompts are the real plugin's. The runner calls them through the
  same `athel review context` manifests the build loop uses.

  **The defects stay out of the plugin:** the leak guard covers the defect corpus's
  identifiers.

## 5. Packages

| Package | Purpose |
|---|---|
| `internal/gitw` | Worktrees, branches, push, merge and rebase inside Athel's workspace |
| `internal/preflight` | Active-work collection and overlap |
| `internal/gitlint` | Branch, commit and PR title lint, trailers and the pre-commit check |
| `internal/gates` | `gates@1`, runner, baseline, evidence, waivers |
| `internal/gates/coverage` | Go profile and lcov, joined with the diff |
| `internal/gates/mutate` | The Go mutator |
| `internal/gates/complexity` | Go cyclomatic complexity and function length |
| `internal/review` | Rounds, findings, limits, disputes |
| `internal/build` | Scheduler, task records, manifests |
| `internal/findings` | Side findings, ranking, triage, child changes |
| `internal/pr` | PR body, size count and split proposal, the host adapter (GitHub through `gh`) |

## 6. Data and files (doc 02 layout)

```
workspace/repos/<repo>@CHG-NNN/          worktree (FR-GIT-001)
.athel/changes/CHG-NNN/
  preflight/<repo>.md                    preflight report
  build/<task>/manifest.yaml             developer context manifest
  build/<task>/result-N.yaml             build-result@1 per round
  reviews/<task>/round-N.yaml            review@1
  reviews/<task>/qa-N.yaml, security-N.yaml
  gates/<task>.yaml                      gate-result@1, with outputs in artifacts/
  findings.yaml                          finding@1
  pr/<repo>.md                           rendered PR body
```

New schemas:
- `change@4`;
- `gates@1` and `gate-result@1`;
- `build-result@1`, `qa-report@1`, `security-report@1` and `review@1`;
- `finding@1`;
- `preflight@1`.

Each lands with its first user, with migrations where a released schema changes (M3 practice).

## 7. CLI additions

- **Git:** `git preflight`, `git push`, `git lint`.
- **Pull requests:** `pr open --draft`, `pr update`, `pr ready`, `pr body`.
- **Build:** `build next`, `build context`, `build result save`, `build done`.
- **Gates:** `gate baseline`, `gate run --stage`, `gate waive`, `gate show`, `gates template`.
- **Review:** `review context`, `review save`, `qa save`, `security save`.
- **Side findings:** `finding add`, `finding list`, `finding present`, `finding triage`,
  `finding fix`.
- **Plan:** `plan change-request`.
- **Templates:** one for each new agent document.

Every command has `--help` and `--json`.

## 8. Dependencies

None planned: standard library only (`go/ast`, `go/parser`, `os/exec`). `golangci-lint` is
already a required tool. `govulncheck` (Q3) and PostgreSQL (Q2) are tools, not Go dependencies,
and only with the owner's yes.

## 9. Ordered task list (one PR each, about 400 changed lines; tests named up front)

| # | Branch | What | Key tests |
|---|---|---|---|
| 00 | `feat/m4-fixture-build` | Buildable Go fixture repos with tests and lint config; TS tests on Node's runner; a failing-on-base test, a flaky test, a generated folder, a planted nearby bug; the fake GitHub PR store and call log; bare remotes; a teammate push | `TestFixtureReposBuildAndTest`, `TestFakePRStore` |
| 01 | `feat/m4-worktrees` | `gitw`, `change@4` with migration, worktree and branch, `OwnBranches` wiring, the developer-clone checksum scenario | `TestWorktreeFromCacheNotClone`, `TestDeveloperCloneUnchanged` |
| 02 | `feat/m4-preflight` | Preflight, overlap, report, A4 card, the `plan_approved → building` guard | `TestTeammatePROnPlannedPathRaisesCard` |
| 03 | `feat/m4-git-lint` | Commit, branch and title lint, trailers, max files, 1 MB and secret pre-commit checks, the PreToolUse check on `git commit` | `TestCommitLint`, `TestSecretBlocksCommit` |
| 04 | `feat/m4-push-pr` | `git push` (foreign commits, rebase vs merge, conflicts in unowned files), PR open, update and ready, size limit and split proposal, PR body; ADR 026 with red-team cases | `TestForeignCommitBlocksPush`, `TestOversizedDiffProposesSplit`, `TestNoMergeEver` |
| 05 | `feat/m4-gates` | `gates@1`, the policy-checked runner (ADR 023), stages, baseline, flaky handling, evidence, unavailable gates | `TestGateCommandGoesThroughPolicy`, `TestBaselineFailuresNotTaskFailures` |
| 06 | `feat/m4-gates-quality` | Changed-lines coverage, complexity, contract (from `blast diff`), `ac-tests`; ADR 025 | `TestChangedLinesCoverage`, `TestEveryACHasATest` |
| 07 | `feat/m4-gates-deep` | Go mutator, performance gate, migration gate (static checks, plus the DB run if Q2 is yes), `security-scan` | `TestMutationScoreOnChangedLines`, `TestSecretInDiffFailsScan` |
| 08 | `feat/m4-review` | `review@1` rounds, carry-forward, verified-only-by-reviewer, limits, disputes, identical failures | `TestRoundMustCarryOpenFindings`, `TestSixthRoundRaisesCard` |
| 09 | `feat/m4-waivers` | `/athel:waive` grant, waiver rules, expiry, security owner, PR body listing | `TestWaiverNeedsHumanReasonExpiry`, `TestNeverWaivableGates` |
| 10 | `feat/m4-build-engine` | Scheduler, task records, manifests (FR-BUILD-002/006), build step guards, `building → ready_for_pr`, the Stop hook on gate status, the M3 G9 carry | `TestTopologicalOrder`, `TestReviewerManifestHasNoTranscript`, `TestStopBlocksWhileGatesRed` |
| 11 | `feat/m4-findings` | `finding@1`, ranking, present, `/athel:triage` grant, child change, ticket draft | `TestFindingRanking`, `TestFixBecomesChildChange` |
| 12 | `feat/m4-agents-policy` | Developer, QA, security and reviewer agents; per-agent write rules (ADR 024); templates; prompt-sync; leak guard; red-team cases | `TestQAWritesOnlyTests`, `TestReviewerCannotWrite`, `TestRuleTablesMatchCLI` |
| 13 | `feat/m4-skills` | `/athel:build`, `/athel:ship`, `/athel:waive`, `/athel:triage`; status shows tasks, gates and findings; resume inside a task | `TestBuildSkillFollowsTheLoop`, `TestShipNeedsBothReports` |
| 14 | `feat/m4-sandbox-net` | Network-deny sandbox test for the CLI (backlog) | `TestCLIMakesNoNetworkCalls` |
| 15 | `feat/m4-evals-defects` | Defect corpus, reviewer eval runner and grader (EC-2) | `TestDefectGrader` |
| 16 | `feat/m4-evals-build` | Build eval PRDs, multi-prompt runner, the grader that re-runs gates (EC-1) | `TestBuildGraderRerunsGates` |
| 17 | `docs/m4-report` | M4 report, `EVALS.md`, HANDOFF, ROADMAP | — |

ADRs expected:
- 023: gate commands through the policy engine;
- 024: per-agent write scopes;
- 025: changed-lines coverage and the built-in Go mutator;
- 026: pushes and PRs only through the CLI in change mode.

## 10. Risks

| Risk | Mitigation |
|---|---|
| **Cost:** a build journey runs 4 agents over several rounds | Q1 budget. Smoke one PRD first. Cheap gates before agents. The round limit is 5 |
| **The CLI running repo commands is a new attack surface** | ADR 023: every gate command goes through the engine. The environment is scrubbed and there is a timeout. Red-team cases for a malicious `Makefile` or `package.json` script |
| **The reviewer marks its own earlier finding verified without checking** | Verified needs the fix commit to touch the finding's location. The grader re-runs gates. EC-2 measures the reviewer alone |
| **Seeded defects leak into prompts** | The leak guard covers the defect corpus. The defect diffs live only under `tests/` |
| **AI variance** | Deterministic CLI checks wherever possible. Regrade from saved runs. Every run must be valid for EC-1 |
| **Scope is the largest so far** (doc 08: 4 weeks) | 18 PRs, each about 400 lines, stacked as in M3, so each is reviewable alone |

## 11. Questions for the owner (only cost, tools and outward-facing actions; everything else is decided above, open to change)

| # | Question | Recommendation |
|---|---|---|
| Q1 | M4's evals are the most expensive so far. A **build run** (3 PRDs × 2 runs, full journey with 4 agents) is estimated at **$60–150**. A **defect run** (30 diffs × 3 runs, reviewer and security only) at **$25–45**. Plan for up to 3 build runs and 2 defect runs (about $250–550 in all), each still asked first? | Yes. Smoke one PRD and 5 defects first (about $15–25) |
| Q2 | Install **PostgreSQL** with Homebrew (`postgresql@17`), used only by tests through a throwaway data folder, so the migration gate can really run migrations up and down? Without it, any task with a migration can never finish (the gate is never waivable) | Yes. CI gets a PostgreSQL service container. Until then, the evals avoid migrations |
| Q3 | Install **`govulncheck`** (from `golang.org/x/vuln`, a Go module) and allow it to read the Go vulnerability database at `vuln.go.dev` (official Go, but not in CLAUDE.md's network list)? Without it, any task that changes a dependency needs the security owner's waiver | Yes, limited to `vuln.go.dev` |
| Q4 | After the build evals pass, one **live check on a real GitHub repository**: a throwaway private repo under your account, where Athel opens a draft PR and marks it ready (never merges)? Then, per doc 08, **dogfooding with writes**: one small change to Athel itself, with a PR you review | Yes to both, each asked again right before it happens |
