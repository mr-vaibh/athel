# M5 Multi-service: plan

Status: **Approved by the owner on 2026-11-23** ("cool, go ahead with the plan please"), with Q1–Q3 as recommended: budget up to about $550, each run still asked first; the change label may be created; the live multi-repo check and dogfooding yes in principle, asked again right before each.

Sources:
- **Doc 04 §Multi-repo change sets** (change set record, ordering rules 1–5, cross-repo PR rules)
  and FR-GIT-008, read in full.
- **Doc 05** FR-PLAN-003, planning rule 3, the risk rules that count services and consumers
  "outside the change set", and §The change plan and approval.
- **Doc 01** F-17 and the end-to-end example (steps 6, 7 and 9: provider first, consumers next,
  removal later; preflight per repo; linked PRs with the merge order stated).
- **Doc 02**: `/athel:status` and the change folder layout; **doc 07** §Contract cleanup check
  (the deferred removal); **doc 08**: the M5 row, the fixture system and the eval harness
  ("4 multi-service" eval PRDs).
- What M2–M4 already built (checked in the code, §2).

"§" means a section of the named doc. "G" items are spec gaps I resolved (§3). "A" items go
beyond the spec (§4) and are part of what you approve. "Q" items are questions for you (§12).

> **What M5 ships, in plain words.** After M4, Athel builds a change well in each repo, but it
> treats the repos of one change as separate jobs. After M5, **a change across many services is
> one piece of work**, handled the way a staff engineer who knows the whole system would:
> - it works out which service must ship first, and plans the change so that **no order of
>   merging the stated way ever breaks production in between**: new things are added first,
>   callers move next, old things are removed only later;
> - when a service is changed to use another service's new API, the agent writing it **sees
>   the exact new contract** from the other repo's work, instead of guessing;
> - before marking anything ready, Athel **rehearses the merges in order** on its own copies,
>   and runs the checks after each step, so "every step in between works" is proven, not hoped;
> - every PR names the others, its place in the order ("2 of 3, merge after users#12") and
>   carries the same change label;
> - one status view shows the whole set: review, CI, what is merged, what is next, and an alarm
>   if someone merges out of order;
> - when safe ordering is impossible, it **says so and asks**, with a coordinated-release
>   proposal; it never ships a breaking change silently;
> - the removal of old fields is tracked, and Athel drafts that follow-up change when you say the
>   consumers are deployed;
> - and it does this **about as efficiently per repo as a one-repo change**: one investigation,
>   one spec, one set of questions for the whole change, not one per repo. This is measured.

## 1. Exit criteria

| # | Criterion | How it is proven |
|---|---|---|
| EC-1 (doc 08, M5 row) | A 3-service fixture change merges in order with every intermediate state compatible | `tests/evals/m5/set`: 3 multi-service PRDs on the Go fixture services (§5.9), run as real Claude Code sessions through the whole journey (start, spec, decide, plan, approve, build, ship, triage). A run is **valid** only if every deterministic check passes: the change is `in_review`; one PR per repo, each opened as a draft then marked ready, all with the change label; every PR body lists every other PR and its position, and the order matches the change set and doc 04's rules; **the grader, playing the human, merges the PRs in the stated order** in the fake GitHub and, after each merge, builds every service from main, runs its tests and runs the cross-service system check: all green at every step; the deferred removal is recorded and not built; Athel's own merge-order rehearsal (A1) agrees with the grader; the planted nearby bug is a side finding and not edited; no merge call by Athel anywhere; the developer's clones are unchanged. Bar: every run valid |
| EC-2 (owner, 2026-11-23: "as efficiently on multiple repos as on one") | A multi-service change costs about the same per repo as a one-repo change | Measured on the same runs. Bars: (a) cost per changed repo ≤ 1.25 × M4's average valid one-repo run ($7.69, from `m4-build-2026-11-23*.json`), so about $29 for 3 repos; (b) wall time per repo ≤ 1.25 × M4's average (20 min); (c) **no question asked twice**: no two cards in a change share a decision (the grader compares them); (d) one investigation and one spec session per change. Reported per run; the bar is the median over all runs, so one noisy run does not decide it |
| Carried | M0–M4 gates | `scripts/check.sh`, red-team and allow-list suites (new set, label and system-check cases), resume and guardrails scenarios, hook latency in CI, the M4 build eval as a regression check (3 runs, one per PRD) |

