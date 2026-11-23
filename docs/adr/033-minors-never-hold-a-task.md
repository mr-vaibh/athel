# ADR 033: Open minor findings never hold a task; every agent records side findings

Status: Accepted (2026-11-23; found by build evals 4 and 5, fix pass chosen by the owner)

## Context
Doc 06 says three things about ending review:
- §Iterative review protocol: "a task can only finish when no blocker or major finding is open".
- The severity table: a minor is "fixed if cheap; otherwise listed in the PR"; a nit is
  "optional; never blocks".
- Round rule 4: "a round with zero new blocker or major findings and all previous ones verified
  ends review for the task".

The reviewer prompt read rule 4's "all previous ones" as every earlier finding, minors
included, and the CLI took the reviewer's verdict as given. In build eval 4 (`saved-addresses`
run 2) every blocker and major was verified by round 3, but the security checker then raised
one new minor point per round about unusual Unicode characters, the reviewer kept
`changes_requested`, and the 5-round card stopped the task.

Separately, doc 07 says "any agent can record a side finding at any time", but only the build
agents were told to. In build evals 4 and 5, two runs read the file with the planted nearby bug
during spec and plan, and no agent recorded it.

## Decision
- Rule 4's "previous ones" means the earlier blocker and major findings, read together with the
  severity table and the protocol's own summary.
- `review save` refuses a verdict that disagrees with the findings: `approved` while a blocker or
  major is open, and `changes_requested` while none is. The verdict is thereby set by the CLI's
  rule, not by the model.
- Open minors and nits carry over as before and are listed in the PR body under "Tests and
  gates", with their task, ID, status, location and text.
- The scout (when working for a change) and the architect record side findings with `athel
  finding add`, after checking `athel finding list`, and never put such a bug in the spec or
  the plan. The policy engine already allowed this; a test now holds it.

## Alternatives rejected
- **Limit what later rounds may report** (for example only new blockers and majors): it hides
  real findings from the PR, and doc 06 lets later rounds review the new diff in full.
- **Keep the reviewer's verdict free and ignore it in `Done`**: the round file would then say
  `changes_requested` for a task that ended, which misleads anyone reading the history.

## Consequences
- A task with only minor and nit findings open finishes; the human reviewer sees them in the PR.
- A round file's verdict always matches its findings.
- Side findings may now be recorded before the build starts; triage at PR time is unchanged.

## Spec references
Doc 06 §Iterative review protocol, severity table, round rules 1–5, FR-BUILD-010, FR-BUILD-013;
doc 07 §Side findings, FR-FIND-001–003.
