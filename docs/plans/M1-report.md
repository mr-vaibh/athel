# M1 Guardrails — report

Date: 2026-11-01 (session 1). Plan: `docs/plans/M1-plan.md` (approved by the owner, all
recommendations Q1–Q12 accepted). Environment: macOS, Apple M3 Pro, Go 1.27.3, golangci-lint
2.15.0, Claude Code 2.1.308.

**Verdict: M1 is complete. All three exit criteria and the carried M0 gates are proven below**
(doc 08, Milestones, M1 row). The work is in 14 stacked PRs (#24–#37).

## 1. Exit criteria

| # | Criterion | Proof | Result |
|---|---|---|---|
| EC-1 | Red-team suite 100% blocked | `tests/redteam/` (`TestRedTeamSuite`): 431 cases in doc 04's seven groups, run through the real binary (`athel policy decide`) against a fixture workspace. Each case states its decision **and** rule | **431/431 deny with the expected rule**. Groups: prod-writes 75, cloud-writes 76, secrets 90, git 62, injection 34, obfuscation 62, scope 32 |
| EC-2 | 0 false allows | The same suite fails on any `allow`, `approve` or `none` for a forbidden case. `TestAllowListSuite`: 25 legitimate calls must get their exact allow, approve or none decision | **0 false allows**; the allow-list passes (planned edits, commits, reads, remote reads, athel CLI, guard mode) |
| EC-3 | Audit verify passes | `tests/scenarios/m1-guardrails` (`TestScenarioGuardrails`): a real change, 11 tool calls through real `athel hook` processes, one audit record per decision and per completed call, then `athel audit verify`. Editing, deleting, reordering or truncating a line must fail verify | **verify passes**; all four tamper kinds are detected (exit 17, `E-INTEGRITY-AUDIT`); no secret value in the log |
| M0 | Gates stay green | `scripts/check.sh` (build, vet, golangci-lint, race tests, per-package coverage ≥ 90%, now run without the test cache), hook latency bench, M0 resume scenario | lint 0 issues; every package ≥ 90% (total **95.8%**); hook latency over 1,000 real PreToolUse processes in change mode: p50 15.00 ms, **p95 16.17 ms**, p99 18.03 ms (budget 50 ms); resume scenario passes |

Coverage per package: adapters/claudecode 100, audit 95.8, classify 96.9, cli 92.9, clock 100,
config 97.4, digest 100, engine 99.4, envdetect 99.2, errors 100, fsutil 92.3, grants 97.0,
hooks 99.5, ids 100, lock 90.0, plan 97.6, policy 91.8, schema 92.5, scrub 97.0, shellparse 95.2,
state 95.3, cmd/athel 100, bench 94.2.

## 2. What M1 delivered

| Area | Where | Requirements |
|---|---|---|
| Hash-chained audit log, `audit verify/tail` | `cli/internal/audit` (#25) | FR-SAFE-011, EC-3 |
| Secret scrubber (tokens, keys, connection strings, entropy) | `cli/internal/scrub` (#26) | FR-SAFE-008 |
| Policy files tools/environments/git, tighten-only merge, `policy validate/show` | `cli/internal/policy` (#27) | FR-SAFE-012, FR-GIT-004 |
| Shell parser (pipes, lists, subshells, `bash -c`, eval, xargs, aliases, variables, loops, functions, scripts, make, here-docs) | `cli/internal/shellparse` (#28, ADR 014) | FR-SAFE-009/010 |
| Classifier: tiers, targets and env hints for built-ins, shell commands, git, gh, cloud CLIs and MCP | `cli/internal/classify` (#29) | doc 04 tiers, FR-SAFE-007 |
| Environment detection from flags, env vars and config files; unknown = prod | `cli/internal/envdetect` (#30) | FR-SAFE-003 |
| `plan@1`, `change@2`, hash-bound plan approval, `athel migrate` | `cli/internal/plan`, `state` (#31) | FR-SAFE-004 |
| Rules 0–13, guard and change modes, reasons, headless, `policy decide` | `cli/internal/engine` (#32, ADR 015) | doc 04 decision order, FR-SAFE-001..005 |
| Engine in PreToolUse, PostToolUse scrubbing, `onFailure: block` | `hooks`, `hooks.json` (#33, ADR 016) | FR-SAFE-001/008 |
| Human-origin grants, `/athel:approve`, `/athel:accept` | `cli/internal/grants`, skills (#34, ADR 017) | doc 02 transition rules |
| change.yaml content hash, freeze, `integrity accept` | `state` (#35) | doc 02 security baseline |
| Red-team and allow-list suites, CI step | `tests/redteam` (#36) | EC-1, EC-2 |
| End-to-end guardrails scenario | `tests/scenarios/m1-guardrails` (#37) | EC-3 |

## 3. Bugs found by the suites, fixed with tests

- **Bypass:** `git -c alias.p='push --force' p` was allowed (#36). Git aliases defined on the
  command line are now tier X; unknown git subcommands (possible config aliases) are uncertain
  (approve in a change).
- `policy decide` found the workspace from the process directory instead of `--cwd` (#36).
- Between two equal denies, the stricter environment is now reported (#36).
- File tools without a path were not malformed; Write/Edit into `.athel/` was not tier X (#32).
- The per-action decision lost its plan task; the headless reason pointed at nothing (#32).
- The M0 resume scenario compared the new `integrity.at` field (#35/#36) and passed the
  machine's full environment to hooks (#36). The gate served stale cached results for tests that
  build the binary; it now runs with `-count=1`.

## 4. Deviations and interpretations (all recorded in ADRs or PRs)

1. Rules live in `cli/internal/engine`, not `policy` (import cycle), ADR 015.
2. `athel policy decide` takes `--tool/--input/--agent` plus `--cwd`, ADR 015.
3. Policy cache deferred: p95 is 16 ms without it, ADR 016 and backlog.
4. The CLI *requires* a grant for `--by human` (stricter than the plan), ADR 017. Humans approve
   with the slash commands, not from a terminal.
5. Integrity is sealed on every CLI write, not only on transitions (#35).
6. The live `claude -p` spot check was replaced by the official docs, on the owner's
   instruction (this machine runs live production; no tool access). code.claude.com/docs/en/hooks
   (2026-11-01) confirms `onFailure: "block"`, the literal typed skill command in UserPromptSubmit,
   and `updatedToolOutput` for all tools with the tool's output shape.

## 5. Not in M1 (later milestones or backlog)

- `OwnBranches` (rule 9) is filled when Athel creates branches (M4); until then every push is
  denied.
- Repo search roots for rule 12 (M2), Praxis read allow-list (M2), PowerShell parsing (backlog),
  policy cache (backlog), PermissionRequest/Denied audit (backlog).
- ~~Live confirmation in a real Claude Code session of the three documented behaviours.~~ Done
  2026-11-09 (§6).

## 6. Follow-up: live confirmation (2026-11-09, session 3)

`tests/live/claude-behaviours.sh` (needs a logged-in `claude`; not run in CI) runs real
`claude -p --plugin-dir` sessions in a temporary workspace. Athel's hooks get a temporary HOME
there, and user settings and MCP servers are not loaded. Environment: Claude Code 2.1.311, model haiku, macOS arm64. **9/9 checks
pass:**

| Behaviour | Result |
|---|---|
| PreToolUse `onFailure: "block"` (ADR 016) | A hook that exits 1 blocks the call, and so does one that times out. The control without `onFailure` lets the call through. A crashing `athel` in Athel's own plugin blocks the call |
| UserPromptSubmit gets `/athel:approve CHG-001` literally (ADR 017) | The literal prompt arrives. Grant created and used, `plan_approved` reached, audit verify passes |
| `updatedToolOutput` replaces Bash output (FR-SAFE-008) | Claude saw `deploy token: [REDACTED:github-token]`, and the raw fake token appears nowhere in the session transcript |