## 2. What exists and what is missing

| Already built | Where |
|---|---|
| Task kinds `expand`, `migrate`, `consume`, `contract`, `internal` and the ordering lint (consumers after an expand; the removal deferred, after every consumer and migration) | `plan@2`, `plan/lint.go` (M3, FR-PLAN-003) |
| Blast radius counts services changed and knows consumers "in the change set" (risk rules) | `blast` (M3) |
| A change can plan tasks in several repos; each repo gets its own worktree, branch, preflight and PR | `state.RepoWork`, `gitw`, `preflight`, `pr` (M4) |
| PR bodies have a "Linked PRs and merge order" section listing the other repos' PRs | `pr.Body` (M4) |
| `change.yaml` has a `change_set` field, untyped, always empty | `state/change.go` (M0 placeholder) |

| Missing (this milestone) | Requirement |
|---|---|
| A typed change set: roles, order, dependencies between PRs, deferred entries, strategy | doc 04 §Change set record, FR-GIT-008 |
| The PR's position ("2 of 3, merge after …"), the change label, all bodies refreshed when any PR opens | FR-GIT-008, doc 04 §Cross-repo PR rules |
| Proof that every intermediate state works | doc 04 invariant, doc 08 M5 exit |
| The coordinated release when compatibility is impossible | doc 04 ordering rule 5 |
| `/athel:status` for the whole set with each PR's review and CI state | doc 04 §Cross-repo PR rules |
| The deferred removal as a tracked follow-up | doc 04 rule 3, doc 07 §Contract cleanup check |
| End-to-end multi-repo runs: the M4 evals changed one repo each | doc 08 eval harness |

## 3. Spec gaps and interpretations (my decisions; tell me if you disagree)

