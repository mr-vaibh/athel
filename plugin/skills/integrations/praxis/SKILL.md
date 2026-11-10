---
description: Reference for the Athel scout - reading the Facets view of environments and the organisation's Praxis data (status, duties, memory, catalogs) with the praxis CLI, read-only.
user-invocable: false
---

# Praxis playbook (Facets, read-only)

Facets is read through Praxis; Raptor (the Facets write CLI) is never run. This playbook and the
`praxis` entry in tools.yaml are the only Facets-specific parts of Athel, so they can be removed
when Facets is retired (doc 03 §Facets layer).

## When to use
The *intended* setup Facets holds (services, releases, environment config) to compare with what
GCP and the cluster actually run (a difference is a drift finding); the organisation's recorded
conventions and decisions (memory); scheduled agents' findings (duties); and which catalogs claim
a repo.

## Read queries
```sh
praxis status
praxis memory recall "customer phone number"
praxis memory list
praxis duty list
praxis duty findings nightly-drift
praxis ig claims --git https://github.com/acme/users
praxis ig list
praxis mcp
```

## Filtering large output
Praxis prints JSON when its output is not a terminal; filter with `jq`. `memory recall` ranks on
the server; use `memory list` only when recall misses.

## Pitfalls
- `praxis mcp <server> <fn>` runs a function on the Praxis server under the organisation's cloud
  credentials, which can change real systems. Athel denies every call; `praxis mcp` with no
  arguments only lists the functions.
- `praxis git-credential` hands out VCS tokens and is forbidden.
- `login`, `logout`, `update`, `refresh-skills`, `report`, `memory add`, `ig sync`, `ig publish`
  and `ig manifest` change state and are denied.
- Memories are org knowledge written by people. Use them as hints (confidence likely) and verify
  against the systems themselves.

## Environments
Praxis profiles map to organisations, not environments. Name the environment from the Facets data
you read; when it is unclear, it counts as prod.
