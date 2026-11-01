# M0 Skeleton — plan

Status: **Approved by the owner on 2026-11-01 and fully built** (see `docs/plans/M0-report.md`). Deviations and interpretations are listed in the report §3 and ADR 011.

Sources: doc 01 (Master PRD), doc 02 (Master TRD), doc 08 (Implementation Plan) govern M0.
Docs 03–07 were read in full to make sure M0's state model, schemas and CLI do not block
later milestones. "§" refers to a section heading in the named doc; "p" to a PDF page.

> **M0 STUB WARNING.** In M0, `athel hook pre-tool-use` returns **ALLOW for every
> well-formed tool call**. It enforces nothing. The real policy engine (tiers, environments,
> plan scope, forbidden patterns) is milestone **M1**. The stub says so in code, in every
> hook response reason (`ATHEL M0 STUB: no policy enforcement`), and here.

---

## 1. Exit criteria (doc 08, Milestones table, M0 row)

| # | Criterion | How M0 proves it |
|---|---|---|
| EC-1 | A change can be created, paused and resumed in a brand-new Claude Code session with no context lost | Scripted demo `tests/scenarios/m0-resume/` + scenario test; Phase 4 runs `/athel:resume` in a fresh `claude --plugin-dir ./plugin` session |
| EC-2 | Helper CLI test coverage ≥ 90% | CI fails under 90% **per package** (stricter than the total) |
| EC-3 | Hook path (Claude Code → hook → `athel` → decision) under 50 ms at p95 | Go benchmark of the in-process path + end-to-end spawn timing of the real binary, ≥1,000 invocations, p50/p95/p99 reported |

## 2. Deliverables mapped to the spec

Features: **F-15 Session resume** (doc 01 Feature catalogue, P0). Requirement group:
**"Master TRD state model and workflow engine"** (doc 08). Use case **UC-10** (resume with
no lost context). Doc 02 has no numbered FR IDs for the state model, so this plan labels
M0 requirements `M0-<area>-NN` and cites the doc 02 section for each.

| ID | Deliverable | Spec source |
|---|---|---|
| M0-REPO-01 | Repo layout `plugin/ cli/ schemas/ tests/ docs/ .github/workflows/` | doc 08 §How Athel itself is engineered, Repository |
| M0-PLUG-01 | `.claude-plugin/plugin.json`: name `athel`, version, minimum tested Claude Code version | doc 02 §Plugin layout; §Cross-cutting, Compatibility |
| M0-PLUG-02 | Skills `start`, `status`, `resume` (`/athel:start`, `/athel:status`, `/athel:resume`); every state change goes through the CLI | doc 02 §Plugin layout, §Runtime model rule 5; doc 08 M0 row |
| M0-PLUG-03 | `hooks/hooks.json` wiring all doc 02 hook events to `athel hook <event>` | doc 02 §Runtime model, Hooks used |
| M0-CLI-01 | Helper CLI `athel` (Go, single static binary) | doc 02 ADR 003, §Workflow engine |
| M0-CLI-02 | `--json` on every command; documented exit codes | doc 02 §Cross-cutting, Error taxonomy |
| M0-STATE-01 | `.athel/` tree; create what M0 uses, reserve the rest | doc 02 §State model |
| M0-STATE-02 | `change.yaml` schema `athel/change@1`, validated on every read and write | doc 02 §State model |
| M0-STATE-03 | Lifecycle as a fixed transition table; illegal → `E-STATE` + audit record | doc 02 §State model Lifecycle; §Workflow engine Transition rules |
| M0-STATE-04 | Human-only transitions (`→plan_approved`, `→done`) need `--by human`; origin data recorded for M1 hook enforcement | doc 02 §Workflow engine Transition rules |
| M0-STATE-05 | Append-only history: from, to, by, at, reason, evidence file hashes | doc 02 §Workflow engine Transition rules |
| M0-STATE-06 | Checkpoints inside steps; resume restarts at last checkpoint | doc 02 §Workflow engine Resume and idempotency |
| M0-STATE-07 | Locks: one writer per change, session ID, 30-min TTL, heartbeat, read-only second session, logged take-over | doc 02 §Workflow engine Locking |
| M0-STATE-08 | Budgets (cost, time, tool calls) with 80% warning / 100% exceeded states, stored and reported only | doc 02 §Runtime model, budget paragraph |
| M0-STATE-09 | Step attempts (`step_attempts`) and Stop-loop counter | doc 02 §State model example; §Runtime model (3 blocked stops) |
| M0-CFG-01 | Four config layers: built-in (embedded), org `~/.athel/org/`, user `~/.athel/user.yaml`, project `.athel/config/` | doc 02 §Configuration and policy files |
| M0-CFG-02 | Generic merge engine: lower layers may only tighten unless the higher layer marks a rule overridable | same |
| M0-CFG-03 | `athel.yaml` schema for M0 settings; other files reserved | same, Files table |
| M0-CFG-04 | Effective config hash (for later audit records) | same, Rules |
| M0-HOOK-01 | `pre-tool-use` STUB allow | kickoff F; doc 04 FR-SAFE-001 deferred to M1 |
| M0-HOOK-02 | `SessionStart` loads active change summary; `PreCompact` writes checkpoint; `SessionEnd` releases this session's locks; `Stop`/`SubagentStop` log only, with loop protection | doc 02 Hooks used table + stop_hook_active paragraph |
| M0-HOOK-03 | One JSONL line per hook invocation in `.athel/audit/hook-events.jsonl`, forward compatible with doc 04 audit record | kickoff F; doc 04 §Audit log |
| M0-HOOK-04 | All Claude Code payload parsing/formatting in `internal/adapters/claudecode` | doc 02 §Cross-cutting, Compatibility |
| M0-QA-01 | CI: build, vet, golangci-lint, `go test -race` + coverage gate 90% | doc 02 §How Athel itself is tested level 1; doc 08 Practices |
| M0-QA-02 | Hook latency benchmark p50/p95/p99, ≥1,000 runs | doc 02 §Open technical questions (50 ms) |
| M0-QA-03 | Fuzz tests: change.yaml loading, hook JSON parsing | kickoff G |
| M0-QA-04 | Resume demo for EC-1 | doc 08 M0 exit criteria |
| M0-ADR-01 | ADRs 001 plugin, 002 state as files, 003 Go CLI, 010 worktrees, one per dependency | doc 02 §Key design decisions |

