# ADR 015: Guard mode and change mode for the policy decision

Status: Accepted (2026-11-01; approved by the owner as M1 decision Q1)

## Context
Doc 04's ordered rules assume an Athel change is active: rule 7 denies any write that no
approved plan task allows, and rule 10 denies writes outside the change's worktree. The plugin's
hooks run in every Claude Code session where the plugin is enabled, including sessions where
the developer is not using Athel at all. Applying rules 7–13 there would deny every edit.

## Decision
`cli/internal/engine` runs in one of two modes, chosen per call from the session's change lock:

- **Guard mode** (every session): rule 0 (invalid policy → deny writes), 1 (malformed), 2
  (unparseable: deny writes and unknown commands, approve reads), 3 (forbidden, tier X,
  unregistered MCP tools, direct writes into `.athel/`), 4 (environment, unknown = prod), 5
  (prod write), 6 (W3) and rule 12's always-deny secret paths. Anything else gets **no decision**
  (`none`): the hook prints nothing and Claude Code's own permission flow applies.
- **Change mode** (the session holds a change lock, `Store.HeldBy`): all rules 0–13.

Interpretations made while implementing doc 04 literally:

1. Rules run in doc order per action, so a W3 action whose environment cannot be resolved is
   reported as rule 5 (prod write), not rule 6. Both deny.
2. Secret-file reads are classified as tier X but reported as rule 12 with
   `E-POLICY-SECRET-PATH`, so the reason names the real cause.
3. `athel …` CLI calls are allowed by the rule 7 exception (`7x`); the CLI enforces its own
   rules (human origin, transitions, locks). Commands Athel does not know are approved (`7u`)
   instead of denied by rule 13, matching decision Q3 for unknown built-in tools.
4. Repo-level writes with no path (`git push`, `gh pr create`) are matched to the plan through
   the worktree they run in (the call's `cwd`). Plan verbs: edit, create, delete, test, commit,
   push, pr, comment.
5. Rules 8 and 11 read `tools.yaml policy.<env>`. A registered read with no rule for that
   environment is allowed (rule 11: "allow if the tool is registered with that read verb"); a
   shared write with no rule needs approval (rule 8: "approve unless tools.yaml allows").
6. A stale lock still held by the session keeps change mode; only release or take-over ends it.
7. The rules live in `cli/internal/engine`, not `cli/internal/policy` as M1 plan §5 names it:
   `classify` and `envdetect` import `policy` (the loaded files), and the engine imports both,
   so one package would be an import cycle. `policy` keeps the files; `engine` the rules.
8. `athel policy decide` also takes `--cwd`, because rules 7 and 10 depend on the directory a
   call runs in.
9. Headless (`ATHEL_HEADLESS=1`, decision Q6): approve → deny with `E-POLICY-HEADLESS`, rule
   suffixed `+headless`.

## Alternatives rejected
- Change mode everywhere: unusable outside Athel; the developer would disable the plugin.
- No guard outside a change: forbidden and prod-write calls would pass whenever no change is
  active, which is exactly when the developer is least careful.

## Consequences
- The hook (PR-09) maps `none` to "no output", `allow` to allow, `approve` to `ask`, `deny` to
  deny.
- Without a change, ordinary edits are untouched by Athel, but prod writes, secrets and tier X
  stay blocked in every session.

## Spec references
Doc 04 §Permission tiers, §Decision order (rules 0–13), FR-SAFE-001..012; doc 02 §Locking;
M1 plan §4.1–4.2, decisions Q1, Q3, Q6.

## Follow-up (2026-11-09, M2 PR 07): which mode the M2 rules use

M2 adds four rules. Each one is placed by the same test as M1's: does it change what the developer
can do in an ordinary Claude Code session?

| Rule | Mode | Why |
|---|---|---|
| Rule 12 exception: inside confirmed search roots, reads in folders with `.git` and seven read-only git commands are allowed; **any other git command in a developer's clone is denied** (`E-POLICY-DEV-CLONE`) | **change mode only** | The exception is part of rule 12, and rules 7–13 apply in change mode (decision above). Applied in guard mode, it would deny every `git commit` the developer makes with Claude Code in their own clone under `~/Developer`. FR-DISC-016 protects the clones from *Athel's* work ("Athel never changes their working tree"), which only happens under a change lock |
| The scout cannot write (`E-POLICY-AGENT-READONLY`, rule 3); its own `athel` CLI calls are exempt | both | The scout only exists during Athel work; doc 04 injection-defence layer 4 |
| Direct reads of `.athel/knowledge/` are tier X (rule 3) | both | Athel's own data; every use goes through `athel fact …` so the access log is complete (FR-DISC-004, M2 plan Q6) |
| `athel repos roots set` is human-only (rule H) | both | Like every human-origin grant (ADR 017) |

M2 plan §4.5 first said the developer-clone denial applied in guard mode too. That would break
everyday use, so it applies in change mode only. The plan has been corrected.

