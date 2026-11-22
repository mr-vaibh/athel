# ADR 026: In change mode, pushes and pull requests go only through the CLI

Status: Accepted (2026-11-14; M4 task 04, under the owner's delegation for technical choices;
M4-plan G15)

## Context
Doc 04 rule 9 allows W1 actions, "pushes and PR edits on branches Athel created", and M1 built
it that way: an agent's `git push origin athel/CHG-001-x` was allowed once the branch was
Athel's own. Doc 04 also requires, before every push:
- a re-fetch;
- stopping when someone else has pushed to the branch;
- a rebase on base only while the branch was never pushed, and a merge afterwards;
- the pre-commit secret check.

FR-GIT-007 adds that PRs are drafts until every gate passes. A raw `git push` or
`gh pr ready` typed by an agent can skip every one of these steps, and nothing in the command
line shows whether they ran.

## Decision
- **Rule 9 in change mode denies every W1 action of the session**, with the next action
  naming the command that does it safely: `athel git push CHG REPO`, and
  `athel pr open|update|ready CHG REPO`. The error code is `E-POLICY-USE-ATHEL`.
- **A push to someone else's branch** is still denied first, with `E-POLICY-FOREIGN-BRANCH`,
  as before.
- **The CLI pushes and edits PRs itself**, in Athel's worktree, after its own checks. It
  refuses any branch that is not the change's own, and any protected branch, whatever
  `change.yaml` says.
- **Merging stays forbidden for everyone (rule 3).** The CLI has no merge command.

## Alternatives rejected
- **Allow raw pushes and run the checks in a git pre-push hook:** a hook cannot raise a
  decision card, cannot merge base safely mid-push, and is skipped by `--no-verify`, which
  would need yet another rule.
- **Allow raw pushes and re-check afterwards:** the push has already left the machine.

## Consequences
- This narrows M1's rule 9 and widens nothing. The engine test "9 own branch" now expects a
  deny, and the M4 CLI test proves that the athel path works.
- The developer agent's instructions (task 12) name the athel commands. A denied raw push
  tells the agent what to run instead.

## Spec references
Doc 04 rule 9, FR-GIT-004, FR-GIT-006, FR-GIT-007, §Before every push; M4-plan G15.
