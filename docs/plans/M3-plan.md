# M3 Specify: plan

Status: **Approved by the owner on 2026-11-10** ("review your plan … if all looks good, then
start implementing"). The self-review before approval added the architect's read-only policy,
CLI templates, the fixture-first ordering, leak and prompt-sync guards, the M4/M6 scope notes,
and a split of the agent task. Q1 and Q2 (cost) are asked when each run is due.

Sources:
- **Doc 05** (Spec, Blast Radius and Planning) governs M3 and was read in full.
- **Doc 08**: the M3 row, the fixture system and eval harness, and the definition of done.
- **Doc 02**: plugin layout, the architect agent, the change folder layout, lifecycle states and
  the workflow engine.
- **Doc 04**: expand-and-contract ordering (FR-PLAN-003) and rule 7 (writes must match the
  plan).
- **Doc 01**: features F-07 to F-10 and F-21, and the asking metrics (5 or fewer cards per
  change, median 2; under 10% "should not have asked").

"§" means a section of the named doc. "G" items are spec gaps I resolved (§3). "Q" items are
questions for the owner (§11).

> **What M3 ships, in plain words.** Today Athel can answer questions about running systems
> (M2). After M3, given a PRD (a feature request), it works out what to build *before any code is
> written*:
> - it checks the PRD for gaps and contradictions;
> - it writes a spec per service, with every line traced back to the PRD;
> - it measures what the change could break (blast radius), from the spec and later from the
>   real diff;
> - it asks only the questions that matter, as decision cards with a recommendation;
> - it produces an ordered task plan that a human approves.
>
> It learns an existing repo's conventions first (brownfield onboarding). M3 still writes no
> product code: that is M4.

## 1. Exit criteria (doc 08, Milestones table, M3 row)

| # | Criterion | How M3 proves it |
|---|---|---|
| EC-1 | 3 eval PRDs produce valid specs, reports and plans | `tests/evals/m3/`: 3 PRDs on the `shop` fixture (§4.9), each run as real Claude Code sessions: `/athel:start`, `/athel:spec`, card answers through `/athel:decide`, then `/athel:plan`. "Valid" is checked deterministically. Specs, blast reports, decisions and plans validate against their schemas; traceability is 100% (FR-SPEC-003); every acceptance criterion passes the testability lint (FR-SPEC-005); plan coverage and ordering lints pass (FR-PLAN-002/003); the expected risk level is reached |
| EC-2 | Blast radius recall 95% on fixtures | Each eval PRD lists its expected affected items (services, consumers, data, people). Recall = expected items found in `blast-radius.md` ÷ expected items, over all runs; bar ≥ 95%. Precision and card recall and precision are reported too (doc 08 harness), not gated |
| Carried | M0–M2 gates stay green | 90% per package, hook p95 < 50 ms, red-team 100% blocked with 0 false allows (new cases for every new verb), audit verify, resume scenario; M2's open eval target is tracked in `docs/plans/EVALS.md` |

## 2. Deliverables mapped to requirements

Features: **F-07 spec-linked** (planned and actual blast radius), **F-08** PRD intake and spec,
**F-09** decision cards, **F-10** change plan, **F-21** brownfield onboarding.

| ID | Requirement (short) | M3 deliverable |
|---|---|---|
| FR-SPEC-001 | Completeness checklist; missing items listed | `prd-review@1`; the architect fills the checklist, and the CLI refuses a review missing any of the 8 items. Fixture PRD without acceptance criteria → gap named |
| FR-SPEC-002 | Contradictions and ambiguity → cards | The CLI pre-scans for vague words ("fast", "secure", "etc.", "should be easy"); the architect judges materiality. Material items become A1 cards; a fixture with two conflicting limits yields one card |
| FR-SPEC-003 | Stable PRD IDs; every requirement cites one | `athel prd index` assigns `PRD-<section>.<n>` IDs to headings and requirement-like sentences, deterministically. Spec save refuses a requirement without a valid PRD ID |
| FR-SPEC-004 | Spec per service, EARS style, `spec@1` | `spec@1` YAML block rendered to `spec/<service>.md` (same pattern as `investigation.md`). An EARS lint checks each requirement's form |
| FR-SPEC-005 | Every acceptance criterion testable | An AC lint requires Given/When/Then with an observable result; flagged criteria are refused |
| FR-SPEC-006 | Brownfield conventions before any spec | `athel repos survey` collects the inputs; the architect writes `conventions.md` and `map.md`, stored as facts with recipes (re-verified on build or lint config change, or after 30 days). Spec save in a repo without fresh conventions is refused |
| FR-BLAST-001 | Planned report after spec; actual report before each PR | `blast@1` with `mode: planned` (from the spec) and `mode: actual` (from `athel blast diff`). The pre-PR wiring lands in M4 (G2) |
| FR-BLAST-002/003/004 | Six sections, evidence and traffic per consumer, deterministic risk | Built in M2 (ad-hoc); M3 links findings to REQ IDs and adds the planned and actual modes |
| FR-BLAST-005 | Impact not in the approved plan blocks the PR | `athel blast check` compares actual with planned and exits `E-GATE-BLAST-DRIFT` on new impact. Tested with fixture branches; called by `ship` in M4 |
| FR-DEC-001 | Cards only when ask rules A1–A7 fire | `decision@1` requires `rule`. Decisions Athel takes alone are recorded with `decided_by: athel` and a reason |
| FR-DEC-002 | Card fields | Schema requires: question, context, 2–4 options with consequences, recommendation with reason, default, `default_safe`, blocks |
| FR-DEC-003 | Batched: one interruption per step | `athel decide present` records each presentation; a second one in the same step is refused unless it carries a new blocker |
| FR-DEC-004 | Answers recorded with who and when, then cited | Answers are accepted only from the human's own `/athel:decide` prompt (human-origin grant, as `/athel:approve`, ADR 017). Spec lines cite `[Decision D-n]`, and the trace checks that the IDs exist |
| FR-PLAN-001 | Task graph, schema, acyclic | Exists since M1 (`plan@1`, cycle check). `plan@2` adds a task `kind` for the ordering lint (G4) |
| FR-PLAN-002 | Coverage both ways | Plan lint: every AC is covered by a task, and every task covers an AC |
| FR-PLAN-003 | Expand-and-contract ordering | Ordering lint over task kinds and dependencies (doc 04 ordering rules 1–4) |
| FR-PLAN-004 | Approval bound to the plan's hash | Exists since M1 (`/athel:approve`, hash-bound; an edit voids approval). M3 adds the one-screen summary and the plan diff |
| Doc 05 gates | HIGH → rollback task; CRITICAL → human sign-off of the report | Plan lint requires a `rollback` on risky tasks when risk is HIGH or above. `/athel:signoff CHG-NNN` is human-origin, like approve |
| Edge cases | One-liner PRD, huge PRD, conflicts, renamed service, ambiguous owner, internal-only change, no runtime data, free-text answers, unanswered cards, human plan edits, PRD changes | §4.8 maps each to a behaviour and a test |

## 3. Spec gaps and interpretations (my decisions; tell me if you disagree)

| # | Gap | Decision |
|---|---|---|
| G1 | Doc 08's harness describes 12 eval PRDs and rubric grading by a separate model; the M3 row asks for 3 eval PRDs producing *valid* outputs | M3 builds 3 PRDs, graded by deterministic checks only. The other 9 PRDs and the model rubric for spec quality come with the full harness (M6); added to the backlog |
| G2 | "Blast radius (planned and actual)" is in M3, but the actual report needs code diffs, which M4 produces | M3 builds the actual report and the planned-versus-actual check, tested on fixture branches with prepared diffs. Blocking a real PR is wired into `ship` in M4 |
| G3 | Doc 05 shows a spec as Markdown, yet says it "validates against `spec@1`" | Same pattern as `investigation.md`: the architect passes a YAML block, the CLI validates it and renders Markdown with the block embedded. One source of truth, and humans read Markdown |
| G4 | The ordering lint (FR-PLAN-003) needs to know whether a task is a migration, a provider addition, a consumer update or a removal; `plan@1` has no such field | New `plan@2` with a required `kind` (`expand`, `migrate`, `consume`, `contract`, `internal`); `athel migrate` upgrades `plan@1` (default `internal`). `plan@1` shipped in M1, so ADR 002's rule applies: version bump plus migration |
| G5 | "Cards are answered by the human", but the model runs the CLI | Answers count only from the human's typed `/athel:decide CHG-NNN D-1=b …` (UserPromptSubmit grant, ADR 017 pattern). The architect cannot answer its own cards. Free-text answers are allowed and flagged for the architect's conflict re-check (doc 05 edge case) |
| G6 | `default_safe` applies after 4 hours; a reminder goes out after 2 | Time comes from the injected clock. SessionStart runs `athel decide tick`: it applies due safe defaults (logged, re-shown at plan approval) and reports reminders. Athel never runs in the background |
| G7 | Table size for the "migration on a table over 1M rows" rule | The fixture serves table row counts as a Postgres-exporter metric through the fake Grafana (`pg_stat_user_tables_n_live_tup`), as a real setup would; no real 1M-row database (M2 Q9 stands) |
| G8 | Huge PRDs split into several changes "with a parent record" | `change new --parent CHG-NNN` and a `children` list, read by `/athel:status`. The architect proposes the split as an A1 card. The minimal version is in M3; a test covers it |
| G9 | "PRD changes mid-build: unaffected completed tasks keep their state" | M3 builds `athel prd add-version`, the spec diff and the plan diff. Keeping the state of completed tasks needs M4's build states and lands there |
| G10 | Architect model tier is "strongest" (doc 02) | `models.architect` in `config@3`, default the strongest model. The evals use the same setting (cost: Q1) |
| G11 | Two acceptance tests need M4 actors: "`conventions.md` … is cited by the developer agent" (FR-SPEC-006) and "both reports exist for every change that reached PR" (FR-BLAST-001) | M3 proves the parts it owns: conventions exist, are fresh and are required before a spec; both report modes exist and are checked. The developer-agent citation and the per-PR check are M4 acceptance items, listed in the M4 plan |
| G12 | Ask rule A4 includes "conflicts with … a lesson"; lessons are M6 (doc 07) | A4 fires in M3 for conflicts with existing behaviour, policy or active work. The lesson part comes with M6 |
| G14 | Each service has its own spec, but plans cite acceptance criteria by bare ID (`AC-1`), so per-spec numbering would collide | REQ and AC numbers are unique across the change; `spec save` refuses a duplicate. Plans keep citing `AC-n` |
| G13 | Safety: the architect is a new subagent that reads code and writes Athel files | Same as the scout (ADR 015 follow-up): it is denied every file write and every direct read of `.athel/`. Its outputs go only through the CLI (stdin and arguments). Red-team cases cover writes, `.athel/` reads, answering its own cards and forging `--by human` |

## 4. Design

### 4.1 Flow (doc 05 §Intake steps), who does what

1. **`/athel:start`** (exists) creates the change and `prd/v1.md`. New: `athel prd index`
   writes `prd/v1.index.yaml` (ID → text and line) and is called by start.
2. **`/athel:spec`** runs the **architect** subagent:
   - **Onboarding:** for each repo in scope without fresh conventions, `athel repos survey`
     collects the inputs; the architect writes `conventions.md` and `map.md` through
     `athel conventions save`.
   - **Review:** the completeness checklist and the ambiguity scan go to
     `prd/review-v1.md` through `athel prd review save`. The CLI pre-scan supplies the vague
     words.
   - **Investigation:** the scout investigates the services named or implied (M2,
     `--change`).
   - **Decisions:** material gaps become cards (`athel decide add`); the rest become stated
     assumptions.
   - **Specs:** one per service through `athel spec save`, with the EARS lint, the AC lint and
     the PRD-ID check.
   - **Planned blast radius:** `athel blast save --mode planned`, with findings linked to REQ
     IDs.
   - **Transitions:** draft → investigating → specified; then awaiting_decisions when blocking
     cards are open.
3. **`/athel:decide`**:
   - shown to the human: the batched cards (`athel decide present`);
   - typed by the human: `/athel:decide CHG-NNN D-1=b D-2="…"`, which is human-origin.
4. **`/athel:plan`**: the architect writes `plan.yaml` (`plan@2`). `athel plan set` runs the
   coverage, ordering, size, rollback and env-action lints; `athel trace` writes
   `traceability.md` and fails on any missing link. The summary is one screen. → planned.
5. **`/athel:approve`** (exists): hash-bound. If the risk is CRITICAL, `/athel:signoff` comes
   first.

### 4.1b Lessons from M2 built in from the start

- **Templates.** `athel spec template`, `athel decide template` and `athel plan template` print
  valid examples with every allowed value. They use an unrelated example domain, and their
  examples are tested against the schemas and the injection check.
- **`--help`** on every new command (exists since M2).
- **Leak guard.** `TestPluginNeverHintsAtTheFixture` covers the architect, the new skills and
  the templates.
- **Prompt sync.** Tests keep the architect's tables (ask rules, EARS patterns, task kinds)
  equal to the CLI's.
- **Hints, not blocks, where the AI may be right:** for example, "AC-3 covers no REQ: link it
  or drop it".

### 4.2 PRD intake (FR-SPEC-001/002/003)

- **IDs.** Headings get `PRD-<n>`. A requirement-like sentence (contains shall, must, should,
  will, can, or is a list item under a heading) gets `PRD-<heading>.<k>`. Stable: re-indexing
  the same text gives the same IDs. A new PRD version keeps IDs for unchanged sentences (match
  by normalised text) and assigns new ones after the last.
- **Review.** `prd-review@1` has 8 checklist items (goal, actors, scope, out of scope,
  functional requirements, acceptance criteria, non-functional needs, constraints and
  dependencies), each `present | missing | partial` with the PRD IDs as evidence. It also holds
  ambiguities (PRD ID, phrase, material yes/no, card or assumption) and contradictions (two PRD
  IDs, card).

### 4.3 Specs (FR-SPEC-004/005, G3)

- **`spec@1`:** service, change, requirements (`REQ-n`, `prd: [PRD-…]`, EARS `text`,
  `decisions: [D-…]`), `interfaces_changed` (kind, name, change: `new|additive|behavioural|breaking`),
  `data_changes`, design notes, acceptance criteria (`AC-n`, `req: [REQ-…]`, given, when, then),
  out of scope, assumptions.
- **EARS lint:** each requirement matches one of the five EARS patterns (ubiquitous,
  event-driven "When", state-driven "While", unwanted "If … then", optional "Where") and names
  the service as subject ("the users service shall").
- **AC lint:** given, when and then are all present; "then" names something observable (a
  status code, a returned value, an emitted event, a stored value, a log or metric), not "works"
  or "is fast". Numbers carry units.

### 4.4 Brownfield onboarding (FR-SPEC-006, F-21)

- **Survey.** `athel repos survey <repo>` reads, from Athel's cache clone (never the developer's):
  build files, lint and format configs, CI config, the folder tree to depth 3, the 20 most
  recently changed files and the test file list. Output: `artifacts/survey-<repo>.json`.
