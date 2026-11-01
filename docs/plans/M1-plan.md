# M1 Guardrails — plan

Status: **DRAFT, waiting for owner approval** (Phase 1 of M1). Nothing below is built yet.

Sources: doc 04 (Safety, Guardrails and Git) governs M1; doc 02 (Master TRD: hooks, workflow
engine, cross-cutting rules, testing) and doc 08 (milestones, rules) frame it; doc 03 supplies
`tools.yaml`, forbidden secret-read patterns and FR-DISC-007/012; docs 05–07 were re-checked for
what M1's data must not block (plan@1 tasks for rule 7, gate waivers, staging DB SELECT helper).
"§" = section in the named doc; "D" items = Claude Code docs differences (§3).

> **M1 replaces the M0 stub.** From M1 on, `athel hook pre-tool-use` makes real decisions. Every
> place that says "M0 STUB" (code, hook reason, docs) is removed in the PR that lands the engine.

---

## 1. Exit criteria (doc 08, Milestones table, M1 row)

| # | Criterion | How M1 proves it |
|---|---|---|
| EC-1 | Red-team suite 100% blocked | `tests/redteam/`: ≥ 200 forbidden-action variants across doc 04's seven groups; each case states the expected decision **and** rule number; CI fails on any mismatch |
| EC-2 | 0 false allows | Same suite: no forbidden case may return `allow` or `approve`; plus an allow-list suite of legitimate actions that must not be denied (so "0 false allows" is not won by denying everything blindly) |
| EC-3 | Audit verify passes | `athel audit verify` over the hash-chained log after the full scenario run; tamper tests (edit, delete, reorder, truncate a line) must fail verify |
| Carried | M0 gates stay green | 90% per package, hook p95 < 50 ms (now with parsing + policy), resume scenario |

## 2. Deliverables mapped to requirements

Features **F-01 Guardrail hooks, F-02 Audit log, F-03 Tool policy** (doc 01, P0).
Requirements **FR-SAFE-001 … FR-SAFE-012** (doc 04 §Functional requirements), plus the
M1-relevant parts of FR-DISC-007/012 (doc 03) and the forbidden-git subset of FR-GIT-004.

| ID | Requirement (short) | M1 deliverable |
|---|---|---|
| FR-SAFE-001 | Every tool call gets allow / deny / approve | Policy engine in `athel hook pre-tool-use` + `athel policy decide`; decision logged for 100% of calls |
| FR-SAFE-002 | No writes to prod; no deploy/infra change anywhere | Tier W3 + env resolution; rules 5 and 6 |
| FR-SAFE-003 | Unknown environment = prod | `envdetect` default `prod` |
| FR-SAFE-004 | Writes only if they match an approved plan task | Rule 7 against `plan.yaml` (`athel/plan@1`) bound to the approved plan hash (see Q2) |
| FR-SAFE-005 | Approve prompt shows command, env, plan task, reason (and source) | `permissionDecisionReason` template with all fields incl. "came from tool output" source (doc 04 injection layer 5; resolves the "five fields" gap) |
| FR-SAFE-006 | Approvals per action, expire with session | Claude Code `ask` per call; Athel never caches approvals (see D3) |
| FR-SAFE-007 | Never read secret values | Secret path patterns + value-reading command patterns → tier X |
| FR-SAFE-008 | Scrub all tool output before model and audit | `scrub` package; PostToolUse output replacement where Claude Code allows it (D4); audit always scrubbed |
| FR-SAFE-009 | Parse shell, don't string-match | `shellparse` on `mvdan.cc/sh/v3`: pipes, `&&`, `;`, subshells, `bash -c`, `eval`, `xargs`, wrappers, in-command aliases, scripts and make targets |
| FR-SAFE-010 | Unparseable → deny writes, approve reads | Confidence flag per atomic action |
| FR-SAFE-011 | Append-only audit for every decision and every completed call; tamper detected | `audit` package: daily `audit/YYYY-MM-DD.jsonl`, SHA-256 hash chain, `athel audit verify` |
| FR-SAFE-012 | Policy validated at session start; invalid policy disables writes | All policy files validated on every hook call (cached); invalid → every write tier denied with a clear message |
| FR-DISC-007 | Unregistered MCP tools denied | Registry lookup (see Q3) |
| FR-DISC-012 | Secret names may be listed, values never | Value-read verbs in `forbidden_patterns` for every environment |
| FR-GIT-004 (subset) | No force push, no push to protected/base, no history rewrite, no foreign branch delete, never merge | Tier X git classifier using `git.yaml` `protected` + `forbidden` |
| doc 02 Transition rules | Human-only transitions only from a human prompt, never a subagent | Human-origin grants (see Q4) |
| doc 02 Security baseline | State files hashed at each transition; mismatch → E-INTEGRITY | Deferred from M0 (Q10 of M0); implemented here (§6.6) |
| doc 02 Headless | approve becomes deny without a human | Headless detection (see D5) |