| # | Gap | Decision |
|---|---|---|
| G1 | Doc 04 shows a `change_set` record but not who writes it. A model writing it could get the order wrong | **The CLI derives it** from the approved plan, never the model: a repo's role comes from its task kinds (`expand` or `migrate` → provider, `consume` → consumer, deferred `contract` → provider_cleanup, otherwise internal); its order is its depth in the graph of cross-repo task dependencies; `depends_on` lists the PRs of the repos it waits for. It is recomputed whenever the plan changes, and fills in PR numbers as they open. `change@5` types it, with a migration. ADR 035 |
| G2 | One PR per repo (M4) cannot be ordered when two repos depend on each other both ways (users needs orders first, and orders needs users first) | The plan lint refuses it: "users and orders wait on each other; one PR per repo cannot be ordered. Split the change so one side expands first, or raise a coordinated-release card." Several PRs per repo stays in the backlog (ADR 032) |
| G3 | Doc 04 rule 3: the removal is "a separate, later change, raised only after consumers are deployed everywhere (verified by traffic, doc 07)". Traffic checks are M6 | M5 records the deferred entry in the change set, shows it in every PR body and in status as "waiting: consumers deployed", and **never builds it** in this change. When the developer types `/athel:followup CHG`, Athel drafts the removal change (its PRD, linked to the parent) for the developer to start. M6 adds the traffic check before the draft is offered |
| G4 | "Every intermediate state compatible": the spec states the invariant but not how Athel shows it | Athel rehearses the merges (A1), and the eval's grader re-checks them independently. Athel itself never merges anything: the rehearsal happens in throwaway local clones and is never pushed |
| G5 | PR numbers exist only after each PR opens, but each body must list all the others | `pr open` for a repo refreshes the bodies of every other open PR in the set; `pr update` does the same. Until a sibling is opened, its line reads "users: not opened yet (1 of 3)" |
| G6 | "The same change ID label": GitHub refuses a label that the repo does not have | `pr open` adds the label `athel:CHG-NNN`, creating it in the repo first if it is missing, through the tools policy like the PR itself. This is an outward-facing write, so it is Q2 |
| G7 | When should a set's PRs be marked ready: each when its own gates pass, or together? | **Together**, only after every repo's gates are green and the rehearsal (A1) passed on the current heads. A ready PR whose sibling is still red could be merged too early |
| G8 | Humans merge (doc 01). What if they merge out of order? | Status compares merge times with the order. An out-of-order merge raises an alarm with what it breaks and what to do (for example "merge users#12 now; orders#8 calls an endpoint that main of users does not have yet"). Athel never merges, reverts or closes anything |
| G9 | Doc 04 rule 5: "if backward compatibility is impossible, the plan says so in a decision card with a coordinated-release proposal" | The plan lint refuses a breaking contract change (from blast radius) whose consumers are in the set unless there is an expand path, or an answered card choosing the coordinated release. Then `strategy: coordinated`: every PR says "merge together, in this order, in one deploy window", and the rehearsal checks the final state plus each repo's own gates, since in-between states are declared unsupported. ADR 037 |
| G10 | Doc 06 FR-BUILD-002 lists the developer's inputs per task, written for one repo. A consumer task in `orders` needs the contract that the provider task in `users` just built | The build manifest of a task gets an **upstream contract** section: the diff of the contract files (API handlers, schemas, event payloads, migrations) of every task it depends on in other repos, read from Athel's own worktrees. The reviewer's manifest gets the same, so a consumer is reviewed against the real provider, not against a guess (A2) |
| G11 | Doc 04 runs preflight "per repo, before the first write" | Preflight runs for **every repo in the set before the first write in any repo**, so a teammate's open PR in the third repo is found before Athel touches the first |
| G12 | One repo's task blocked on a card while the others can go on | Doc 08's decided rule applies: work continues on unblocked tasks, including other repos; `/athel:ship` waits for the whole set |
| G13 | The developer narrows the change by a card answer while some PRs are already open | The change set is recomputed; the dropped repo's PR body says "dropped from CHG-NNN" and stays a draft; closing it is the developer's call (Athel does not close PRs). Its worktree is removed |
| G14 | Base branches move while the set is in review | The rehearsal records the base heads and PR heads it ran on. Status marks it stale when any of them moves, and `/athel:ship` re-runs it before marking ready |
| G15 | Backlog items marked for M5: per-repo test-file patterns for QA (structured field in conventions), per-command limits for the scout and the architect | Both are built here (tasks 10 and 11). Performing `env_actions` stays M6; symbol-level overlap stays later |
| G16 | Doc 08 puts parallel worktrees under "Later" | Repos are built one task at a time, as in M4. EC-2's numbers will show whether parallel builds are worth bringing forward; that goes to the backlog with the data |

## 4. Beyond the spec (additions you approve with this plan)

These do not change any spec rule; they add checks the spec does not ask for, so that Athel
"thought of everything".

| # | Addition | Why |
|---|---|---|
| A1 | **Merge-order rehearsal**, `athel set verify CHG`: for each step of the order, in throwaway local clones it merges the PRs up to that step into their bases, then runs every affected repo's build and unit gates and the workspace's optional **system check** (a command in `.athel/config/system.yaml` that starts the services together and runs cross-service flows). Each gate command goes through the policy engine (ADR 023). The evidence goes to `changeset/verify.yaml`; without a system check configured, the PR bodies say so plainly. ADR 036 | The spec states the invariant; this proves it before a human merges anything |
| A2 | Upstream contract in the developer's and reviewer's manifests (G10) | The biggest cause of cross-repo bugs is a consumer written against the wrong idea of the provider |
| A3 | Out-of-order merge alarm and stale-rehearsal warning in status (G8, G14) | Humans merge; Athel watches that the order is kept |
| A4 | All-repo preflight up front (G11) | No half-built change because the third repo was busy |
| A5 | Per-repo cost and time in status, and EC-2 | The owner's efficiency goal is measured, not claimed |
| A6 | `/athel:followup` for the deferred removal (G3) | The removal is never forgotten |

## 5. Design

### 5.1 The change set (FR-GIT-008, ADR 035)