- **Save.** `athel conventions save <repo>` stores `conventions.md` and `map.md` under
  `.athel/repos/<repo>/`, plus a fact with a recipe: the sha256 of each build or lint config
  file, freshness 30 days.
- **Use.** Spec save refuses a spec for a repo whose conventions are missing or stale.
- **New projects** (no repo yet): the architect drafts conventions from the PRD's stack and one
  card confirms them.

### 4.5 Blast radius, planned and actual (FR-BLAST-001/005, G2)

- **Planned:** M2's report plus `mode: planned`, `spec_refs` on each item, and the rules from
  `blast.yaml` (all of doc 05's rules exist since M2).
- **Change surface from a diff.** `athel blast diff --repo R --base B --head H` reads the git
  diff in the cache clone and maps it to a change surface: routes (Go `HandleFunc` and TS
  router patterns), exported symbols, event or message types, migrations (files under
  `migrations/`), config keys (env lookups), and dependency manifests. These are deterministic
  heuristics per language, starting with Go and TypeScript (the fixture's languages).
- **Check.** `athel blast check` compares the actual surface and consumers with the planned
  report. New items → `E-GATE-BLAST-DRIFT` naming each one.

### 4.6 Decision cards (FR-DEC-001…004, G5, G6)

- **Storage.** `decisions.yaml` (`decision@1` list) is written only through
  `athel decide add|present|answer|tick|feedback`.
- **Lint.**
  - A card needs 2–4 options, each with a consequence.
  - The recommendation must name one of the options.
  - The default must be an option key.
  - `blocks` may only name tasks or steps that exist.
- **Presentation.** `present` batches all open cards for the step, shortest question first, and
  records the step. A second presentation in the same step needs a card flagged as a new
  blocker (FR-DEC-003).
- **Feedback.** Each answered card can carry `should_not_have_asked: true`. `/athel:status`
  reports cards per change and the feedback rate (the doc 01 metrics).

### 4.7 Plan (FR-PLAN-001…004)

`plan@2` = `plan@1` + `kind` per task. Lints run in `athel plan set`:
- **Coverage both ways** (FR-PLAN-002).
- **Ordering** (FR-PLAN-003):
  - `expand` comes before the `consume` tasks that depend on it;
  - `contract` depends on every consumer and is marked deferred (doc 04 rule 3);
  - `migrate` follows add → backfill → switch reads → remove (doc 04 rule 4).
- **Size.** S, M or L; L needs a `size_reason`.
- **Tests stay with their change.** A task whose `kind` changes behaviour must list `test` in
  its verbs (planning rule 2).
- **Rollback.** At HIGH or CRITICAL risk, risky kinds (`migrate`, `expand`, `contract`) need a
  `rollback`.
- **Env actions** never include a deploy.

`athel plan summary` prints the one-screen summary: tasks per repo, gates, env actions, risk,
open assumptions, and estimated cost and time. The cost and time estimate is a simple
per-task-size model; its constants live in config. `athel plan diff` shows what changed since
the approved hash.

### 4.8 Edge cases (doc 05 table)

| Edge case | Behaviour, and where it is tested |
|---|---|
| One-liner PRD | Review marks most items missing. The spec carries explicit assumptions plus one batched card to confirm them. Eval PRD 3 |
| Huge PRD | The architect proposes a split card; `change new --parent` links the children (G8). Unit test plus skill text |
| PRD conflicts with code | An A4 card with options change / keep / adapt. Eval PRD 2 (current behaviour from the scout) |
| Service missing or renamed | The scout resolves aliases (M2); if unresolved, a card. Eval PRD 1 uses an alias (`user-svc`) |
| Ambiguous owner (duplicated data) | The blast report shows both with evidence; the card recommends the owner of the write path. Eval PRD 1 (the duplicated `phone` column) |
| Internal-only change | The consumers section must state "no contract change detected" with evidence; the schema enforces it. Unit test |
| No runtime data | The report says so, and the risk cannot be LOW (a new rule, `no-runtime-evidence`). Unit test |
| Free-text answer | Accepted and flagged; the architect re-checks it before the spec uses it. Skill and unit test |
| Never answered | `tick`: a reminder after 2 hours; a safe default applied after 4 hours and re-shown at approval; other cards stay blocked. Clock-injected tests |
| Human edits the plan | `plan set` re-runs every lint, and approval binds to the new hash (exists). Test |
| PRD changes | `prd add-version`, spec diff, plan diff (G9). Test |

### 4.9 Eval PRDs and grading (EC-1, EC-2)

The three PRDs are on `shop`, with expected outcomes written by hand from the fixture:

1. **Single service, brownfield:** "Customers can update their phone number; it must be stored
   in E.164." Expected:
   - changed: users;
   - consumers: notifications (`users.updated.v1`), orders (GET user);
   - data: the users table (row count from the metric);
   - people: the teammate PR #812 overlapping `api/user.go`;
   - risk HIGH (large-table migration);
   - cards: phone format (A1), migration of the duplicated column owner (A4/A2).
2. **Multi-service, breaking contract:** "Rename `phone` to `phone_number` in
   `users.updated`." Expected:
   - changed: users, notifications;
   - risk CRITICAL (breaking change with a consumer outside the set) or HIGH if the set
     includes all consumers;
   - plan ordering: expand → consume → deferred contract;
   - a card for a coordinated release.
3. **Ambiguous one-liner:** "Make payments more reliable." Expected:
   - the review lists missing acceptance criteria, scope and non-functional targets;
   - one batched card with assumptions (A1);
   - the blast report links the firing payments alert to runtime health;
   - risk CRITICAL (`critical-alert-firing`).

The harness reuses M2's sandbox (fixture, fake HOME, fake CLIs and MCP servers, regrade from
saved runs). It drives several prompts per session: start, spec, a scripted human answer to
the cards (the recommended options), then plan. Grading is deterministic:
- schema validity;
- traceability 100%;
- AC and EARS lints;
- plan coverage and ordering;
- expected risk level;
- blast recall and precision against the expected items;
- card recall and precision against the expected rules.

## 5. Packages

| Package | Responsibility |
|---|---|
| `cli/internal/prd` | ID assignment, version diff, ambiguity pre-scan, `prd-review@1` checks |
| `cli/internal/spec` | `spec@1` decode, EARS and AC lints, render and extract |
| `cli/internal/decisions` | `decision@1` store, lint, presentation log, answers, tick |
| `cli/internal/plan` | (exists) plus `plan@2`, coverage, ordering, size, rollback, summary, diff |
| `cli/internal/trace` | PRD → REQ → AC → task links, `traceability.md` |
| `cli/internal/blast` | (exists) plus planned/actual, diff surface, check |
| `cli/internal/conventions` | survey inputs, save, freshness recipe |
| `tests/evals/m3` | cases, grader, runner (reusing M2's sandbox code, moved to a shared package) |

## 6. Data and files (doc 02 layout)

New under `changes/CHG-NNN/`:
- `prd/v1.index.yaml`, `prd/review-v1.md`
- `spec/<service>.md`
- `decisions.yaml`
- `blast-radius.md` (planned) and `blast-actual-<repo>.md`
- `traceability.md`
- `plan.yaml` (`plan@2`)

New under `.athel/repos/<repo>/`: `conventions.md`, `map.md`, and the survey artifact.

New schemas: `prd-index@1`, `prd-review@1`, `spec@1`, `decision@1`, `plan@2`, `config@3`
(`models.architect`, plan estimate constants), `change@4` (parent and children, sign-off record).

## 7. CLI additions

| Command | Purpose |
|---|---|
| `prd index` | Assign PRD IDs |
| `prd review save` | Store the PRD review |
| `prd add-version` | Add a new PRD version |
| `prd diff` | Diff two PRD versions |
| `spec save`, `spec show`, `spec validate` | Store, read and check specs |
| `decide add`, `present`, `answer`, `tick`, `feedback`, `list` | Decision cards |
| `blast save --mode planned` | Planned blast report |
| `blast diff`, `blast check` | Actual change surface; planned-versus-actual check |
| `repos survey` | Collect onboarding inputs |
| `conventions save`, `conventions show` | Store and read conventions |
| `plan set` (more lints), `plan summary`, `plan diff` | Plan |
| `trace` | Write `traceability.md` |
| `change new --parent` | Split a PRD into child changes |

Every new verb gets red-team cases:
- forged `--by human` on `decide answer` and on sign-off;
- a subagent answering cards;
- writes into `decisions.yaml` or `spec/` bypassing the CLI.

## 8. Dependencies

None new. EARS and AC lints are regular expressions and word lists in Go. Diff parsing uses
`git diff` through the existing `gitro` package.

## 9. Ordered task list (one PR each, about 400 changed lines; tests named up front)

| # | Branch | What | Key tests |
|---|---|---|---|
| 00 | `feat/m3-eval-carry` | EVALS.md item 3: gateway-trap check. CLI flags a direct "calls" claim when the same block routes it via a gateway. Then a re-run of the M2 evals (Q2) | `TestHintDirectCallBesideRoute` |
| 01 | `feat/m3-schemas` | New formats `prd-index`, `prd-review`, `spec`, `decision`. *As built:* the version bumps land with their first user, so each PR carries its own migration: `plan@2` with task 07, `change@4` with tasks 04/09, `config@3` with task 11a | `TestM3SchemasValidExamples`, `TestM3SchemasRejectInvalid` |
| 02 | `feat/m3-prd` | `prd index`, `add-version`, `diff`, ambiguity pre-scan, review save | `TestPRDIDsStable`, `TestReviewNamesMissingAC`, `TestConflictingLimits` |
| 03 | `feat/m3-decisions` | Store, lint, present (batching), feedback | `TestCardLint`, `TestOneInterruptionPerStep` |
| 04 | `feat/m3-decide-human` | `/athel:decide` grant (ADR 019), answer, tick (reminder, safe default), red-team cases | `TestAnswerNeedsHumanGrant`, `TestSafeDefaultAfter4h` |
| 05 | `feat/m3-spec` | `spec@1` store and render, EARS lint, AC lint, PRD-ID check | `TestEARSLint`, `TestACNeedsObservableResult`, `TestSpecCitesPRD` |
| 06 | `feat/m3-conventions` | Survey, conventions save, freshness, spec gate | `TestSurveyFromCacheOnly`, `TestStaleConventionsBlockSpec` |
| 07 | `feat/m3-plan-lints` | `plan@2` lints: coverage, ordering, size, rollback, env actions; summary; diff | `TestCoverageBothWays`, `TestExpandContractOrder`, `TestRollbackAtHighRisk` |
| 08 | `feat/m3-trace` | `trace`, `traceability.md`, step guards in transitions | `TestTraceFailsOnMissingLink`, `TestSpecifiedNeedsTrace` |
| 08b | `feat/m3-fixture` | Table-row metric (G7), 3 PRDs and expected outcomes, prepared branches for the actual-blast tests (needed by 10) | `TestFixturePRDsExpectedItems`, `TestTableRowsMetric` |
| 09 | `feat/m3-blast-planned` | Planned mode, spec refs, `no-runtime-evidence` rule, `/athel:signoff` for CRITICAL | `TestPlannedReportLinksREQ`, `TestCriticalNeedsSignoff` |
| 10 | `feat/m3-blast-actual` | `blast diff` change surface (Go, TS), `blast check` | `TestDiffSurfaceGoRoutes`, `TestExtraConsumerBlocks` |
| 11a | `feat/m3-architect-policy` | Architect agent definition, read-only policy (G13), templates for spec, decide and plan; red-team cases | `TestArchitectCannotWrite`, `TestArchitectCannotAnswerCards`, `TestTemplatesValidAndClean` |
| 11b | `feat/m3-skills` | Skills `spec`, `decide`, `plan`, `signoff`; `/athel:status` card view; prompt-sync and leak tests | `TestArchitectTablesMatchCLI`, `TestPluginNeverHintsAtTheFixture` |
| 13 | `feat/m3-evals` | M3 cases, grader, multi-prompt runner, regrade; shared sandbox package | `TestGraderBlastRecall`, `TestM3Sandbox` |
| 14 | `docs/m3-report` | M3 report, EVALS.md, HANDOFF, ROADMAP | — |

ADRs expected:
- 019: human-origin decision answers and report sign-off;
- 020: `plan@2` and its ordering model;
- 021: diff change-surface heuristics.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Eval cost: longer sessions on the strongest model | Q1 budget; smoke-run one PRD first; stop early on harness bugs (M2 lesson) |
| Lints too strict for real PRDs (EARS, AC) | Lints name the exact problem and the fix; tested on the 3 PRDs plus doc 05's own example spec |
| Diff heuristics miss a language's patterns | Start with Go and TS (the fixture's languages); unknown files count as "unclassified change", which never lowers risk |
| AI variance (M2 lesson) | Deterministic CLI checks wherever possible, prompt-sync tests, regrade from saved runs, two consecutive runs for any 100% claim |
| Scope growth (huge PRDs, mid-build changes) | Minimal versions in M3 (G8, G9); full behaviour with M4/M6, in the backlog |

## 11. Questions for the owner (only cost; everything else is decided above, open to change)

| # | Question | Recommendation |
|---|---|---|
| Q1 | M3's eval runs are longer than M2's: each session writes specs and plans with the strongest model. Estimate **$25–55 per full run** (3 PRDs × 3 runs). Plan for up to 4 full runs (about $100–200), each still asked first? | Yes. Smoke-test one PRD first (about $5) |
| Q2 | Re-run the M2 evals once after task 00 (about $13) to measure the fixes still unmeasured? | Yes |