## 3. Claude Code docs vs the spec (checked 2026-11-01, code.claude.com/docs)

| # | Topic | Spec says | Official docs say | Resolution |
|---|---|---|---|---|
| D1 | Decision vocabulary | allow / deny / approve | `allow`/`deny`/`ask`/`defer`; deprecated `"approve"` = allow (hooks.md) | Unchanged from M0: Athel approve → `ask`, pinned by test |
| D2 | Precedence with the user's permission rules | the hook decides | Hooks run first; **a hook `deny` always wins**; a hook `allow` does **not** override the user's deny/ask rules (agent-sdk/permissions.md). Several hooks: deny > defer > ask > allow | Fits "safe before smart": Athel can only make things stricter than the user's own settings. Documented in ADR 015 |
| D3 | FR-SAFE-006 approvals per action, expire with session | — | `ask` prompts the user each time the hook returns it | Athel never caches approvals; every approve-tier call returns `ask` again |
| D4 | FR-SAFE-008 scrub output before the model | — | PostToolUse `updatedToolOutput` "replaces the tool's output … before it is sent to Claude", all tools, value must match the tool's output shape; not confirmed for PostToolUseFailure | Scrub by walking the `tool_response` JSON and replacing only string leaves (shape preserved). PostToolUseFailure: audit is scrubbed; error text replacement **unconfirmed**, tested empirically in PR-09 |
| D5 | Headless: approve becomes deny | — | No documented headless field or env var; in a Manual-mode `claude -p` run "a call that needs approval is denied unless something approves it"; `--bare` **skips plugin hooks entirely** | Belt and braces: Claude Code already denies unanswered prompts in `-p`; Athel also converts approve → deny when `ATHEL_HEADLESS=1` (set by Athel's CI/headless wrappers). Warn in docs: never run Athel headless with `--bare` (Q6) |
| D6 | Tool inputs | — | Built-in tools: Bash, PowerShell, Read, Write, Edit, NotebookEdit, Glob, Grep, WebFetch, WebSearch, Agent, Workflow, Skill, Monitor, AskUserQuestion, ExitPlanMode, Task* tools; `file_path` is always absolute for Read/Write/Edit; MCP `mcp__<server>__<tool>`, plugin MCP `mcp__plugin_<plugin>_<server>__<tool>` | Built-in tool table in `classify`; PowerShell treated as unparseable (deny writes, approve reads) until a parser exists (backlog); unknown tool names → Q3 |
| D7 | Subagents | every call checked | Plugin hooks fire for subagent tool calls with `agent_id`/`agent_type` | Human-origin check uses "no `agent_id`" = main agent |
| D8 | Environment of the hook | per-call env | Command hooks inherit the parent environment (`KUBECONFIG`, `AWS_PROFILE`, … available); `CLAUDE_PROJECT_DIR` exported | envdetect reads these; file roots injected for tests |
| D9 | **Hook timeout = fail open** | "a hook that exits with a deny cannot be ignored" (ADR 004) | "On `PreToolUse`, a timed-out `command` … hook doesn't block the tool call, except for hooks whose failure blocks the action" (SDK page says the opposite) | Keep p95 far below the timeout; set the handler's failure behaviour to block if `onFailure` supports it (verified empirically in PR-09); a red-team case measures that no decision path is slow; ADR 015 |
| D10 | Unconfirmed: same `prompt_id` on tool calls of a turn and inside subagents | — | Common field "UUID identifying the user prompt currently being processed"; sharing not stated | Grant check (§4.5) does not rely on it: it uses session + main agent + fresh one-time grant; `prompt_id` is recorded and compared when present |

## 4. Policy engine design

### 4.1 Pipeline per PreToolUse call (doc 04 §Decision order)

```
payload → adapter (claudecode) → Request
  → load effective policy (cached by file mtimes + hash) ── invalid → "policy invalid" mode (rule 0)
  → classify tool call into atomic actions:
        built-in tool table (Read/Glob/Grep R0, Edit/Write/NotebookEdit W0, WebFetch/WebSearch R1, …)
        Bash → shellparse → per-command classifiers (git, gh, kubectl, gcloud, aws, raptor, praxis,
               rm/mv/cp/chmod, curl/wget, package managers, athel, unknown)
        mcp__<server>__<tool> → tools.yaml registry
  → each action: {tier R0|R1|W0|W1|W2|W3|X, verb, tool id, targets (paths/branches/resources),
                  env hints (flags, env vars), confidence, source}
  → resolve environment per action (envdetect): explicit flag → env vars in command →
        live current setting (read from config files, not by spawning CLIs) → default prod
  → rules 1–13 per action; strictest result wins; first matching rule recorded
  → Decision{allow|deny|approve, rule, tier, env, code (E-POLICY-*), reason, actions}
  → headless? approve → deny
  → audit record (scrubbed action text, policy hash, decision, rule, tier, env)
```

### 4.2 The ordered rules (doc 04, implemented literally)

| Rule | Condition | Decision | Code |
|---|---|---|---|
| 0 | Policy files invalid (FR-SAFE-012) | deny any write tier; reads continue through rules 11–12 | E-POLICY-CONFIG-INVALID |
| 1 | Malformed payload | deny | E-POLICY-MALFORMED |
| 2 | Parse into atomic actions; decide each; strictest wins | — | — |
| 3 | Forbidden pattern or tier X | deny | E-POLICY-FORBIDDEN |
| 4 | Resolve environment; unknown = prod | — | — |
| 5 | Write tier in prod | deny | E-POLICY-PROD-WRITE |
| 6 | Tier W3 anywhere | deny | E-POLICY-IRREVERSIBLE |
| 7 | Write not matching an approved plan task (repo, path glob or resource, verb); exception: `.athel/changes/<id>/` via the athel CLI | deny | E-POLICY-SCOPE |
| 8 | W2: approve unless tools.yaml allows the exact verb + env | approve / allow | — |
| 9 | W1: allow only on Athel's own branch (created by this change, no foreign commits) | allow / deny | E-POLICY-FOREIGN-BRANCH |
| 10 | W0: allow inside the change worktree, deny outside | allow / deny | E-POLICY-OUTSIDE-WORKTREE |
| 11 | R1: allow if the tool is registered with that read verb, else approve | allow / approve | — |
| 12 | R0: allow inside the workspace (+ confirmed repo search roots, M2), approve outside; always deny `~/.ssh`, `~/.aws/credentials`, `~/.config/gcloud`, `.env*` … | allow / approve / deny | E-POLICY-SECRET-PATH |
| 13 | Anything else | deny | E-POLICY-DEFAULT |

Strictness order for combining actions: deny > approve > allow (same order the merge engine's
`Ordered` kind uses).

### 4.3 Shell parsing (FR-SAFE-009/010)

`mvdan.cc/sh/v3/syntax` parses the command into an AST. A walker produces atomic actions:
- Splits lists, pipelines, `&&`/`||`/`;`, subshells, command substitutions `$(…)`, process
  substitutions, function bodies, `if`/`for`/`while`/`case` bodies (a `for ctx in …; kubectl
  --context $ctx …` loop is decided for **every** listed value; unknown values → unparseable).
- Unwraps wrappers: `env`, `sudo`, `nohup`, `time`, `timeout`, `nice`, `command`, `exec`,
  `builtin`, `xargs` (the command after its flags; its input is unknown → arguments unknown).
- Recurses into `bash|sh|zsh -c "<literal>"` and `eval <literal>` (max depth 5); a non-literal
  string, `| sh`, `| bash`, `source <(…)`, base64/hex decode piped to a shell → **unparseable,
  treated as write** (deny).
- Tracks aliases and functions defined earlier in the same command (`alias k=kubectl; k delete`)
  and simple variable assignments (`C=prod-ctx; kubectl --context $C …`); anything else dynamic
  marks that word unknown.
- Output redirections (`>`, `>>`, `tee`) are W0 writes to their target path.
- Scripts: `./x.sh`, `bash x.sh`, `sh x.sh`, `make <target>` → the script file / Makefile recipe
  is read (inside the workspace only) and parsed the same way, depth ≤ 3; a script that cannot be
  read or parsed → `approve` (doc 04 edge case "unknown scripts are approve").

### 4.4 Environment detection (doc 04 §Environment detection)

`environments.yaml` (`athel/environments@1`, doc 04 format + schema line). Per call:
1. explicit flags (`--context`, `--project`, `--profile`, `--region`+account, `--kubeconfig`);
2. env vars set in the command (`KUBECONFIG=…`, `CLOUDSDK_CORE_PROJECT=…`, `AWS_PROFILE=…`);
3. live current setting, read **from files**, fresh on every call, never cached:
   kube `current-context` from `$KUBECONFIG` / `~/.kube/config`; gcloud active configuration
   (`~/.config/gcloud/active_config` → `configurations/config_<name>` `project`); AWS profile from
   the hook's environment / `~/.aws/config`; reading these is the CLI's job, never the model's;
4. `default` (prod). Several environments → strictest.
Home and file roots are injected; tests use fixture trees, never the real home.

### 4.5 Human-origin enforcement (doc 02 §Transition rules; see Q4)

Recommended design: `/athel:approve CHG-NNN` and `/athel:accept CHG-NNN` skills. The
**UserPromptSubmit** hook sees the human's literal prompt; when it is exactly one of those
commands it writes a one-time grant `{change, target, session, prompt_id, at, used:false}` to
`.athel/state/grants.jsonl` (hook-written, never model-written). PreToolUse allows
`athel state transition <id> --to plan_approved|done --by human` only when the call comes from
the main agent (no `agent_id`), in the same session, for the same change and target, with an
unused grant less than 10 minutes old; the CLI consumes the grant in the same locked write as the
transition. Every other `--by human` → deny `E-POLICY-HUMAN-ORIGIN`.

## 5. Packages

| Package | Responsibility |
|---|---|
| `cli/internal/shellparse` | Bash → atomic commands (argv, redirects, assignments, confidence, depth); no policy knowledge |
| `cli/internal/classify` | Tool call + atomic commands → actions with tier, verb, targets, env hints; built-in tool table; per-CLI classifiers; tools.yaml registry lookup |
| `cli/internal/envdetect` | environments.yaml matching + live settings from config files (injected FS root) |
| `cli/internal/policy` | Policy files (`tools@1`, `environments@1`, `git@1`) loaded through `config.Merge` with their rule sets; rules 0–13; `Decide()`; reason text (FR-SAFE-005) |
| `cli/internal/scrub` | Secret patterns + entropy → `[REDACTED:kind]` |
| `cli/internal/audit` | Hash-chained daily log, `Append`, `Verify`; replaces `eventlog` users |
| `cli/internal/grants` | Human-origin grants (create from UserPromptSubmit, check + consume) |
| existing `hooks`, `cli`, `state`, `config`, `schema` | Wire the engine in; `athel policy decide`, `athel audit verify`, `athel migrate`; remove the M0 stub |

## 6. Data and files

### 6.1 New policy files (all with a `schema:` line; merged through `config.Merge`)

| File | Schema | Merge kinds (lower layers may only tighten) |
|---|---|---|
| `tools.yaml` | `athel/tools@1`: `tools:` list keyed by `id` (doc 03 entry fields: kind, match, purpose, read, write, environments.detect, policy.<env>.{read,write}, forbidden_patterns, playbook, auth_check) | `read` Intersect, `write` Union, `forbidden_patterns` Union, `policy.*.*` Ordered allow<approve<deny, others Free |
| `environments.yaml` | `athel/environments@1` (doc 04 format) | pattern lists Union per env (a context can only become *more* prod), `default` Ordered dev<staging<prod |
| `git.yaml` | `athel/git@1` (doc 04 format, full schema; M1 uses `base_branch`, `protected`, `forbidden`) | `protected` Union, `forbidden` Union, rest Free/Min as listed in the schema notes |

Built-in defaults in `plugin/policies/defaults/`: `tools.yaml` (git, gh, kubectl, gcloud, aws,
praxis read-only placeholder, raptor deny-all, common MCP servers unregistered by default),
`environments.yaml` (`default: prod`, no patterns), `git.yaml` (doc 04 defaults).

### 6.2 Audit log (`athel/audit@1`, doc 04 §Audit log)

`.athel/audit/YYYY-MM-DD.jsonl`, one record per decision (PreToolUse), per completed call
(PostToolUse / PostToolUseFailure), per other hook event, and per CLI state event (transitions,
rejections, step start/end, lock take-over, grant use). Fields: doc 04's `seq, ts, change, step,
task, agent, tool, action (scrubbed, ≤ 2 KB), tier, env, decision, rule, policy_hash, result,
prev_hash` + `kind`, `session`, `prompt_id`, `tool_use_id`, `code`, `idempotency_key` (doc 02
§Resume and idempotency; reserved), `hash`.
Chain: `hash = sha256(canonical JSON of the record without "hash")`; `prev_hash` = previous
record's `hash`; global `seq` across days; the first record of a day links to the last record of
the previous file; genesis `prev_hash = "sha256:" + 64×"0"`. `athel audit verify [--from DATE]`
recomputes every hash and link and reports the first break. M0 files (`hook-events.jsonl`,
`state-events.jsonl`) are no longer written; kept for history.

### 6.3 Plan and approval (for rule 7; see Q2)

`athel/plan@1` exactly as doc 05 §The change plan (tasks with repo, paths, verbs, covers,
depends_on, size, gates, risk_notes, rollback, env_actions) and `change@2` adding
`approvals.plan: {hash, by, at, grant}`; `athel migrate` upgrades `change@1` → `change@2`
(doc 08 Practices). M1 builds the matcher and the binding; M3 builds plan authoring and approval.

### 6.4 E-INTEGRITY (deferred from M0)

Each transition records `state_hash` (sha256 of change.yaml content excluding `lock`,
`updated_at`, `state_hash`). On load, a mismatch with the recomputed hash → `E-INTEGRITY-STATE`
(exit 17) and the change is frozen until a human runs `athel integrity accept <id> --reason`
(logged). Hand edits of change.yaml are therefore detected, as doc 02 requires.

## 7. CLI additions

| Command | Purpose |
|---|---|
| `athel policy decide --tool T --input JSON [--session S] [--agent A]` | Same decision as the hook, for debugging and red-team runs (doc 04 names it) |
| `athel policy validate` / `show [--effective]` | Validate and show tools/environments/git policy with origins and hash |
| `athel audit verify [--from YYYY-MM-DD]` | Verify the hash chain (EC-3) |
| `athel audit tail [-n N]` | Read recent records (scrubbed) |
| `athel migrate [--dry-run]` | Upgrade state files to current schema versions |
| `athel integrity accept ID --reason R` | Human acknowledgement after a detected hand edit |

## 8. Dependencies

| Dependency | Why | Stdlib alternative | ADR |
|---|---|---|---|
| `mvdan.cc/sh/v3` v3.14.1 (pure Go, BSD-3) | Complete Bash parser; doc 02 chose Go partly for it; FR-SAFE-009 forbids string matching | Hand-written tokenizer: misses heredocs, quoting, substitutions — exactly the bypasses the red team targets | ADR 014 |

No other new dependency. Glob matching for paths uses stdlib `path.Match` plus a small `**`
matcher (tested), not a library.

## 9. Ordered task list (one PR each, ~400 changed lines; tests named up front)

| # | Branch | Content | Tests named up front |
|---|---|---|---|
| 01 | `feat/m1-audit-log` | hash-chained daily audit log + `audit verify`; hooks and state events write to it | `TestChainLinksAcrossDays`, `TestVerifyDetectsEditDeleteReorderTruncate`, `TestGenesis`, `TestConcurrentAppendsChainIntact`, `TestAuditRecordSchema` |
| 02 | `feat/m1-scrub` | secret scrubber | `TestScrubKnownTokens` (table: AWS, GCP SA key, GitHub, Slack, JWT, private key block, conn strings), `TestScrubEntropyNearKeywords`, `TestNoFalsePositiveOnHashes`, `FuzzScrub` |
| 03 | `feat/m1-policy-files` | schemas `tools@1`, `environments@1`, `git@1`; built-in defaults; merge rule sets; `athel policy validate/show` | `TestPolicyMergeTightenOnly` (table per file), `TestInvalidPolicyFile`, `TestBuiltinPoliciesValid` |
| 04 | `feat/m1-shellparse` | `shellparse` (ADR 014) | `TestSplitsListsPipesSubshells`, `TestRecursesBashCAndEval`, `TestWrappersUnwrapped`, `TestInlineAliasesAndVars`, `TestLoopsExpanded`, `TestDynamicIsUnparseable`, `TestRedirectTargets`, `FuzzParse` |
| 05 | `feat/m1-classify` | built-in tool table, per-CLI classifiers, registry lookup, tiers | `TestBuiltinToolTiers`, `TestGitClassifier` (force push variants, protected push, merge, reset+push, branch delete), `TestKubectlGcloudAwsVerbs`, `TestSecretReads`, `TestRaptorAlwaysW3`, `TestUnregisteredMCP` |
| 06 | `feat/m1-envdetect` | environments.yaml + live settings from files | `TestResolutionOrder`, `TestUnknownIsProd`, `TestMultipleEnvsStrictest`, `TestKubeconfigParsing`, `TestGcloudActiveConfig`, `TestNeverReadsRealHome` |
| 07 | `feat/m1-plan-binding` | `plan@1`, `change@2` + `athel migrate`, path/resource/verb matcher | `TestPlanSchema`, `TestMigrateChange1To2`, `TestTaskMatch` (globs incl. `**`), `TestApprovalVoidOnPlanHashChange` |
| 08 | `feat/m1-policy-engine` | rules 0–13, strictest combination, reasons, `athel policy decide` | `TestRuleOrder` (one case per rule), `TestStrictestWins`, `TestAthelStateExceptionRule7`, `TestReasonHasAllFields`, `TestHeadlessApproveBecomesDeny` |
| 09 | `feat/m1-hook-wiring` | PreToolUse uses the engine (stub removed), PostToolUse scrub + completed-call audit, policy cache, latency kept < 50 ms | `TestPreToolUseDecisions` (CLI level), `TestPostToolUseScrubs`, `TestPolicyCacheInvalidatesOnEdit`, bench gate |
| 10 | `feat/m1-human-origin` | grants + `/athel:approve`, `/athel:accept` skills | `TestGrantFromHumanPromptOnly`, `TestSubagentCannotUseGrant`, `TestGrantOneTimeAndExpires`, `TestByHumanWithoutGrantDenied` |
| 11 | `feat/m1-integrity` | state hash per transition, `E-INTEGRITY-STATE`, `integrity accept` | `TestHandEditDetected`, `TestAcceptLogged`, `TestFrozenUntilAccepted` |
| 12 | `feat/m1-redteam` | `tests/redteam/` ≥ 200 cases (7 groups) + allow-list suite + CI job | `TestRedTeamSuite` (per-case decision+rule), `TestAllowListSuite`, `TestCoveragePerGroup` |
| 13 | `feat/m1-scenarios` | scenario: forbidden attempts during a change → denied, audited, `audit verify` passes; real `claude -p` spot check | `TestScenarioGuardrails` |
| 14 | `docs/m1-report` | M1 report, handoff | — |

## 10. Risks

| Risk | Handling |
|---|---|
| Hook latency with parsing + policy | Policy cache keyed by file mtimes; schemas compiled once per process; no subprocesses on the hot path; bench gate stays at p95 < 50 ms |
| False allows through shell tricks | Parser-based, depth limits, dynamic = unparseable = deny writes; red-team suite with obfuscation group; fuzzing |
| Over-blocking makes Claude Code unusable | Q1 (scope of enforcement) + allow-list suite |
| Reading the developer's cloud config on this borrowed machine | All file roots injected; tests use fixtures; `TestNeverReadsRealHome` |
| Claude Code behaviour gaps (D2–D8) | Adapter-only changes; compatibility tests on recorded payloads |
| Spec gaps (hash algorithm, five fields, MCP rule conflict) | Questions below; ADR for each choice |

## 11. Questions for the owner

Answers go to `docs/context/decisions-from-chat.md`.

| ID | Question | Recommendation |
|---|---|---|
| Q1 | Where does the full policy apply? Rules 7, 9, 10 deny every write outside an approved plan, a change worktree and Athel's own branch. None of those exist before M3/M4, so with the plugin enabled Claude Code could not edit any file at all | **Two modes.** *Guard mode* (always, every session): rules 0–6, 12's always-deny paths, tier X, unregistered MCP — prod, secrets, infra, forbidden git are blocked everywhere. *Change mode* (only while the session holds an Athel change lock): all 13 rules. Outside change mode, non-forbidden calls get no decision from Athel and follow Claude Code's normal permission flow. ADR 015 |
| Q2 | Rule 7 needs plans, which M3 authors | Define `plan@1` and `change@2` (approval bound to the plan hash) + `athel migrate` now; implement the matcher now; with no approved plan, change mode denies writes outside `.athel/` (literal spec behaviour; doc 08 says Athel builds Athel read-only until M4) |
| Q3 | Unregistered tools: doc 02 says "writes forbidden, reads allowed after approval"; doc 03 FR-DISC-007 says unregistered MCP tools are denied | Unregistered **MCP** tool → deny (cannot tell read from write; FR-DISC-007 is the stricter, testable rule). Unknown **built-in** tool name (a new Claude Code tool) → approve, so a Claude Code upgrade never silently allows or hard-breaks; unknown **CLI** command → classified by known read verbs, else unparseable rules |
| Q4 | How M1 proves `--by human` came from a human prompt | One-time grants written by the UserPromptSubmit hook only for the literal prompts `/athel:approve CHG-NNN` and `/athel:accept CHG-NNN` (§4.5) |
| Q5 | Audit log: hash details are not in the spec (algorithm, what is hashed, genesis, day boundaries, seq scope) | SHA-256 of the canonical JSON record without its own `hash`; global seq; first record of a day links to the previous day's last; genesis all-zero; M0's `hook-events.jsonl`/`state-events.jsonl` stop being written and are replaced by the daily files (§6.2) |
| Q6 | Headless detection | `ATHEL_HEADLESS=1` (set by Athel's headless wrappers and CI) + rely on Claude Code denying unanswered prompts in `-p`; document "never `--bare`" |
| Q7 | `tools.yaml` format: doc 03 shows a bare top-level list with no `schema` line | Wrap it: `schema: athel/tools@1` + `tools: [ … ]` (same entry fields), so ADR 002's "every file has a versioned schema" holds |
| Q8 | `git.yaml` is doc 04 / M4, but the red-team "Git" group needs protected branches and the forbidden list now | Define the full `git@1` schema in M1; enforce only `protected`, `base_branch` and `forbidden` (push/merge/rewrite rules) now; the rest is used from M4 |
| Q9 | Live environment setting: spawning `kubectl config current-context` per call costs 50–100 ms | Read the kubeconfig / gcloud active configuration / AWS profile **files** directly, fresh each call; unknown → prod |
| Q10 | E-INTEGRITY (deferred from M0) | Implement in M1 (§6.4) with `athel integrity accept` for deliberate human edits |
| Q11 | Red-team corpus | YAML cases in `tests/redteam/cases/*.yaml` (`input`, `expect: {decision, rule}`, `group`), ≥ 200 variants from templates (each forbidden action × wrappers × obfuscations × environments), plus an allow-list suite of legitimate calls |
| Q12 | New dependency `mvdan.cc/sh/v3` v3.14.1 | Approve (ADR 014) |

