# Athel roadmap: what is done, what is left

Last updated: 2026-11-02, at the move from the borrowed machine to the owner's personal machine.
Source of truth for milestones: `docs/specs/08-implementation-plan.pdf`. This page summarises it
and records where the build stands.

## Progress at a glance

| Milestone | What it ships (doc 08) | Exit gate | Status |
|---|---|---|---|
| **M0 Skeleton** | Plugin manifest, helper CLI (state, locks, schemas, IDs), `/athel:start`, `/athel:status`, `/athel:resume`, config validation | Create, pause and resume a change in a new session; 90% CLI coverage; hook latency < 50 ms | ✅ Done. PRs #1–#23 merged. `docs/plans/M0-report.md` |
| **M1 Guardrails** | Policy engine, shell parser, environment detection, scrubber, hash-chained audit log, `tools.yaml`, `environments.yaml` | Red-team suite 100% blocked; 0 false allows; audit verify passes | ✅ Done. PRs #24–#38 merged. `docs/plans/M1-report.md` |
| M2 Understand | Scout agent, local repo index and repo cache, investigation protocol, fact cache with verification, integration playbooks (GitHub, kubectl, gcloud, Praxis/Facets, Grafana, Jira or Linear, Slack), ad-hoc blast radius | All 10 question types pass on fixtures with traps; confidence labelling 100% correct | **Done**, accepted by the owner 2026-11-10 with the measured shortfall (`M2-report.md`): claims 100%, traps 29/30, exact labels 85.7–90.5%. PRs #43–#58, not yet merged |
| M3 Specify | PRD intake, spec writer, traceability, decision cards, blast radius (planned and actual), plan and approval, brownfield onboarding | 3 eval PRDs produce valid specs, reports and plans; blast radius recall 95% on fixtures | Not started |
| M4 Build | Git preflight, worktrees, developer, QA, security and reviewer agents, gates, iterative review, side findings, draft PRs | One-repo eval changes reach a PR with all gates green; reviewer catches 90% of seeded defects | Not started |
| M5 Multi-service | Change sets, expand-and-contract ordering, linked PRs, cross-repo status | A 3-service fixture change merges in order with every intermediate state compatible | Not started |
| M6 Verify and learn | Deploy-and-test guide, read-only staging verification, staging DB credential helper, UAT rounds, lessons, eval harness in CI, headless investigation, reports | Full journey end to end on the fixture system; a lesson from round 1 prevents the same correction in a second change | Not started |
| Later | Parallel worktrees, org-wide policy sync (F-22, F-24) | To be specified | Not planned |

Doc 08's estimates for what is left: M2 3 weeks, M3 3, M4 4, M5 2–3, M6 3 (one developer with
Claude Code).

## What exists in the repo today (M0 + M1)

| Area | Where |
|---|---|
| Claude Code plugin: skills `start`, `status`, `resume`, `approve`, `accept`; hooks for 9 events | `plugin/` |
| Helper CLI `athel` (Go) | `cli/cmd/athel`, `cli/internal/*` |
| State: change model, transitions, session lock, steps, checkpoints, budgets, plan binding, integrity seal | `cli/internal/state`, `cli/internal/plan` |
| Config and policy files (layered, tighten-only merge) | `cli/internal/config`, `cli/internal/policy`, `plugin/policies/defaults/` |
| Guardrails: shell parser, classifier, environment detection, rules engine, scrubber, audit log, human-origin grants | `cli/internal/{shellparse,classify,envdetect,engine,scrub,audit,grants}` |
| Schemas (versioned) | `schemas/` |
| Tests: unit (≥ 90% per package), red-team (431 cases), allow-list, scenarios, latency bench | `cli/**/_test.go`, `tests/` |
| Decisions | `docs/adr/` (001–003, 010–017) |

CLI commands: `version`, `init`, `change new|list|show`, `state show|transition|enter|exit`,
`checkpoint set|show`, `lock acquire|refresh|release|status`, `budget add|show`, `resume`,
`config validate|show`, `schema validate`, `policy validate|show|decide`, `plan set|show`,
`migrate`, `audit verify|tail`, `integrity accept`, `hook <event>`.

