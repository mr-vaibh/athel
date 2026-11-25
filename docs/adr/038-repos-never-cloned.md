# ADR 038: A repo the developer never cloned is found under the one org

Status: Accepted (2026-11-25; found by M5 full eval 1, under the owner's delegation for technical
choices)

## Context
Doc 08's fixture plants "a consumer only in another repo": `notifications`, which the developer
has no clone of. M2 finds it; in M5 the architect rightly put it in a change set. Plans name repos
by short name (`notifications`), and Athel resolves a short name through the repo index, which
lists only the developer's clones. So `git preflight CHG notifications` failed ("not an indexed
repo"), and the full address failed too ("no task writes in a repo by that name"): the change could
never start building. In a company with many services, most repos are not cloned locally.

## Decision
- `repos.Index.Infer(name)`: when a short name has no clone and every indexed remote shares one
  host/org, the name means `<host/org>/<name>`. Repos across several orgs get no guess: the
  developer or the plan names the repo in full, as before.
- `athel repos cache` and the CLI's name resolution use it, so preflight, the worktree, the
  rehearsal and status work for such a repo like any other: Athel clones it into its own cache
  from the remote, never into the developer's folders.
- A short name with no such remote fails as a clone error naming the remote it tried.

## Alternatives rejected
- **Require full names in plans:** the plan, specs and PR titles would all carry host/org; the
  architect already struggled with the mismatch in the eval.
- **Search the git host for the name:** a network search and a guess across orgs.

## Consequences
- A change can reach services the developer never cloned, which is most of them at a company.
- The M3 behaviour ("name it as host/org/repo") remains for multi-org workspaces.

## Spec references
Doc 03 §Repo discovery; doc 04 FR-GIT-001 (Athel's own cache clones); doc 08 §Fixture system
("a consumer only in another repo"); M5-plan §5.8.
