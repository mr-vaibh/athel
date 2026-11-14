# ADR 025: Changed-lines coverage and Athel's own Go checks for complexity and mutation

Status: Accepted (2026-11-14; M4 tasks 06 and 07, under the owner's delegation for technical
choices; M4-plan G5, G14, G16)

## Context
Doc 06's gates include:
- **coverage:** changed-lines coverage ≥ 90%, and the total does not drop more than 0.5
  points;
- **complexity:** no new function with cyclomatic complexity over 15 or longer than 80
  lines;
- **mutation:** for HIGH-risk tasks, a mutation score on changed code ≥ 70%.

Go reports coverage per package and per block, not per changed line. Repos rarely ship a
complexity checker Athel could rely on, and Go has no standard mutation tester. Every new
dependency needs an ADR, and CLAUDE.md asks for the standard library first.

## Decision
- **Coverage (Go)** is one gate run, `go test -count=1 -coverprofile=<file> ./...`, with
  the profile written outside the worktree so it never lands in a commit.
  - The CLI joins the profile's blocks with `git diff -U0` against the task's baseline head.
  - A changed line inside a block that ran counts as covered, and one inside a block that did
    not run counts as uncovered. A line in no block (comments, declarations) does not count,
    and neither do test files.
  - The total is compared with the coverage the baseline recorded. So deleting dead code never
    fails a task (doc 06 edge case), and adding code without tests does.
- **Complexity (Go)** is computed by the CLI with `go/ast`. Cyclomatic complexity is counted
  the way gocyclo counts it: 1, plus each if, for, range, non-default case, select case,
  `&&` and `||`. Length is in lines.
  - Only functions in files the task changed are judged, and only if they are new or newly
    over a limit. A function already over the limit on base is not the task's.
- **Mutation (Go, task 07)** will be a small built-in mutator, also with `go/ast`. It changes
  only changed lines (`==`/`!=`, `<`/`<=`, `+`/`-`, `true`/`false`, a dropped
  `if err != nil` return), runs the package's tests for each mutant and reports the share
  killed.
- **Other languages:** these gates use the repo's own scripts when they produce an lcov file
  or a complexity report. Otherwise the gate is unavailable and blocks unless a human waives
  it, as doc 06 says for a missing tool.

## Alternatives rejected
- **A third-party coverage diff, complexity or mutation tool:** each is a dependency to vet,
  pin and keep current, for a few hundred lines of standard-library code.
- **Total coverage only:** fails tasks that delete dead code, and passes tasks that add
  untested code to a well-covered repo.

## Spec references
Doc 06 §Quality gates (coverage, complexity, mutation), the edge case "coverage drops because
of deleted dead code"; M4-plan G5, G14, G16.