## 3. Claude Code docs vs the spec (checked 2026-11-01 against code.claude.com/docs)

| # | Topic | Spec says | Official docs say | Proposed resolution |
|---|---|---|---|---|
| D1 | Minimum Claude Code version | `plugin.json` declares it (doc 02) | No such manifest field; `metadata` is "a free-form object for your own data … Claude Code doesn't read it" (needs Claude Code ≥ 2.1.222) | Store `metadata.athel.min_claude_code_version` in plugin.json; the `athel` binary reads it from disk and SessionStart warns. Claude Code itself cannot enforce it |
| D1b | Claude Code version at runtime | SessionStart checks the running version (doc 02) | Not in hook input, no documented env var | SessionStart runs `claude --version` once with a 2 s timeout; "unknown" never blocks |
| D2 | "approve" decision | doc 04: `approve` = ask the human | `permissionDecision`: `allow`/`deny`/`ask`/`defer`. **Deprecated `"approve"` means ALLOW** | Adapter maps Athel `approve` → `ask`. Unit test pins this mapping so a regression cannot silently allow |
| D3 | Deny mechanism | "a hook that exits with a deny" (ADR 004) | Preferred: exit 0 + JSON `permissionDecision: deny`; exit 2 also blocks; other non-zero codes are **non-blocking** | Always exit 0 with JSON. Note: a Go panic exits 2, which Claude Code treats as block (fail-closed for PreToolUse; for Stop, loop protection + Claude Code's own 8-continuation cap apply) |
| D4 | Hook events | 8 events (doc 02) | Many more exist (`PermissionRequest`, `PermissionDenied`, `PostToolBatch`, `SubagentStart`, …) | M0 wires exactly the doc 02 events. `PermissionDenied`/`PermissionRequest` noted in backlog for M1 audit |
| D5 | Stop loop | Check `stop_hook_active` or a counter; 3 blocked stops → `blocked` | `stop_hook_active` is true when already continuing from a stop hook; Claude Code overrides after 8 consecutive continues | Implement both: honour `stop_hook_active`, plus Athel's own counter in change.yaml (limit 3). M0 never blocks anyway |
| D6 | Plugin subagents and hooks | "To confirm" (doc 02 open question) | Confirmed: plugin subagents ignore `hooks`, `mcpServers`, `permissionMode` | Matches spec; all enforcement stays in plugin-level `hooks/hooks.json`. Open question closed |
| D7 | Binary location | `bin/athel` in the plugin | Plugin `bin/` is on the Bash tool's PATH; `${CLAUDE_PLUGIN_ROOT}` is set for hooks but **not** in Bash tool commands | Skills call bare `athel`; hooks call `${CLAUDE_PLUGIN_ROOT}/bin/athel` |
| D8 | Hook timeouts | 50 ms decision target | Default command timeout 600 s | hooks.json sets explicit short timeouts (10 s) so a hung binary cannot stall a session |
| D9 | Local testing | not covered | `claude --plugin-dir ./plugin` loads for one session without touching settings; `claude plugin validate` | M0 uses exactly these; never installs into user settings |
| D10 | Spec naming | doc 02 diagram says `.sdadf` | — | Treated as a leftover of the old name; `.athel/` everywhere |
| D11 | Session ID inside skills | not covered | `${CLAUDE_SESSION_ID}` substitution in skill content; `CLAUDE_ENV_FILE` for SessionStart hooks | See §6.4 |
| D12 | SessionEnd | "release locks, write a session summary" | Input has `reason`; **default timeout 1.5 s** | Lock release only does small local writes; set an explicit timeout |
| D13 | `prompt_id` | not covered | Common input field: "UUID identifying the user prompt currently being processed"; present in PreToolUse input | Basis for M1's human-origin check (§6.3); that tool calls share their prompt's `prompt_id` is *likely*, to be verified in M1 |

## 4. CLI design

### 4.1 Global conventions

- `athel [--json] [--workspace DIR] <command> …`. Workspace defaults to the nearest parent
  directory containing `.athel/`, else the current directory (for `init`).
- `--json`: stdout is exactly one JSON object.
  Success `{"ok":true,"data":{…}}`; failure
  `{"ok":false,"error":{"code":"E-STATE-ILLEGAL-TRANSITION","message":"…","step":"…","next_action":"…"}}`.
  Without `--json`: short human text on stdout, errors on stderr.
- Session identity: `--session ID`, else env `ATHEL_SESSION_ID` (see §6.4).
- Actor identity for `created_by`: `identity` in `~/.athel/user.yaml`, else `unknown`.
  The CLI **never** reads git's global config (it would leak the machine owner's email here).