```yaml
change_set:
  strategy: expand_contract          # or coordinated (G9), or single (one repo)
  repos:
    - {repo: users, role: provider, order: 1, pr: acme/users#12, state: ready}
    - {repo: orders, role: consumer, order: 2, pr: acme/orders#8, depends_on: [acme/users#12]}
    - {repo: gateway, role: consumer, order: 2, pr: acme/gateway#3, depends_on: [acme/users#12]}
    - {repo: users, role: provider_cleanup, order: 3, deferred: true,
       note: "remove GET /v1/users/{id} after consumers deploy"}
  invariant: every merge in order leaves all services compatible
  verify: {at: …, bases: {…}, heads: {…}, result: pass}
```

Derived by the CLI when the human approves the plan (G1, ADR 035); read with `athel set show`, which shows a preview before approval. A single-repo
change gets `strategy: single`, so M4 behaviour is unchanged.

### 5.2 Planning

- The architect plans each repo's tasks with kinds, and cross-repo `depends_on` from a consumer
  task to the provider task it needs.
- New lints:
  - two-way repo dependencies (G2);
  - a breaking change with no expand path and no coordinated-release card (G9);
  - every `consume` task depends on an `expand` task in the provider's repo, not just any expand
    task;
  - a consumer of a changed contract (from blast radius) that is in the set but has no task.
- `/athel:plan`'s summary shows the merge order, one line per step.

### 5.3 Build

- Preflight for all repos first (G11).
- `build next` crosses repos in dependency order and goes on past a blocked repo (G12).
- Manifests carry the upstream contract (G10).

### 5.4 Ship

`/athel:ship`:
1. opens or refreshes every PR as a draft with the label (G5, G6);
2. runs `athel set verify` (A1);
3. marks every PR ready together (G7).

The body's "Linked PRs and merge order" section reads, for example:

```
This PR is 2 of 3 in CHG-012. Merge after acme/users#12.
1. acme/users#12 (provider, ready)
2. acme/orders#8 (this PR), acme/gateway#3: either order, both after #12
Later, a separate change: remove GET /v1/users/{id} after consumers deploy (not in this set)
Merge-order rehearsal: passed at <time> on these heads (build, unit, system check at each step).
```

### 5.5 Status (`athel set status`, `/athel:status`)

Each PR's state, review decision and CI from `gh pr view` (read-only). Also:
- what is merged and when;
- the next PR to merge;
- the out-of-order alarm (G8);
- whether the rehearsal is stale (G14);
- the deferred removal;
- cost and time per repo (A5).

### 5.6 Coordinated release (G9, ADR 037)

The card's recommendation states the release plan: the order, one deploy window, and the
rollback of each repo. The strategy and the card are cited in every PR body.

### 5.7 Follow-up (G3, A6)

`/athel:followup CHG` takes a human-origin grant, as `/athel:triage` does. It drafts
`prd/followup-<CHG>.md` with the removal tasks and links it to the parent. Starting the
follow-up is the developer's.

### 5.8 Edge cases

| Case | Behaviour |
|---|---|
| A repo with only `internal` tasks in a set | Its own order step, after the repos it depends on; no role in the contract chain |
| Two consumers at the same order | Either order is fine; the bodies say so, and the rehearsal tries both |
| The system check is not configured | The rehearsal runs per-repo gates at each step and says the system check did not run; it is not reported as passed |
| A system check command the policy denies | `E-POLICY`; the rehearsal fails and names the rule |
| A PR closed by a human | Status shows it; ship refuses to mark the others ready; a card asks whether to re-open or drop that repo |
| Rate limits or `gh` down during status | Status shows the last known state with its time |
| A repo the developer has not cloned (`notifications`) | Works as in M2–M4 through Athel's cache clone |

### 5.9 Eval design

**Fixture.**
- The Go services `users`, `orders` and `gateway` read their port and the other services'
  addresses from the environment.
- A system check under `tests/` starts them from given folders and runs flows across them:
  orders reads a user; the gateway routes to users.
- The fake `gh` gains labels, review decisions, check states, and a test-only merge helper
  that the grader uses as the human. Agents' `gh pr merge` stays denied.
- A one-time fixture test proves each case **needs** its order: merging the consumer before the
  provider fails the system check.

**PRDs:**
1. **`users-v2-profile`** (3 services, the exit criterion):
   - users adds `GET /v2/users/{id}` (expand);
   - orders moves its client to v2, and the gateway routes `/v2/users/` (both consume);
   - removing v1 is the deferred cleanup.
