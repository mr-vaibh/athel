# ADR 010: Git worktrees per change per repo

Status: Accepted (2026-11-01, from doc 02 §Key design decisions). Recorded in M0; used from M4.

## Context
Athel must never disturb the developer's own checkouts or teammates' work (doc 01 principle 5,
FR-GIT-001, FR-DISC-016), and later must run independent tasks in parallel (F-22).

## Decision
Every write happens in a git worktree created from Athel's own cache clone
(`workspace/cache/<repo>.git`, partial clone, refreshed before use), one worktree per change
per repo under `workspace/repos/<repo>@<change-id>`. The developer's local clones are only
read (a fixed allow-list of read-only git commands).

## Alternatives rejected
- Switching branches in the developer's checkout: disturbs their work and their uncommitted
  changes; makes parallel tasks impossible.

## Consequences
- M0 reserves the `workspace/repos/` and `workspace/cache/` paths and a `repos:` field in
  `change.yaml`, and designs the lock model so repo/worktree locks and task locks (doc 06) can
  be added under `.athel/locks/` without changing the change lock.
- Disk usage grows with active changes; worktrees are removed when a change reaches a
  terminal state (M4 design).

## Spec references
doc 02 ADR 010, §State model; doc 03 §Repo access across services; doc 04 FR-GIT-001.