- Time, IDs and hashes always come from the CLI (`internal/clock`, `internal/ids`).
  Timestamps: RFC 3339 UTC with milliseconds. Hashes: `sha256:<hex>`.

### 4.2 Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unexpected internal error (bug) |
| 2 | Usage error (unknown command, bad flag) |
| 10 | `E-POLICY-*` (e.g. human-only transition attempted by athel; config loosening) |
| 11 | `E-AUTH-*` (reserved) |
| 12 | `E-TOOL-*` (reserved) |
| 13 | `E-STATE-*` (illegal transition, schema failure, lock held, not found, I/O) |
| 14 | `E-GATE-*` (reserved) |
| 15 | `E-BUDGET-*` (reserved in M0: budgets are reported, not enforced) |
| 16 | `E-CONFLICT-*` (reserved) |
| 17 | `E-INTEGRITY-*` (reserved, see Q10) |

`athel hook <event>` is special: it always exits **0** and writes Claude Code's JSON, because
any other code changes Claude Code's behaviour (D3).

### 4.3 Commands

| Command | Flags | Output (`data`) | Notes |
|---|---|---|---|
| `athel version` | | `{version, commit, schema_versions, min_claude_code}` | |
| `athel init` | | `{workspace, created:[paths]}` | Creates `.athel/{config,changes,audit}` + `config/athel.yaml` template; idempotent |
| `athel change new` | `--title T` (req), one of `--prd-file PATH` / `--prd-text TEXT` / `--prd-stdin` | change summary | Allocates next `CHG-NNN`, writes `change.yaml` (state `draft`) and `prd/v1.md`, records PRD sha256 |
| `athel change list` | `--all` (include done/cancelled) | `[{id,title,state,step,lock,updated_at}]` | Sorted by ID |
| `athel change show` | `ID` | full change.yaml as JSON | Validates on read |
| `athel state show` | `[ID]` | `{id,state,blocked_from,step,checkpoint,budget,lock,allowed_next:[…]}` | `ID` optional if exactly one active change |
| `athel state transition` | `ID --to STATE --by human\|athel --reason TEXT [--evidence PATH]…` | new state + history entry | Fixed table; human-only targets need `--by human`; evidence files are hashed |
| `athel state enter` | `ID STEP [--task T-NN]` | step record | Checks the step is legal in the current state, takes/refreshes the change lock, records start, increments `step_attempts` (see Q1) |
| `athel state exit` | `ID STEP --result ok\|blocked\|failed [--reason TEXT]` | step record | `blocked` moves the change to `blocked` (origin remembered) |
| `athel checkpoint set` | `ID NAME [--note TEXT]` | checkpoint | e.g. `build.task-07.review-round-2`; appended, last one wins on resume |
| `athel checkpoint show` | `ID` | last checkpoint + list | |
| `athel resume` | `[ID] [--takeover --reason TEXT]` | resume brief: state, step, last checkpoint, notes, lock, budget, next action | One call for the `/athel:resume` skill |
| `athel lock acquire` | `ID [--takeover --reason TEXT]` | lock | Fails `E-STATE-LOCK-HELD` (names holder) if a live lock is held by another session; `--takeover` only for **stale** locks, logged |
| `athel lock refresh` | `ID` | lock | Heartbeat |
| `athel lock release` | `ID` / `--all-mine` | released list | Only the holder's session may release |
| `athel lock status` | `[ID]` | lock(s) with `stale` flag | |
| `athel budget add` | `ID [--cost-usd X] [--minutes N] [--tool-calls N]` | budget with status | Records usage; status `ok`/`warning`(≥80%)/`exceeded`(≥100%); crossings appended as events. Nothing enforces it in M0 |
| `athel budget show` | `ID` | budget | |
| `athel config validate` | | `{valid, errors[], effective_hash}` | Non-zero exit on schema error or loosening violation |
| `athel config show` | `[--effective] [--layer builtin\|org\|user\|project]` | config + per-key origin layer | |
| `athel schema validate` | `FILE [--schema athel/change@1]` | `{valid, errors[]}` | Schema inferred from the file's `schema:` field |
| `athel hook` | `<event>` (stdin: hook JSON) | Claude Code hook JSON on stdout | Events: `session-start`, `user-prompt-submit`, `pre-tool-use`, `post-tool-use`, `post-tool-use-failure`, `subagent-stop`, `stop`, `pre-compact`, `session-end` |

