# ADR 014: Dependency `mvdan.cc/sh/v3` for shell parsing

Status: Accepted (2026-11-01; approved by the owner as M1 decision Q12)

## Context
FR-SAFE-009 requires shell commands to be parsed, not pattern-matched: pipes, `&&`, `;`,
subshells, `bash -c`, `eval`, `xargs`, aliases and scripts must be expanded and each part
decided. Doc 02 chose Go partly because `mvdan.cc/sh` exists. Bash grammar (quoting, here-docs,
substitutions, `$'…'`, arrays, functions) is too large to hand-write safely, and every gap is a
red-team bypass.

## Decision
Use `mvdan.cc/sh/v3` v3.14.1 (pure Go, BSD-3-Clause), importing only its `syntax` package, with
the Bash variant. `cli/internal/shellparse` walks the AST and does all semantic expansion itself
(variables, loops, aliases, functions, wrappers, nested shells, scripts, make targets), so the
dependency is only a parser, not an interpreter: nothing is ever executed.

## Alternatives rejected
- Hand-written tokenizer: misses here-docs, nested quoting and substitutions.
- `mvdan.cc/sh/v3/interp` (run in a sandbox and observe): executes code; unacceptable in a hook.
- Regex matching on the raw string: explicitly forbidden by FR-SAFE-009.

## Consequences
- Constructs the walker cannot know statically (computed command names, `$'…'` escapes, piped
  input to a shell, runtime `eval`) are reported as unparseable → deny writes, approve reads
  (FR-SAFE-010).
- PowerShell is not parsed (backlog); the policy engine treats PowerShell as unparseable.
- Fuzzing found one crash (nameless function, `()0`); fixed and kept as a seed.

## Spec references
doc 04 FR-SAFE-009, FR-SAFE-010, §Edge cases (scripts, make targets); doc 02 §Open technical questions.
