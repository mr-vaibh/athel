# ADR 012: Dependency `go.yaml.in/yaml/v3` for YAML

Status: Accepted (2026-11-01; approved by the owner as decision Q13)

## Context
ADR 002 makes YAML the format for state (`change.yaml`) and config (`athel.yaml`, later
`tools.yaml`, `git.yaml`, …). Go's standard library has no YAML support.

## Decision
Use `go.yaml.in/yaml/v3` (v3.0.6), the YAML organisation's maintained continuation of the
archived `gopkg.in/yaml.v3`, with the same API. It is pure Go with no dependencies. Decoding
rejects duplicate keys; Athel additionally rejects multi-document input (`schema.DecodeYAML`).

## Alternatives rejected
- Store state as JSON instead of YAML: stdlib-only, but deviates from the spec (doc 02 ADR 002,
  every example in docs 02–07) and is harder for humans to read and edit.
- `gopkg.in/yaml.v3`: archived upstream; no security fixes.
- `sigs.k8s.io/yaml` / `goccy/go-yaml`: more dependencies or a different API with no benefit here.

## Consequences
- One direct dependency; tracked in `go.mod` and checked by `go mod verify` in CI.
- Timestamp-looking strings are quoted on output by the encoder, so they round-trip as strings.

## Spec references
doc 02 ADR 002, §State model, §Configuration and policy files; CLAUDE.md "every dependency
justified in an ADR".