Changes from the proposed surface, and why: added `checkpoint set|show` (checkpoints need a
writer that is not a hand edit), `budget add|show` (budgets must be recordable to be reported),
and `resume` (one deterministic call gives the skill everything, instead of the skill stitching
four commands). `--takeover` lives on `lock acquire` and `resume`, matching
`/athel:resume --take-over` in doc 02.

## 5. Go package layout

Module `github.com/mr-vaibh/athel` at the repo root (so `go:embed` can reach `schemas/` and
`plugin/policies/defaults/`). Entry point `cli/cmd/athel`.

| Package | Responsibility |
|---|---|
| `/` (package `athel`, `embed.go`) | Embeds `schemas/*.json` and `plugin/policies/defaults/*.yaml` |
| `cli/cmd/athel` | `main`: build the dependency graph (real clock, real FS, env), call `cli.Run`, exit with its code. Nothing else |
| `cli/internal/cli` | Command dispatch (stdlib `flag`), `--json` envelope, exit-code mapping. No business logic |
| `cli/internal/errors` | `Error{Code, Message, Step, NextAction, cause}`; code families; `ExitCode(err)`; wrap helpers |
| `cli/internal/clock` | `Clock` interface; real and fixed implementations |
| `cli/internal/ids` | `CHG-NNN` allocation (scan + max+1 under workspace lock) and parsing; accepts child IDs `CHG-042-F3` (doc 07) for later |
| `cli/internal/fsutil` | Atomic write (temp in same dir, fsync file, rename, fsync dir); append-line with fsync |
| `cli/internal/lock` | OS advisory file locks (`flock`) with timeout, used for every read-modify-write; plus the **logical** session lock model (session, since, heartbeat, TTL, stale check, takeover) |
| `cli/internal/schema` | Load embedded JSON Schemas by ID (`athel/change@1`, …), validate YAML/JSON documents, return path-precise errors |
| `cli/internal/state` | `Change` model, load/save (validate on both), transition table, history, steps, checkpoints, budgets, PRD versions, workspace tree |
| `cli/internal/config` | Layer discovery (root dirs injected; tests use temp dirs), generic merge engine with strictness kinds, effective config + hash |
| `cli/internal/eventlog` | Append-only JSONL writer (`audit/hook-events.jsonl`, `audit/state-events.jsonl`) with per-file `seq`; record envelope compatible with doc 04. M1's hash-chained `internal/audit` replaces it |
| `cli/internal/hookio` | Athel-internal, Claude-Code-independent hook model: `Event`, `Request`, `Decision` (`allow`/`deny`/`approve`), `Response`; the event handlers (stub policy, summary, checkpoint, lock release, stop guard) |
| `cli/internal/adapters/claudecode` | The **only** place that knows Claude Code: event-name mapping, input JSON parsing, output JSON formatting, `approve→ask` mapping, exit-code rules |

Rule: `hookio` and `state` never import `adapters/claudecode`; the adapter imports them.

## 6. State model

### 6.1 `.athel/` tree in M0

```
.athel/
├── config/athel.yaml            M0 (project layer)
├── changes/CHG-NNN/
│   ├── change.yaml              M0
│   ├── prd/v1.md                M0
│   └── artifacts/               M0 (created, empty)
├── audit/hook-events.jsonl      M0 (one line per hook call)
├── audit/state-events.jsonl     M0 (illegal transitions, take-overs, step start/end)
├── .lock                        M0 (workspace flock: ID allocation)
├── knowledge/  lessons/  repos/   reserved: not created in M0; names are constants in code
```

`workspace/repos/` and `workspace/cache/` (doc 02, doc 03) are reserved, created in M4.

### 6.2 `change.yaml` (`athel/change@1`), M0 fields

