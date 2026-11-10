# Eval history

One JSON file per eval run, `m2-<UTC start>.json`, written by `TestEvals` in `tests/evals/m2`
(`ATHEL_EVALS=1 go test -run TestEvals -v -timeout 0 ./tests/evals/m2/`). Runs make paid model
calls, so they are started by hand with the owner's approval (M2 plan Q11) and never in CI.

Each file records:

- the commit, the Claude Code version, the model and the runs per case;
- every run's grade and session cost, time and turns;
- the summary against M2's exit criteria:
  - EC-1: claim correctness at least 95%, and every trap handled;
  - EC-2: every expected claim carries its exact label, and every label sits at its evidence's
    ceiling.

Files are kept as they were written, so results can be compared across commits.
