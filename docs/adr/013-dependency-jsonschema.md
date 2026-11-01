# ADR 013: Dependency `github.com/santhosh-tekuri/jsonschema/v6` for schema validation

Status: Accepted (2026-11-01; approved by the owner as decision Q13)

## Context
Doc 02 requires that every state file has a versioned JSON Schema and that the helper CLI
rejects any write that fails it (§State model, §Component responsibilities). Doc 08 requires
schema tests in CI. Go's standard library has no JSON Schema validator.

## Decision
Use `github.com/santhosh-tekuri/jsonschema/v6` (v6.0.3): pure Go, JSON Schema draft 2020-12,
schemas loaded from memory (no network loader is configured; schema URLs use the unresolvable
host `schemas.athel.invalid`). Schemas live in `schemas/` and are embedded in the binary.
Compiled schemas are cached per process in a `schema.Registry`.

Transitive dependency: `golang.org/x/text` (used by the library for message formatting).

## Alternatives rejected
- Hand-written Go validators with the JSON Schemas kept only as documentation: two sources of
  truth that drift, and the schemas would never be tested.
- `xeipuuv/gojsonschema`: unmaintained, older drafts only.
- `qri-io/jsonschema`: unmaintained.

## Consequences
- Validation errors carry JSON-pointer instance locations, reported as `ViolationErrors`.
- Compiling a schema costs a few milliseconds, so hot paths (the PreToolUse hook) must not
  compile schemas they do not need; measured by the hook benchmark (PR-17).

## Spec references
doc 02 §State model, §Component responsibilities (Schemas row); doc 08 Practices.