```yaml
schema: athel/change@1
id: CHG-001
title: Customers can update phone number
state: draft                     # lifecycle state
blocked_from: null               # origin state while state == blocked
created_by: dev@example.com      # from user.yaml identity, else "unknown"
created_at: 2026-11-01T10:00:00.000Z
updated_at: 2026-11-01T10:05:00.000Z
prd: {current: v1, versions: [{version: v1, path: prd/v1.md, sha256: "sha256:…", source: file|text}]}
step: {name: investigate, task: null, started_at: …, attempt: 1, status: running|ok|blocked|failed, ended_at: null}
step_attempts: {investigate: 1}
stop_blocks: {count: 0}          # Athel's own Stop-loop counter (limit 3, doc 02)
checkpoints:                     # append-only; last entry is where resume restarts
  - {name: investigate.sources-planned, step: investigate, at: …, note: "…", session: …}
budget:
  max_cost_usd: 40
  max_hours: 8
  max_tool_calls: 2000
  used_cost_usd: 0
  used_minutes: 0
  used_tool_calls: 0
  status: ok                     # ok | warning | exceeded  (80% / 100%)
  events: [{at: …, status: warning, dimension: cost}]
lock: {session: abc123, since: …, heartbeat_at: …, ttl_min: 30}   # or null
approvals: {plan: null}          # reserved: M3 stores {plan_hash, by, at}
waivers: []                      # reserved: M4
repos: []                        # reserved: doc 02 example, M4
change_set: null                 # reserved: doc 04, M5
parent: null                     # reserved: split changes (doc 05) and side-finding children (doc 07)
flags: []                        # reserved: FR-DISC-005 dependent-fact flags (M2)
history:                         # append-only
  - seq: 1
    from: draft
    to: investigating
    by: athel                    # human | athel
    at: …
    reason: "start investigation"
    evidence: [{path: prd/v1.md, sha256: "sha256:…"}]
    origin: {session: abc123, prompt_id: null}   # M1 uses this to enforce "human-originated"
```

Unknown fields are rejected (schema `additionalProperties: false`), so later milestones add
fields with a schema version bump and an `athel migrate` step (doc 08 Practices).

### 6.3 Lifecycle transition table (fixed in code)

Working states: `draft, investigating, specified, awaiting_decisions, planned, plan_approved,
building, ready_for_pr, in_review, verifying, uat`. Terminal: `done`, `cancelled`.

| From | To | By | Source |
|---|---|---|---|
| draft | investigating | athel/human | doc 02 lifecycle |
| investigating | specified | athel/human | same |
| specified | awaiting_decisions | athel/human | same |
| awaiting_decisions | planned | athel/human | same |
| planned | plan_approved | **human only** | doc 02 diagram (blue arrow), Transition rules |
| plan_approved | building | athel/human | lifecycle |
| building | ready_for_pr | athel/human | lifecycle |
| ready_for_pr | in_review | athel/human | lifecycle |
| in_review | verifying | athel/human | lifecycle ("after dev deploys") |
| verifying | uat | athel/human | lifecycle |
| uat | done | **human only** | doc 02 diagram (blue arrow) |
| uat | specified | athel/human | "UAT change requests" (doc 02, doc 07 FR-UAT-003) |
| verifying | building | athel/human | "failed verification" (doc 02, doc 07 FR-VER-006) |
| plan_approved, building, ready_for_pr | planned | athel/human | "plan changes after approval → planned" (doc 02, doc 05 FR-PLAN-004) — **see Q2** |
| any working state | blocked | athel/human | doc 02 ("reachable from every working state") — **see Q4** |
| blocked | the state it came from (`blocked_from`) only | athel/human | doc 02 ("returns to the state it came from") |
| any state except done and cancelled (incl. blocked) | cancelled | athel/human | doc 02 diagram ("from any state that is not Done") |

Everything else is illegal → `E-STATE-ILLEGAL-TRANSITION`, exit 13, plus a line in
`audit/state-events.jsonl`. Self-transitions are illegal. `--by athel` into a human-only
state → `E-POLICY-HUMAN-REQUIRED`, exit 10.

**Human-originated enforcement (M1).** M0 trusts `--by human`. To let M1 enforce "only from a
UserPromptSubmit-originated command, never a subagent" (doc 02), M0 records:
the `UserPromptSubmit` hook logs `prompt_id`, session and a SHA-256 of the prompt (never the
text) to `hook-events.jsonl`; `PreToolUse` logs `prompt_id`, `agent_id`/`agent_type` and the
tool input hash; history entries carry `origin.session` and `origin.prompt_id`. M1 can then
deny `athel state transition … --by human` when the calling tool use comes from a subagent or
has no matching human prompt. Written into ADR 011 (state-model notes) in Phase 3.

### 6.4 Sessions

The CLI needs the Claude Code session ID for locks. Hooks receive `session_id` in their input.
Skills run `athel` through the Bash tool, which does not get it automatically. Confirmed in the
docs (skills.md, hooks.md), two mechanisms, used in this order:

1. **Skills pass it explicitly**: skill content may use the `${CLAUDE_SESSION_ID}`
   substitution, so `SKILL.md` runs `athel --session ${CLAUDE_SESSION_ID} …`. Primary path.
2. **SessionStart exports it**: `SessionStart` hooks get `CLAUDE_ENV_FILE`; the hook writes
   `ATHEL_SESSION_ID` there so later Bash commands in the session see it. Secondary path; the
   exact file-line format and plugin-hook support are verified by a real session in PR 15.

If neither is present, mutating commands fail with `E-STATE-NO-SESSION` instead of guessing.

The running Claude Code version is **not** in hook input or any documented env var. SessionStart
therefore runs `claude --version` once (local process, 2 s timeout, no network) to compare with
the minimum tested version; on failure it says "version unknown" rather than blocking.

### 6.5 Locks

