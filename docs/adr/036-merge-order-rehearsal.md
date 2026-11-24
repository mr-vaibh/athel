# ADR 036: The merge-order rehearsal and the system check

Status: Accepted (2026-11-24; M5 task 08; addition A1 of the M5 plan, approved by the owner)

## Context
Doc 04's change set has an invariant: "every merge in order leaves all services compatible".
The spec says how to plan for it (expand and contract) but not how to show it holds. Humans
merge, one PR at a time; if the order is wrong, or the provider is not really backward
compatible, production breaks between two merges. Athel never merges, and the evals' grader
cannot be there for real users.

## Decision
- **`athel set verify CHG`** rehearses the merges before any PR of the set is marked ready:
  - the states are each prefix of the merge order; where PRs share a step, each of them alone
    after the earlier steps, then all of them (a human may merge either first); for a
    coordinated release only the final state (doc 04 rule 5, ADR 037);
  - each state is rebuilt from scratch: every repo cloned (`--shared`) from Athel's cache at its
    base head, and the PRs of the merged repos merged with a merge commit, as a human would. The
    clones live under the workspace's `cache/rehearsal/CHG-NNN/` and are removed afterwards;
    nothing is ever pushed;
  - in each state, the build and unit gates of the repos merged at that step run (the repo's
    own gates from `gates.yaml` or the language defaults), and the workspace's system check;
  - every command, the system check included, goes through the policy check of ADR 023 and the
    scrubbed gate environment: an unknown or network command is reported as unavailable and
    fails the rehearsal;
  - a PR that does not merge cleanly on its base stops with `E-CONFLICT-MERGE`.
- **The system check** is `.athel/config/system.yaml` (`system-check@1`): one argv whose `{repo}`
  words become that repo's folder in the state. Repos it names that are not in the set are
  cloned at their base too. Without it, the rehearsal runs only each repo's own gates and says
  "not_configured"; it is never reported as passed.
- **Evidence** goes to `changeset/verify.yaml` (`changeset-verify@1`); the change set records the
  time, the result and every base and PR head it ran on.
- **`pr ready` is refused** for a set of two PRs or more unless the last rehearsal passed on
  exactly the current heads: a push or a base that moved makes it stale. `/athel:ship` rehearses
  before marking ready, and each PR body says whether and when the rehearsal passed.

## Deviation from the M5 plan
The plan put the system check in `athel.yaml` (`config@3`). It is a separate file instead:
`athel.yaml` holds the change's settings, while the system check is workspace infrastructure,
like `gates.yaml`, and a new file needs no migration of existing configs.

## Alternatives rejected
- **Trust the ordering lint:** it proves the plan's shape, not that the code built from it is
  compatible.
- **Run only each repo's own tests per state:** they pass against stubs of the other services;
  only a check across services shows a consumer calling an endpoint the provider lacks.
- **Rehearse every subset of PRs sharing a step:** exponential; one-alone plus all covers the
  common case of two or three consumers.

## Consequences
- "Every state in between works" is shown on the exact heads reviewers see, before anyone
  merges; the cost is one build per merged repo per state plus the system check.
- A workspace without a system check still gets per-repo checks at each state, and the PR says
  the system check did not run.

## Spec references
Doc 04 §Multi-repo change sets (invariant, ordering rules), FR-GIT-007/008; doc 08 M5 exit
criterion; M5-plan A1, G7, G14, §5.4.

## Follow-up (M5 task 14, 2026-11-24)
- Repos the system check names that the change does not touch are cloned into Athel's cache by
  the rehearsal itself (status and `pr ready` only read, and never fetch).
- The shop fixture's system check is a program in its own `gateway` repo (`gateway/systest`,
  standard library only, flows in `systest/flows.json`), run as
  `go -C {gateway} run ./systest --flows systest/flows.json --users {users} --orders {orders}
  --gateway {gateway}`: inside the rehearsal folder, so ADR 023's check allows it. A command from
  outside the workspace (`go -C <elsewhere>`) is refused, as it should be. Companies often keep
  such end-to-end checks in a repo; the same form works for them.
