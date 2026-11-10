# ADR 021: State transitions check the step's work (step guards)

Status: Accepted (2026-11-10; M3 task 08, under the owner's delegation for technical choices)

## Context
Doc 02's workflow engine checks that a transition is in the fixed table, and that human-only
transitions come from the human. Nothing checked that the work a state stands for exists: a
change could reach `specified` with no spec, or `planned` with no plan. Doc 05 says the CLI
"fails the plan step if any link is missing", and that specs "cannot drift from the PRD
unnoticed".

## Decision
- **The guard** sits in `athel state transition`, before the fixed-table check and before the
  lock is claimed. Every transition goes through this command; the hooks never transition.
  - **→ `specified`:** the current PRD version has a review, and at least one spec exists.
    Every spec passes its checks again against the current PRD, the decisions and the
    conventions. A PRD change, an unknown decision or stale conventions block the move.
  - **→ `planned`:** the trace is complete (PRD → REQ → AC → task). The stored plan passes the
    planning lints (ADR 020). No unanswered card blocks the `spec` or `plan` step; cards that
    block only specific tasks wait for the build.
- **`athel trace`** writes `traceability.md`: one row per criterion, its missing links, and
  the PRD statements no requirement cites. Commit and test columns are filled from M4 on.
- **Always on.** The guards apply to every change; there is no switch for "old style"
  changes, since a switch would be a way round them. The M0 and M1 scenario suites, the
  red-team fixture, the demo and the live-check script now do the spec step's work
  (`tests/internal/m3docs`, with a shell mirror checked against the Go copy).

## Alternatives rejected
- **Guards in the state package:** about 15 store-level tests walk states to test the table
  itself. The CLI is the only caller that transitions, so the CLI is the right gate.
- **Guards only for changes that have specs:** a model could skip the spec step to avoid them.

## Spec references
Doc 02 §Workflow engine; doc 05 §Spec format (traceability), FR-SPEC-003, FR-PLAN-002,
FR-DEC-003.
