# ADR 016: The PreToolUse hook fails closed; policy cache deferred

Status: Accepted (2026-11-01; M1 PR-09)

## Context
Claude Code's hooks reference (code.claude.com/docs/en/hooks, checked 2026-11-01) says a
PreToolUse command hook that times out, crashes, exits with a code other than 0 or 2, or prints
invalid JSON does **not** block the call, unless the handler sets `"onFailure": "block"`
(Claude Code v2.1.295+). ADR 004 / doc 04 require that a deny cannot be bypassed. M1 plan D9
left this to be checked in PR-09.

M1 plan §4.1 and PR-09 also listed a policy cache (compiled policy reused across hook
processes, keyed by file mtimes and hash) to keep latency under 50 ms.

## Decision
1. The PreToolUse handler in `plugin/hooks/hooks.json` sets `"onFailure": "block"`. Athel's
   minimum Claude Code version (2.1.308) supports it. A test pins it, and pins that no other
   event sets it (on Stop it has no effect; on UserPromptSubmit it would block prompts).
2. Inside the CLI, a decision never errors: if the config, policy files, home folder or state
   cannot be read, the error becomes rule 0 (`E-POLICY-CONFIG-INVALID`) and every write is
   denied (FR-SAFE-012). Reads continue through the rules.
3. An audit write failure does not change the answer (M0 behaviour kept); it is reported on
   stderr.
4. **The policy cache is deferred.** Measured with the engine on the hot path (1,000 real
   `athel hook pre-tool-use` processes, change mode): p50 14.92 ms, p95 15.88 ms, p99 16.18 ms,
   against a 50 ms budget. A cache would save a few milliseconds and add a way to decide with a
   stale policy. Added to the backlog with the trigger "p95 above 30 ms".

## Alternatives rejected
- Exit code 2 on internal errors: equivalent for PreToolUse but loses the reason text; the CLI
  already maps every error to a deny with a reason.
- Building the cache anyway: complexity and a stale-policy risk without a latency need.

## Consequences
- A missing or crashing `athel` binary blocks every tool call while the plugin is enabled. This
  is intended; the fix is to repair or disable the plugin.
- Not yet verified live in a `claude -p` session; done in PR-13's real spot check.

## Follow-up (2026-11-09): verified live
`tests/live/claude-behaviours.sh` (check A) ran real `claude -p --plugin-dir` sessions (Claude Code 2.1.311, model haiku, macOS arm64)
against a temporary folder. Results:
- A PreToolUse hook with `"onFailure": "block"` that exits 1 blocked the Bash call.
- The same hook that timed out (2 s timeout, 30 s sleep) also blocked it.
- In both cases the model saw `PreToolUse:Bash hook error: …` as the tool result.
- The control (the same failing hook without `onFailure`) let the call through.
- A copy of Athel's own plugin whose `athel` binary crashes on `pre-tool-use` also blocked the call.

The behaviour matches the docs; the decision stands unchanged.

## Spec references
Doc 04 §Decision order (rule 0), FR-SAFE-012, NFR hook latency (doc 08 M1 exit), ADR 004,
ADR 015, M1 plan §3 D9, §9 PR-09.
