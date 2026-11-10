# ADR 022: The actual blast radius comes from deterministic diff patterns

Status: Accepted (2026-11-10; M3 task 10, under the owner's delegation for technical choices)

## Context
Doc 05 computes the blast radius twice: from the spec before plan approval, and from the real
diff before each PR. "Any difference between them blocks the PR until the plan is updated"
(FR-BLAST-005). The diff side must be repeatable, so the same diff always gives the same
verdict; a model's reading of a diff is not.

## Decision
- **`athel blast diff <repo> --base B --head H`** runs `git diff --unified=0` in Athel's cache
  clone (never the developer's clone) and finds, line by line:
  - routes: Go `HandleFunc("METHOD /path")`, and `router|app|r.get|post|…('/path')` in JS/TS;
  - event names: quoted `name.part.vN`;
  - config keys: `os.Getenv`, `os.LookupEnv`, `process.env.X`;
  - exported Go symbols.

  Files are sorted into migrations (under `migrations/` or `db/migrate/`), dependency
  manifests, and unclassified (no pattern explained them). An unclassified file never lowers
  risk.
- **`athel blast check <change> <repo> …`** compares that surface with the change's planned
  report. It flags:
  - a route or event that no planned changed item names;
  - a migration without a planned data change;
  - a config key without a planned infra item;
  - a manifest change without a planned dependency.

  Any such impact is `E-GATE-BLAST-DRIFT`. The command writes `blast-actual-<repo>.md` either
  way. M4's `ship` step calls it before every PR.
- **Patterns start with Go and TypeScript**, the `shop` fixture's languages. Other languages
  add patterns here.

## Alternatives rejected
- **Ask the model to compare the diff with the plan:** not repeatable, and the model has a
  stake in passing.
- **Parse the ASTs of every language:** heavy, and still per language. Line patterns catch the
  contract surfaces doc 05 lists; anything else shows as unclassified.

## Spec references
Doc 05 §Blast radius analysis (steps 1 and 2) and FR-BLAST-001/005; M3 plan G2.