## What is left, by milestone

Each milestone runs the same four phases as M0 and M1:
1. **Plan:** write `docs/plans/Mn-plan.md` (exit criteria, requirement map, design, packages,
   task list with tests named up front, risks, questions) and **stop for the owner's approval**.
2. **Scaffold** what the plan needs.
3. **Build task by task**, one PR each, gated by `scripts/check.sh`.
4. **Prove the exit criteria** in `docs/plans/Mn-report.md`, then update the handoff.

### M2 Understand (done; see `M2-report.md`)
- Read first: doc 08 M2 row; doc 03 (discovery and integrations) in full; doc 02 (agents, fact
  model); doc 04 rule 12's exception (repo search roots, read-only git commands in the
  developer's clones).
- Features F-04 to F-06 and F-07 (ad-hoc). Requirements FR-DISC-001 to 014.
- Needs the **fixture system `shop`** from doc 08 §Fixture system: services, contracts, data,
  git, cluster, observability, tickets and chat, with traps. M2 is the first milestone that
  needs it, because "all 10 question types pass on fixtures with traps".
- M1 items that land here:
  - repo search roots for rule 12;
  - the Praxis read allow-list in `tools.yaml`;
  - `internal/facts` from doc 08's repo layout.

### M3 Specify
- Docs 05 (spec, blast radius, planning) and 07 (decision cards). FR-SPEC, FR-BLAST, FR-DEC,
  FR-PLAN.
- Needs the first eval PRDs in `tests/evals/`.
- Carries M2's open eval target (`docs/plans/EVALS.md`): 100% exact confidence labels and
  30/30 traps, in two full runs in a row. Plan an improvement from its list as an early M3
  task, then re-run with the owner's go-ahead.
- M1's `plan@1` schema and hash-bound approval are already in place.

### M4 Build
- Doc 06 and doc 04 §Git. FR-GIT-001 to 007, 009, 010; FR-BUILD; FR-FIND.
- From M1: `OwnBranches` for rule 9 must be filled when Athel creates branches. Until then
  every push is denied.
- Also from M1: worktree layout `workspace/repos/<repo>@<CHG>` (rules 7 and 10 already expect
  it) and `internal/git`.
- Dogfooding with writes starts here (doc 08 rules).
- Backlog items due here: a network-deny sandbox test for the CLI; the `in_review → building`
  transition.

### M5 Multi-service
- Doc 04 §Multi-repo change sets. FR-GIT-008, FR-PLAN-003. `change_set` already exists in the
  change schema as a placeholder.

### M6 Verify and learn
- Doc 07. FR-VER, FR-UAT, FR-LEARN. Staging DB credential helper (doc 04 §Secrets,
  `internal/dbquery`), eval harness in CI, headless investigation.

## Open items carried from M0/M1

| Item | Where recorded | When |
|---|---|---|
| ~~Live check in a real Claude Code session of three documented behaviours~~: all confirmed on 2026-11-09 with `tests/live/claude-behaviours.sh` | ADR 016, ADR 017 follow-ups, M1-report §6 | Done |
| Delete merged task branches on GitHub (M0 and M1) | backlog | When the owner says so |
| Windows support, multi-platform packaging, PowerShell parsing | backlog | Release packaging |
| Policy cache | backlog, ADR 016 | Only if hook p95 > 30 ms |
| PermissionRequest/PermissionDenied audit | backlog | Any milestone |

## Path to real use (doc 08 §Pilot)

1. Personal pilot (a new project and an existing repo of the owner's): everything up to the PR.
   Move on after 3 changes with zero guardrail incidents.
2. Read-only at Snabbit (M2 and M3 capabilities only). Prepare first: `environments.yaml` for
   real clusters, scoped read-only credentials, the MCP server list, git and CI details, a
   platform or security owner.
3. Writes at Snabbit on one low-risk service, with its owners' agreement.