- **OS lock** (`flock`, short-lived): every read-modify-write of a `change.yaml` holds
  `changes/CHG-NNN/.lock`; ID allocation holds `.athel/.lock`. Makes concurrent CLI calls safe.
- **Session lock** (logical, in `change.yaml` `lock:`): one writer per change. Mutating
  commands from another session fail with `E-STATE-LOCK-HELD` naming the holder; reads always
  work (read-only second session). Stale = `now − heartbeat_at > ttl_min`. Take-over only of a
  stale lock, only with `--takeover --reason`, recorded in history-adjacent `state-events.jsonl`.
- **Heartbeat**: `lock refresh`, every mutating CLI call, and the `PreToolUse` hook (see Q3).
- **Reserved for later**: task locks (doc 06 "takes the task lock") and repo/worktree locks
  (doc 02) use the same lock record type under `.athel/locks/` from M4.

### 6.6 Hooks in M0

| Event (`athel hook …`) | M0 behaviour | Output |
|---|---|---|
| `session-start` | Validate config; summarise non-terminal changes (ID, title, state, step, last checkpoint, lock holder, budget status); export session ID; warn if Claude Code version is untested (if detectable) | `additionalContext` |
| `user-prompt-submit` | Attach this session's active change ID + step (doc 02); log prompt_id + prompt hash | `additionalContext` (one line) |
| `pre-tool-use` | **STUB: allow** every well-formed call; heartbeat (Q3); log | `permissionDecision: allow`, reason `ATHEL M0 STUB: no policy enforcement (real policy engine is M1)`. Malformed payload → `deny` (Q9) |
| `post-tool-use`, `post-tool-use-failure` | Log only | none |
| `subagent-stop`, `stop` | Log only; loop guard: if `stop_hook_active` is true never block; counter in `stop_blocks` | none (never blocks in M0) |
| `pre-compact` | Append checkpoint `<step>.precompact` with trigger | none |
| `session-end` | Release every lock held by this session | none |

Every invocation appends one line to `audit/hook-events.jsonl`:

```json
{"schema":"athel/hook-event@1","seq":42,"ts":"2026-11-01T10:14:03.221Z","event":"PreToolUse",
 "session":"abc123","prompt_id":"p-9","agent":"main","change":"CHG-001","step":"investigate",
 "task":null,"tool":"Bash","tool_use_id":"toolu_…","input_sha256":"sha256:…",
 "tier":null,"env":null,"decision":"allow","rule":"m0-stub","policy_hash":"sha256:…",
 "result":"ok","prev_hash":null,"duration_us":812}
```

