# Athel — background

Read this first if you are new to the project or arriving on a new machine.

## What Athel is

Athel is a **spec-driven agentic development framework** shipped as a **Claude Code plugin**
(skills, subagents, hooks) plus a small, deterministic **Go helper CLI called `athel`**.

A human gives Athel a PRD; Athel investigates the real running system (code, cluster,
dashboards, tickets) with the developer's own logged-in tools, writes traceable specs,
reports the blast radius, asks only the questions that matter, produces a plan the human
approves, builds it task by task behind hard quality gates and independent review, opens
PRs, writes a deploy-and-test guide, verifies staging read-only after the developer deploys,
runs UAT rounds, and turns confirmed corrections into lessons for future changes.

Claude Code is the reasoning and coding engine. Athel owns the method, the rules, the state
and the safety.

## Why it exists

One huge prompt fails at what matters in a large system: it goes stale, mixes rules with
hints, has no memory between sessions and no enforcement ("run the tests" is a request, not a
gate). Existing spec-driven tools (Spec Kit, Kiro, BMAD, OpenSpec) work inside one repo; none
treats the whole running system as context. Athel targets that gap (doc 01, problem statement).

## Who it is for

First a single developer driving Athel from Claude Code; later teams and the owner's company,
which runs many microservices (on GCP, managed through Facets). Personas: feature owner,
engineer on an existing service, reviewer, tech lead / platform owner (doc 01).

## Key decisions already made by the project owner

All of these are also in the spec (docs 01, 02, 08):

- **Claude Code only.** No other coding engines in v1.
- **Go helper CLI** (`athel`), one static binary per platform inside the plugin. It does all
  state changes, locks, IDs, hashes, schema validation and (from M1) policy decisions. It
  never calls an LLM and makes no network calls except git and configured health checks.
- **GitHub and GitHub Actions** for git hosting and CI.
- **Athel never deploys** to any environment, staging included, and never changes
  infrastructure. It writes a deploy-and-test guide and verifies read-only after the
  developer deploys. **Humans merge and deploy.**
- **Facets** is read through the **Praxis** read CLI. The **Raptor** write CLI is registered
  but **every call is blocked**. Facets entries are isolated so they can be removed in
  6 to 12 months.
- **Local clones are read-only.** Athel works only in its own cache clones
  (`workspace/cache/<repo>.git`) and worktrees (`workspace/repos/…`).
- **Headless runs are supported** (`claude -p`): same hooks and policy, every "approve"
  becomes "deny", decision cards are queued, budgets halved, read-only credentials in CI.
- **90% coverage gate** (changed-lines coverage for target repos; 90% line coverage on
  every package of Athel's own CLI).
- **Tests follow each target repo's existing test style.**
- **Lessons always need human confirmation**; nothing auto-activates.
- PR size limit: 400 changed lines per PR (not per change), raisable at plan approval.
- Verification thresholds, decision-card timers and other numbers: see doc 08 "Decisions
  needed from you" (all decided).

## How it is built

Seven milestones, each with a hard exit gate (doc 08):
M0 Skeleton → M1 Guardrails → M2 Understand → M3 Specify → M4 Build → M5 Multi-service →
M6 Verify and learn. Safety (M1) ships before any capability that can write.

## Where everything lives

| What | Where |
|---|---|
| Specification (source of truth, 8 PDFs) | `docs/specs/01-master-prd.pdf` … `08-implementation-plan.pdf` |
| Standing rules for every session | `CLAUDE.md` |
| Original kickoff instructions | `KICKOFF-PROMPT.md` |
| Milestone plans and reports | `docs/plans/M0-plan.md`, `M0-report.md`, `M1-plan.md`, `M1-report.md` |
| Handoff for the next session | `docs/plans/HANDOFF.md` |
| Out-of-scope ideas | `docs/plans/backlog.md` |
| Architecture decision records | `docs/adr/` |
| Session journals (append-only) | `docs/journal/YYYY-MM-DD-session-N.md` |
| Owner decisions made in chat | `docs/context/decisions-from-chat.md` |
| Tools, versions, machine setup | `docs/context/environment.md` |
| Plugin / CLI / schemas / tests | `plugin/`, `cli/`, `schemas/`, `tests/` (from Phase 2 of M0) |
| Red-team and allow-list suites (M1) | `tests/redteam/cases/*.yaml` |
| Guardrail scenario (M1) | `tests/scenarios/m1-guardrails/` |
