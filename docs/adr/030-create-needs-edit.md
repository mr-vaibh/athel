# ADR 030: A task that creates files also needs the edit verb

Status: Accepted (2026-11-21; found by full build eval 2, under the owner's delegation for
technical choices)

## Context
Doc 05 lists a task's verbs (`edit`, `create`, `delete`, `test`, `commit`, `push`, `pr`,
`comment`) and doc 04 rule 7 allows a write only when a task's verbs and paths match it. Athel
counts the Write tool as `create` and the Edit tool as `edit`. Doc 05 does not say whether
`create` covers later changes to a file the task created.

In full build eval 2, `saved-addresses` run 1, the architect planned a new store as
`verbs: [create, test, commit]`. The developer wrote the files. When the reviewer and security
raised findings, neither the developer's fixes nor QA's tests could edit those files, and the
change stopped in `blocked` with nothing for the developer to answer. Athel's own plan template
showed the same verbs on its example task, and the plan lint accepted them.

## Decision
The plan lint refuses a task that has `create` without `edit`. The task's review fixes and QA's
tests edit what it created, inside the same task (doc 06, the per-task build loop), so the plan
must allow it. The template's example task and its comment carry `edit` with `create`.

## Alternatives rejected
- **Treat an edit of a file the task created as `create`:** the engine would need to remember
  who created each file, and a plan would allow writes its verbs do not show.
- **Let the agents rewrite whole files with Write:** it hides the problem, makes reviews of fix
  commits harder, and QA would still be refused where it uses Edit.

## Consequences
- A plan with `create` alone is refused at `plan set`, before the developer approves it, with a
  message telling the architect to add `edit`.
- A task may now edit any file within its paths, not only those it creates; its paths still bound
  it, as they always did for `edit`.

## Spec references
Doc 05 §Plan format and planning rule 2; doc 04 rule 7; doc 06 §The per-task build loop.