2. **`phone-e164`** (doc 01's own example, 3 services):
   - users adds a normalised phone column, backfills it and serves it (migrate, expand; uses the
     PostgreSQL migration gate approved in M4);
   - orders reads the new field (consume);
   - gateway exposes the new field (consume).
3. **`rename-now`** (2 services): the PRD forbids a transition period ("the old field must not
   be served after this release").
   - Athel must raise the coordinated-release card, with recommendation and strategy
     `coordinated`.
   - The grader merges both PRs and checks the final state.

Each PRD has a planted nearby bug, as in M4. The runner answers cards with the recommended
option and triages every side finding as a ticket, as in M4.

**Grader:** deterministic, as listed in EC-1. EC-2 comes from session costs, times and the
change's cards.

## 6. Packages

| Package | Purpose |
|---|---|
| `internal/changeset` (new) | Derive roles, order and dependencies; strategy; merge-order steps; the PR position line; rehearsal steps and evidence; status rules (next, out of order, stale) |
| `internal/plan` | The new lints (G2, G9, consumer and provider pairing, consumers without tasks) |
| `internal/pr` | Position line, label, refreshing every sibling, dropped repos |
| `internal/build` | Cross-repo scheduling past blocked repos; upstream contract in manifests |
| `internal/preflight` | All repos before the first write |
| `internal/state` | `change@5` and its migration |
| `internal/gates` | System check runner (through the engine, ADR 023) |

## 7. Data and files

```
.athel/changes/CHG-NNN/
  change.yaml                    change@5 with change_set
  changeset/verify.yaml          rehearsal evidence (changeset-verify@1)
  changeset/status.json          last status read from the host, with its time
  prd/followup-CHG-NNN.md        the deferred removal, drafted on /athel:followup
.athel/config/system.yaml        system-check@1: the system check's argv, {repo} words (ADR 036)
```

New schemas:
- `change@5`;
- `changeset-verify@1`;
- `system-check@1` for `.athel/config/system.yaml` (ADR 036: a new file, not `config@3`).

## 8. CLI and plugin additions

- **CLI:** `set show` (the set is derived at approval, ADR 035), `set verify`, `set status`, `followup draft`. `pr open` and
  `pr update` refresh every sibling, and `pr ready --set` marks the set ready. Every command has
  `--help` and `--json`.
- **Skills:**
  - `/athel:followup` (new);
  - `/athel:plan`, `/athel:build` and `/athel:ship` updated for sets;
  - `/athel:status` shows the set.
- **Agents:**
  - the architect plans cross-repo dependencies and raises the coordinated card;
  - the developer and the reviewer use the upstream contract.
  - Prompt sync and the leak guard cover all of it.

## 9. Dependencies

None: the standard library only.

## 10. Ordered task list (one PR each, about 400 changed lines; tests named up front)

| # | Branch | What | Key tests |
|---|---|---|---|
| 00 | `docs/m5-plan` | This plan | — |
| 01 | `feat/m5-fixture-system` | Runnable Go services, system check, fake `gh` labels, reviews, checks and the grader's merge helper; the needs-its-order fixture test | `TestSystemCheckAcrossServices`, `TestWrongOrderBreaksTheSystem` |
| 02 | `feat/m5-change-set` | `change@5`, migration, derivation at approval, `set show`, cleared when a plan change voids the approval; deferred tasks allow nothing; ADR 035 | `TestDeriveRolesAndOrder`, `TestTwoConsumersShareAnOrder`, `TestDeferredCleanupNotBuilt` |
| 03 | `feat/m5-plan-lints` | Two-way dependencies, breaking change without expand or card, consume-to-provider pairing, consumers without tasks; the plan summary's merge order | `TestTwoWayReposRefused`, `TestBreakingNeedsExpandOrCard`, `TestConsumeNeedsProviderExpand` |
| 04 | `feat/m5-coordinated` | Coordinated-release card and strategy, PR wording; ADR 037 | `TestCoordinatedCardSetsStrategy` |
| 05 | `feat/m5-preflight-all` | All-repo preflight before the first write; scheduling past a blocked repo | `TestThirdRepoOverlapFoundFirst`, `TestBlockedRepoDoesNotStopOthers` |
| 06 | `feat/m5-upstream-contract` | Upstream contract in developer and reviewer manifests | `TestConsumerManifestHasProviderDiff`, `TestReviewerSeesUpstreamContract` |
| 07 | `feat/m5-linked-prs` | Position line, label (Q2), refreshing siblings, dropped repos, ready together | `TestEveryBodyListsAllOthers`, `TestOpeningOneRefreshesSiblings`, `TestReadyOnlyTogether` |
| 08 | `feat/m5-set-verify` | Rehearsal in throwaway clones, system check through the engine, evidence, stale detection; ADR 036; red-team cases | `TestRehearsalCatchesBrokenStep`, `TestRehearsalNeverPushes`, `TestSystemCheckGoesThroughPolicy` |
| 09 | `feat/m5-set-status` | Host reads, next to merge, out-of-order alarm, cost per repo; `/athel:status` | `TestOutOfOrderMergeAlarm`, `TestStaleRehearsal` |
| 10 | `feat/m5-test-patterns` | Per-repo test-file patterns for QA from a structured conventions field (backlog) | `TestQAScopeFromConventions` |
| 11 | `feat/m5-agent-limits` | Per-command limits for the scout and the architect (backlog) | `TestScoutCommandTable` |
| 12 | `feat/m5-followup` | `/athel:followup` grant and draft | `TestFollowupNeedsHuman`, `TestFollowupDraftLinksParent` |
| 13 | `feat/m5-skills-agents` | Plan, build, ship and status skills; architect, developer and reviewer prompts; prompt sync | `TestShipVerifiesBeforeReady`, `TestPromptsMatchCLI` |
| 14 | `feat/m5-scenario` | A scripted 3-repo journey through the CLI with no model: derive, build records, PRs, rehearsal, merges by the test as a human, status at each step | `TestThreeRepoJourney` |
| 15 | `feat/m5-evals-set` | The 3 PRDs, runner and grader (EC-1, EC-2) | `TestSetGraderMergesInOrder`, `TestEfficiencyMetrics` |
| 16 | `docs/m5-report` | M5 report, `EVALS.md`, HANDOFF, ROADMAP | — |

ADRs expected:
- 035: the change set is derived by the CLI;
- 036: merge-order rehearsal and the system check;
- 037: coordinated release.

## 11. Risks

| Risk | Mitigation |
|---|---|
| **Cost:** a 3-service journey is about 3 one-repo journeys | Q1. One smoke run first. EC-2 makes cost a measured target, not an afterthought |
| **The system check runs repo code** | It runs only through the policy engine with a scrubbed environment and a timeout (ADR 023), in throwaway clones; red-team cases for a malicious check |
| **The rehearsal disagrees with real CI** | It is labelled a local rehearsal; status shows the real CI checks next to it |
| **The fixture's in-between states are too easy**, so the eval proves nothing | The fixture test proves the wrong order breaks every case (task 01) |
| **AI variance** | Deterministic CLI checks wherever possible; regrade from saved runs; every run must be valid |
| **Usage limits and network outages** cut runs (as in M4) | Runs are kept and regraded; cut runs are reported as such and re-run with your OK |

## 12. Questions for the owner (only cost and outward-facing actions; everything else is decided above, open to change)

| # | Question | Recommendation |
|---|---|---|
| Q1 | Eval budget. A 3-service run is estimated at **$20–30** (EC-2 aims at about $29 or less). One smoke run (about $30), then the full run of 3 PRDs × 3 runs (about $200–270), plus one re-run in reserve: **up to about $550 in all**, each still asked before it starts | Yes |
| Q2 | Labels: `pr open` creates the `athel:CHG-NNN` label in a repo that lacks it (a small write to the repo's settings, through your `gh` login) | Yes. Without it, the change ID stays in the title and body only, and doc 04's label rule is not met |
| Q3 | After the evals pass, the live check M4 left open, now multi-repo: **3 throwaway private repos** under your account, where Athel opens linked draft PRs with labels and marks them ready, and **you** merge them in order. Then dogfooding with writes on Athel itself (doc 08) | Yes to both, each asked again right before it happens |