Field names follow doc 04's audit record (`seq, ts, change, step, task, agent, tool, action,
tier, env, decision, rule, policy_hash, result, prev_hash`). Two deliberate differences:
**no raw `action` text** (M0 has no secret scrubber, so it stores `input_sha256` instead; M1 adds
the scrubbed `action`), and `prev_hash: null` (hash chain is M1).

## 7. Configuration

- Layers (highest authority first): **built-in** (embedded from
  `plugin/policies/defaults/athel.yaml`) → **org** `$ATHEL_HOME/org/athel.yaml` → **user**
  `$ATHEL_HOME/user.yaml` → **project** `.athel/config/athel.yaml`. `ATHEL_HOME` defaults to
  `~/.athel`; tests always set it to a temp dir (never the real home on this machine).
- **Merge engine** (generic, in `internal/config`, reused by M1 policy files). Every key has a
  declared *merge kind* in a registry next to its schema:
  `min` (lower is stricter, e.g. budgets), `max` (higher is stricter, e.g. coverage threshold),
  `ordered` (enum ranked loose→strict, e.g. `allow < approve < deny`), `union` (deny-lists:
  lower layers can only add), `intersect` (allow-lists: lower layers can only remove),
  `free` (preferences with no safety meaning, e.g. model per role, notification style).
  A lower layer that loosens a non-free key is a validation error `E-POLICY-CONFIG-LOOSENS`
  naming file, key and both values — unless a higher layer marked that key overridable
  (format: Q7). Unknown keys are errors.
- `athel.yaml` M0 keys: `schema: athel/config@1`, `budgets.default.{max_cost_usd,max_hours,max_tool_calls}`
  (`min`), `budgets.warn_ratio` (`min`, 0.8), `locks.ttl_min` (`min`, 30), `identity` (`free`, user
  layer), `features` (feature flags for partial work, doc 08 Rules across milestones; `free`).
  `tools.yaml`, `git.yaml`, `gates.yaml`, `environments.yaml`: **not defined in M0**; their file
  names are reserved and rejected if present with a clear "M1+" message only if they fail to parse.
- **Effective hash**: SHA-256 of the canonical JSON (sorted keys, no whitespace) of the merged
  config, shown by `config show --effective` and written into every hook-event line.

## 8. Dependencies

| Dependency | Why | Stdlib alternative | ADR |
|---|---|---|---|
| `go.yaml.in/yaml/v3` (YAML org's maintained continuation of `gopkg.in/yaml.v3`) | State and config files are YAML by spec (ADR 002); stdlib has no YAML | None in stdlib. Alternative: store JSON instead of YAML → deviates from spec | ADR 004-dep-yaml |
| `github.com/santhosh-tekuri/jsonschema/v6` (pure Go, JSON Schema 2020-12, no transitive deps) | Spec requires every state file to have a versioned JSON Schema that the CLI enforces | Hand-written Go validators + schemas as docs only: two sources of truth that drift | ADR 005-dep-jsonschema |

Not used: cobra/urfave (stdlib `flag` + a 60-line dispatcher keeps startup minimal for hooks);
testify (stdlib `testing` + table tests); `golang.org/x/sys` (only needed for Windows locking —
see Q8). `mvdan.cc/sh` arrives in M1 with its own ADR.
ADR numbers 004–009 are taken by doc 02's design decisions, so dependency ADRs start at 011+
(final numbering: 011 state model notes, 012 yaml, 013 jsonschema, more as needed).

## 9. Ordered task list (one PR each, ≤ ~400 changed lines)

Phase 2 (scaffold) — **PR-00** `feat/m0-scaffold`: layout, `go.mod`, `embed.go`, CI workflow,
`.golangci.yml`, `scripts/env.sh`, plugin skeleton (`plugin.json`, empty skill folders with
placeholder `SKILL.md`, `hooks.json` without commands), ADRs 001, 002, 003, 010, remove
committed `.DS_Store` files. Tests: `TestEmbeddedAssetsPresent`; CI runs on the PR.

| # | Branch | Content | Tests named up front |
|---|---|---|---|
| 01 | `feat/m0-errors` | `internal/errors`, exit-code map | `TestCodeFamilies`, `TestExitCodeMapping`, `TestWrapKeepsCodeAndCause`, `TestErrorJSONShape` |
| 02 | `feat/m0-fsutil-clock-ids` | atomic write, append-line, clock, CHG IDs | `TestAtomicWriteReplaces`, `TestAtomicWriteNoPartialOnFailure`, `TestAppendLineFsyncs`, `TestNextChangeID`, `TestParseChangeID` (table incl. `CHG-042-F3`, invalid forms) |
| 03 | `feat/m0-oslock` | `flock` wrapper with timeout | `TestLockExcludesSecondHolder`, `TestLockTimeout`, `TestLockReleasedOnClose`, `TestConcurrentWritersSerialised` (race) |
| 04 | `feat/m0-schema` | `internal/schema`, `schemas/change@1.json`, `config@1.json`, `hook-event@1.json` | `TestValidChangeAccepted`, `TestInvalidChangeErrorsHavePaths` (table), `TestUnknownSchemaID`, `TestAllEmbeddedSchemasCompile` |
| 05 | `feat/m0-change-model` | `Change` type, load/save validate-on-both, PRD v1 | `TestLoadValidChange`, `TestLoadRejectsSchemaViolation` (table), `TestSaveValidatesBeforeWrite`, `TestRoundTripStable`, `FuzzLoadChange` |
| 06 | `feat/m0-state-machine` | transition table, history, human-only, blocked/cancelled, evidence hashes | `TestTransitionTableExhaustive` (all 14×14 pairs), `TestHumanOnlyTargets`, `TestBlockedReturnsOnlyToOrigin`, `TestCancelledFromAnyNonTerminal`, `TestHistoryAppendOnly`, `TestEvidenceHashed`, `TestIllegalTransitionAudited` |
| 07 | `feat/m0-session-lock` | logical lock, heartbeat, stale, takeover | `TestAcquireFree`, `TestSecondSessionReadOnly`, `TestHeartbeatExtends`, `TestStaleAfterTTL`, `TestTakeoverOnlyWhenStale`, `TestTakeoverLogged`, `TestReleaseOnlyByHolder` |
| 08 | `feat/m0-steps-checkpoints-budget` | enter/exit, attempts, checkpoints, budgets | `TestEnterRecordsAttempt`, `TestEnterIllegalStepForState`, `TestExitBlockedMovesToBlocked`, `TestCheckpointAppendAndLast`, `TestBudgetThresholds` (table: 79/80/99/100/120%) |
| 09 | `feat/m0-config-merge` | generic merge engine + kinds | `TestMergeKinds` (table per kind: tighten ok, loosen error), `TestOverridableAllowsLoosen`, `TestUnknownKeyRejected`, `TestErrorNamesLayerAndKey` |
| 10 | `feat/m0-config-layers` | layer discovery, embedded defaults, athel.yaml, effective hash | `TestLayersFromTempDirs`, `TestMissingLayersOK`, `TestEffectiveHashStable`, `TestEffectiveHashChangesWithValue`, `TestNeverReadsRealHome` |
| 11 | `feat/m0-cli-core` | dispatcher, `--json`, `version`, `init`, `change new/list/show`, `state show` | `TestCLIUsageErrors`, `TestJSONEnvelope` (table), `TestInitIdempotent`, `TestChangeNewFromFileTextStdin`, `TestChangeListSorted` |
| 12 | `feat/m0-cli-state` | `state transition/enter/exit`, `checkpoint`, `resume`, `lock`, `budget` | `TestTransitionCommand*`, `TestLockCommands*`, `TestResumeBrief`, `TestExitCodesPerFamily` |
| 13 | `feat/m0-cli-config-schema` | `config validate/show`, `schema validate` | `TestConfigValidateExitCodes`, `TestConfigShowOrigins`, `TestSchemaValidateInfersSchema` |
| 14 | `feat/m0-hook-adapter` | `hookio` model + `adapters/claudecode` parse/format | `TestParseEachEvent` (recorded payloads), `TestApproveMapsToAsk`, `TestMalformedPayload`, `TestUnknownFieldsTolerated`, `FuzzParseHookInput` |
| 15 | `feat/m0-hook-handlers` | `athel hook <event>`, event log, stub, stop guard, checkpoint, release | `TestPreToolUseStubAllows`, `TestStubReasonSaysStub`, `TestStopHookActiveNeverBlocks`, `TestPreCompactCheckpoint`, `TestSessionEndReleasesOnlyOwnLocks`, `TestOneEventLinePerInvocation` |
| 16 | `feat/m0-plugin-skills` | `SKILL.md` for start/status/resume, final `hooks.json`, plugin validation | `TestHooksJSONCoversDocEvents`, `TestSkillsCallOnlyCLI` (lint: no direct writes to `.athel/`), `claude plugin validate` in CI if available |
| 17 | `feat/m0-hook-bench` | Go benchmark + `tests/bench/hooklatency` (spawn 1,000+ runs, p50/p95/p99) + CI job | `BenchmarkPreToolUseInProcess`, `TestPercentiles` |
| 18 | `feat/m0-resume-demo` | `tests/scenarios/m0-resume/` script + Go scenario test running the built binary | `TestScenarioCreatePauseResume` |
| 19 | `docs/m0-report` | `docs/plans/M0-report.md` (Phase 4) | n/a |

If any task grows past ~400 lines it is split before review, and I stop and tell you.

## 10. Risks

| Risk | Handling |
|---|---|
| Claude Code hook format changes | Single adapter package; recorded real payloads as fixtures; fuzzing; version noted in plugin |
| "approve" mis-mapped to allow (D2) | Explicit test `TestApproveMapsToAsk`; no use of deprecated fields |
| Hook latency over budget | Stdlib-only hot path; schemas compiled lazily only where needed; heartbeat throttled (Q3); benchmark in CI |
| Session ID not available to skills | Confirm docs mechanism before Phase 2; explicit failure instead of guessing |
| Concurrent CLI calls corrupt state | flock + atomic rename + race tests |
| Tests touching the borrowed machine's home | `ATHEL_HOME` injected; `TestNeverReadsRealHome`; no git global config reads |
| Coverage gate pressure tempting weak tests | Table tests on every error path; fuzzing; no gate changes |
| Spec gaps on later states | Transition table is data; changes go through ADR + tests |
| GitHub account mismatch on this machine | Blocked until the owner logs in (Q0) |

## 11. Questions for the owner

See the batch at the end of the Phase 1 report in chat; answers are logged in
`docs/context/decisions-from-chat.md` and folded back into this plan.

| ID | Question | Recommendation |
|---|---|---|
| Q0 | GitHub login on this machine | Repo-local `gh auth login` into `.tools/gh-config` + repo-local credential helper |
| Q1 | How `state enter/exit` relate to lifecycle transitions | Decouple: steps track work; only `state transition` moves lifecycle state (except `exit --result blocked`) |
| Q2 | Which states return to `planned` when an approved plan changes | `plan_approved`, `building`, `ready_for_pr` |
| Q3 | Lock heartbeat on every tool call = a state write per tool call | Throttle: refresh when the heartbeat is older than 5 minutes |
| Q4 | Is `draft` a "working state" (can it be blocked)? | Yes: every non-terminal state |
| Q5 | `in_review → building` when PR review asks for changes (not in spec) | Not in M0; decide in M4 |
| Q6 | Allow `specified → planned` when there are no decision cards? | No skip; pass through `awaiting_decisions` with "0 cards" |
| Q7 | Format for marking a config key overridable | Per-file `overridable: [dotted.keys]` list |
| Q8 | Platforms for M0 | macOS + Linux; Windows to backlog |
| Q9 | Stub on malformed hook payload | Deny (doc 04 rule 1); allow everything well-formed |
| Q10 | `E-INTEGRITY` state-file hashing | Defer to M1 with the hash-chained audit |
| Q12 | Spec folder is `docs/specs/`, CLAUDE.md says `docs/spec/` | Fix the path in CLAUDE.md |
| Q13 | Phase 4 resume demo runs `claude -p --plugin-dir ./plugin` on this machine's Claude Code install | OK (no settings touched) |
| Q14 | Two YAML/JSON Schema dependencies | Approve both |
