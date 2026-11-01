# ADR 002: All state as plain files in `.athel/`

Status: Accepted (2026-11-01, from doc 02 §Key design decisions)

## Context
A change must survive context compaction, session death and a move to another machine, and
humans must be able to audit everything Athel did (doc 01 principle 7).

## Decision
All state is plain files under `.athel/` in the workspace: YAML for state and config,
Markdown for human documents, JSONL for append-only logs. Every file has a versioned JSON
Schema (`athel/<name>@<n>`) and the helper CLI rejects any write that fails it. Writes are
atomic (temp file in the same directory, fsync, rename) and serialised with OS file locks.

## Alternatives rejected
- SQLite: not diffable or reviewable by humans; binary merges.
- A server database: needs a daemon; breaks "survives any session".

## Consequences
- Concurrency is handled by file locks plus atomic renames, not transactions.
- Schema changes need a migration (`athel migrate`, later) and a version bump.
- Large raw data goes to `artifacts/`, never into state files.

## Spec references
doc 02 ADR 002, §State model; doc 08 Practices (schema versions, migrations).
