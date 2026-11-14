# ADR 023: Gate commands are judged by Athel's classifier before the CLI runs them

Status: Accepted (2026-11-14; M4 task 05, under the owner's delegation for technical choices;
M4-plan G2)

## Context
Doc 06: "Gates are commands with pass criteria … they run in the task's worktree", and "gate
evaluation by the helper CLI … deterministic, no model judgement". So the CLI runs repo commands
(`go test`, `npm run lint`) itself. A command the CLI runs passes no hook, so without a check
the CLI would be a way around the policy engine. For example, a repo's `Makefile` or
`package.json` script that deploys would run unchecked.

Running each gate command through the full engine does not fit either. Rule 7 asks for a plan
task whose verbs allow the write, which is right for an agent's edit but wrong for `go build`.
A build changes nothing in the repo and is not a task's verb.

## Decision
- **Every gate command is classified** by the same classifier as an agent's Bash call. It
  passes the same `tools.yaml` policy and `Makefile`/script resolution, and runs in the task's
  worktree as its folder.
- **It runs only when every action is:**
  - known: neither uncertain (an unknown command, or a script that could not be read) nor
    unparseable;
  - local: tier R0 or W0. A remote read (R1), any shared or production write (W1–W3) and
    anything forbidden (X) are refused;
  - writing only inside the worktree. A W0 action with a path outside it is refused, and so
    is reading a secret file, which is tier X.
- **A refused command makes its gate `unavailable`**, with the reason, and the gate blocks
  unless a human waives it (doc 06 edge case "gate tool not installed").
- **The command runs as argv, never through a shell**, with a scrubbed environment: `PATH`,
  `HOME`, the Go and Node cache variables, `TMPDIR` and the locale. It has a timeout, and its
  output is saved under the change's `artifacts/`, with secrets scrubbed.
- **The Go toolchain's checkers** (`gofmt`, `goimports`, `golangci-lint`, `staticcheck`) are
  added to the classifier as local tools.

## Alternatives rejected
- **Full engine decision:** rule 7 would refuse every build and test. Adding gate verbs to
  every plan task would widen what agents may do.
- **Trust the repo's commands:** that is the way around the policy this ADR closes.

## Spec references
Doc 06 §Quality gates, the per-task build loop steps 4–5, and the edge case "gate tool not
installed"; doc 04 edge case "a script in the repo wraps a forbidden command"; M4-plan G2.

## Follow-up (2026-11-14, M4 task 07b)
One exception to "local only": `govulncheck` may read the network. It is the security-scan
gate's dependency audit, and the owner approved it, limited to Go's vulnerability database at
`vuln.go.dev` (M4-plan Q3).
- The classifier names it a remote read (R1).
- The gate runner allows R1 for that tool alone.
- It runs only when a dependency manifest or lockfile changed since the task's baseline.

If it cannot run then, the gate is unavailable, not green: only the security owner can waive
it.

## Follow-up (2026-11-14, M4 task 07d)
The migration gate's throwaway PostgreSQL (`initdb`, `pg_ctl`, `psql`) is not a repo command,
so it is not classified. The CLI runs it, like its own git:
- in a temp folder;
- reachable only through a Unix socket there (`listen_addresses=''`), with no TCP;
- stopped and removed after the gate.

The repo's SQL files are the only repo content it runs, against that empty database.
PostgreSQL itself was approved by the owner (M4-plan Q2).
