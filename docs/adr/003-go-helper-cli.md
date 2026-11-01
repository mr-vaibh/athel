# ADR 003: A small deterministic helper CLI in Go

Status: Accepted (2026-11-01, from doc 02 §Key design decisions and §Open technical questions)

## Context
Model output is probabilistic. State transitions, locks, IDs, hashes, schema validation and
(from M1) policy decisions must be exact, repeatable and testable. The PreToolUse hook runs on
every tool call, so the binary's startup time is on the critical path (target < 50 ms p95).

## Decision
`athel` is a Go program shipped as one static binary per platform inside the plugin
(`plugin/bin/athel`). It never calls an LLM and makes no network calls except git and
configured health checks. It uses the standard library first; every dependency needs its own
ADR. Source lives in `cli/` with `internal/` packages; the module root is the repo root so the
binary can embed `schemas/` and `plugin/policies/defaults/`.

## Alternatives rejected
- Let the model edit state files directly: unverifiable, not deterministic.
- Python: needs a matching runtime everywhere; startup with YAML and schema libraries can
  approach or exceed the 50 ms hook budget.

## Consequences
- Command dispatch uses the standard `flag` package (no CLI framework) to keep startup and
  dependency surface minimal.
- Time and randomness are injected (`internal/clock`), so tests are reproducible.
- `mvdan.cc/sh` (full Bash parser) is expected in M1 for FR-SAFE-009, with its own ADR.

## Spec references
doc 02 ADR 003, §Component responsibilities (Helper CLI row), §Open technical questions.
