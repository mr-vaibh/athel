# M0 Skeleton — report

Date: 2026-11-01 (session 1). Plan: `docs/plans/M0-plan.md` (approved by the owner, all
recommendations Q0–Q14 accepted). Environment: macOS 27.0.1, Apple M3 Pro, Go 1.27.3,
golangci-lint 2.15.0, Claude Code 2.1.308.

**Verdict: M0 is complete. All three exit criteria are proven below** (doc 08, Milestones,
M0 row). The work sits in 21 stacked PRs (#1–#21), every one green on CI, waiting for the owner
to merge in order (agent merges are blocked by Claude Code's permission classifier).

## 1. Exit criteria

### EC-1 — A change can be created, paused, and resumed in a brand-new Claude Code session with no context lost

**Proof A — three real, separate headless Claude Code sessions** with the plugin loaded via
`claude -p … --plugin-dir ./plugin` (nothing installed into user settings; `ATHEL_HOME` pointed
to a scratch folder), in a fresh scratch workspace:

```
### SESSION A: claude -p '/athel:start # Phone number update …' --plugin-dir ./plugin
I created change **CHG-001**, titled "Phone number update". It is in `draft`.
Intake is recorded under the `start.prd-captured` checkpoint, and this session holds the lock on the change.
(after the session: change list → CHG-001 draft, step start, lock_holder null → SessionEnd released the lock)

### SESSION B: claude -p '<move CHG-001 to investigating, specified, awaiting_decisions, planned;
###   enter step plan; checkpoint plan.tasks-drafted --note "…"; never pass --session>' \
###   --plugin-dir ./plugin --allowedTools "Bash(athel:*)"
All six commands succeeded: CHG-001 is now `planned`, in the `plan` step (running), with last checkpoint `plan.tasks-drafted`.
(the checkpoint records the real Claude Code session id → the SessionStart CLAUDE_ENV_FILE export works)

### SESSION C (brand new): claude -p '/athel:resume' --plugin-dir ./plugin
I've resumed the change, and this session now holds its lock.
- **CHG-001** "Phone number update": state `planned` (not blocked)
- **Step:** `plan`, attempt 1, running
- **Last checkpoint:** `plan.tasks-drafted`. Note: "T-01 add column, T-02 endpoint, T-03 event; next: decide order of T-02/T-03"
- **Budget:** ok ($0 of $40, 0 min of 8 h, 0 of 2000 tool calls)
- **Other active changes:** none
- **Next action:** continue the `plan` step from that checkpoint by deciding whether T-02 (endpoint) or T-03 (event) goes first.
I have to stop here. Planning belongs to a later Athel milestone, and this version (M0) only supports intake. …

### history in change.yaml after session C
from: draft → investigating (seq 1), investigating → specified (2), specified → awaiting_decisions (3), awaiting_decisions → planned (4)
### hook-events.jsonl: lines from all three sessions (PreToolUse, PostToolUse, Stop, SessionEnd; SessionStart and
### UserPromptSubmit for B and C — in A the workspace did not exist yet when SessionStart ran)
```

**Proof B — scripted demo** (`tests/scenarios/m0-resume/demo.sh`, drives the binary exactly as
the plugin does, including the hooks):

```
$ tests/scenarios/m0-resume/demo.sh
…
== check: nothing was lost
OK: state, step, 4 history entries, 3 checkpoints, budget and PRD survived the pause; session 2 holds the lock and knows the next action
```

**Proof C — automated scenario test** `TestScenarioCreatePauseResume` (builds the binary,
session 1 creates/moves/checkpoints/ends, session 2 resumes; asserts change.yaml is identical
except `lock`/`updated_at`, exact next action, PRD readable; a concurrent third session only
gets read access). Runs in CI on every PR.

### EC-2 — Helper CLI test coverage is 90% or higher

Gate is **per package** (stricter than the total) and fails packages without tests:
`scripts/coverage-gate.sh 90`, run by `scripts/check.sh` and CI.

```
$ go clean -testcache && scripts/check.sh          # build, vet, golangci-lint, go test -race + coverage gate
0 issues.
ok  github.com/mr-vaibh/athel/cli/cmd/athel                    coverage: 100.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/adapters/claudecode coverage: 100.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/cli                 coverage: 92.2% of statements
ok  github.com/mr-vaibh/athel/cli/internal/clock               coverage: 100.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/config              coverage: 98.1% of statements
ok  github.com/mr-vaibh/athel/cli/internal/digest              coverage: 100.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/errors              coverage: 100.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/eventlog            coverage: 94.9% of statements
ok  github.com/mr-vaibh/athel/cli/internal/fsutil              coverage: 92.3% of statements
ok  github.com/mr-vaibh/athel/cli/internal/hookio              coverage: [no statements]
ok  github.com/mr-vaibh/athel/cli/internal/hooks               coverage: 99.4% of statements
ok  github.com/mr-vaibh/athel/cli/internal/ids                 coverage: 100.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/lock                coverage: 90.0% of statements
ok  github.com/mr-vaibh/athel/cli/internal/schema              coverage: 92.5% of statements
ok  github.com/mr-vaibh/athel/cli/internal/state               coverage: 96.4% of statements
ok  github.com/mr-vaibh/athel/tests/bench/hooklatency          coverage: 94.2% of statements
check exit=0
$ go tool cover -func=coverage.out | tail -1
total:                                  (statements)    95.2%
```

Identical numbers on CI (run https://github.com/mr-vaibh/athel/actions/runs/41310743059).
Fuzz tests: `FuzzLoadChange` (802k execs / 30 s, no failure), `FuzzParseHookInput` (30 s, no
failure); seed corpora run in every `go test`.

### EC-3 — Hook path benchmarked under 50 ms at p95

`tests/bench/hooklatency` spawns the real binary exactly like an exec-form command hook (no
shell, JSON on stdin) 1,000 times after 20 warmups, rotating five realistic PreToolUse payloads,
in a workspace where the session holds a change lock (so every call does config load and
validation, change lookup, throttled heartbeat and the audit write).

```
$ scripts/build-plugin.sh && go run ./tests/bench/hooklatency -binary plugin/bin/athel -n 1000
hook latency over 1000 runs: p50 10.65 ms, p95 12.22 ms, p99 13.09 ms, max 49.11 ms (budget p95 < 50 ms)

$ go test -run=NONE -bench=BenchmarkPreToolUseInProcess -benchmem ./cli/internal/cli/
BenchmarkPreToolUseInProcess-11   212   5673656 ns/op   2378452 B/op   33243 allocs/op

CI (GitHub ubuntu-latest, shared runner), same command, gated at p95 < 50 ms:
hook latency over 1000 runs: p50 10.84 ms, p95 26.20 ms, p99 120.84 ms, max 409.17 ms
```

This measures athel's part of the path (process start → decision → output). Claude Code's own
cost of spawning the hook is not included and cannot be measured from outside it.

## 2. What was built

| Area | Result | PR |
|---|---|---|
| Plan, context docs, local toolchain | `docs/plans/M0-plan.md`, `docs/context/*`, `scripts/env.sh` | #1 |
| Scaffold | Go module, CI (build, vet, lint, race, per-package coverage), plugin skeleton, ADRs 001/002/003/010 | #2 |
| Errors | doc 02 taxonomy codes, exit codes, JSON shape | #3 |
| Foundations | atomic writes, fsynced appends, injected clock, canonical `CHG-NNN`, SHA-256 digests | #4 |
| OS locks | `flock` with timeout for concurrent CLI calls | #5 |
| Schemas | `change@1`, `config@1`, `hook-event@1` (+ `state-event@1` in #9), validator; deps ADRs 012/013 | #6 |
| Change model | validated YAML codec, workspace, locked store | #7 |
| Event log | sequenced JSONL audit files | #8 |
| State machine | fixed 14-state table, human-only targets, blocked/cancelled rules, history with evidence hashes and origin | #9 |
| Session lock | one writer per change, 30-min TTL, throttled heartbeat, logged take-over | #10 |
| Steps, checkpoints, budgets | step enter/exit, checkpoints, 80%/100% budget states; ADR 011 | #11 |
| Config | generic tighten-only merge engine; 4 layers; effective hash | #12, #13 |
| CLI | 22 commands, `--json` everywhere, documented exit codes, `resume` brief | #14, #15, #16 |
| Hooks | Claude Code adapter (approve → ask pinned), 9 handlers, **stub PreToolUse**, hook-event log | #17, #18 |
| Plugin | `/athel:start`, `/athel:status`, `/athel:resume`; all doc 02 events wired | #19 |
| Benchmarks | in-process + end-to-end latency, CI gate | #20 |
| Resume proof | scripted demo + scenario test | #21 |
| This report | | #22 |

## 3. Deviations and interpretations (all recorded)

| Item | What and why | Where |
|---|---|---|
| Steps decoupled from transitions | Only `state transition` moves the lifecycle; `state exit --result blocked` is the one exception (Q1) | ADR 011 |
| Approval void → `planned` from plan_approved/building/ready_for_pr | Doc 05/06 raise plan changes during build and pre-PR (Q2) | ADR 011 |
| Heartbeat throttled to 5 min | Avoids a state write per tool call; TTL can never lapse while active (Q3) | ADR 011 |
| Min Claude Code version in `plugin.json` `metadata.athel` | plugin.json has no native field (docs) | ADR 001, plan §3 D1 |
| Athel `approve` → Claude Code `ask` | Claude Code's deprecated `"approve"` means allow | plan §3 D2, `TestApproveMapsToAsk` |
| `hook-events.jsonl` / `state-events.jsonl` instead of doc 04's `audit/YYYY-MM-DD.jsonl` | M0 file names set by the kickoff; record fields follow doc 04; no raw `action` text (no scrubber before M1), only `input_sha256`; `prev_hash` null | plan §6.6, schemas |
| Added commands `checkpoint`, `budget`, `resume` | Needed so checkpoints/budgets are never hand-edited and resume is one deterministic call | plan §4.3 |
| Hook handlers in `internal/hooks` (plan said `hookio`) | Keeps `hookio` a pure model with no state dependency | PR #18 |
| Logical session lock in `internal/state` (plan said `lock`) | It operates on `Change`; `lock` keeps OS file locks | PR #10 |
| Malformed hook payload → deny | Doc 04 rule 1 (Q9); every well-formed call is allowed | PR #18 |
| E-INTEGRITY deferred | With the hash-chained audit in M1 (Q10) | ADR 011 |

No spec statement was contradicted; each item fills a gap the spec left open or follows a
documented Claude Code fact.

## 4. Known limitations

- **PreToolUse is a stub that allows everything.** No safety is enforced until M1.
- `--by human` is trusted; hook-level proof that it came from a human prompt is M1 (data recorded).
- Built for macOS and Linux only (Q8); Windows is in the backlog.
- The plugin ships only a host binary built by `scripts/build-plugin.sh`; multi-platform packaging is a release task.
- CI latency tail is noisy on shared runners (p99 121 ms, p95 26 ms); the p95 gate passes but could flake if runners get slower.
- In-process hook path spends ~5 ms compiling JSON Schemas per call (backlog item for M1).
- PRs are larger than the ~400-line target in several cases (tests make up about half); each is still one concern.
- Running Claude Code version is detected with `claude --version` (2 s timeout) because hooks do not receive it.

## 5. First things M1 needs

1. Policy engine behind `athel hook pre-tool-use`, replacing the stub: tiers, ordered rules
   1–13 (doc 04), environments from `environments.yaml`, tools from `tools.yaml`, plan-task scope
   (rule 7) with the `.athel/changes/<id>/` exception.
2. Shell parser (`mvdan.cc/sh`, new dependency ADR) for FR-SAFE-009/010.
3. Secret scrubber, then store the scrubbed `action` in audit records.
4. Hash-chained audit `audit/YYYY-MM-DD.jsonl` + `athel audit verify`; migrate the M0 logs.
5. Human-origin enforcement for `plan_approved`/`done`, using the recorded `prompt_id`,
   `agent` and `origin` fields.
6. Policy files on the existing merge engine (`config.Merge` with new rule sets); invalid policy
   blocks all writes (FR-SAFE-012).
7. Red-team suite (`tests/redteam/`) at 100% blocked, 0 false allows, in CI.
8. Cache compiled schemas on the hook path to keep headroom under 50 ms.

## 6. How to reproduce everything

```sh
source scripts/env.sh
scripts/check.sh                                   # EC-2 (and every unit, fuzz-seed and scenario test)
tests/scenarios/m0-resume/demo.sh                  # EC-1 scripted
scripts/build-plugin.sh
go run ./tests/bench/hooklatency -binary plugin/bin/athel -n 1000   # EC-3
# EC-1 in real sessions (scratch workspace, never the real home):
export ATHEL_HOME="$(mktemp -d)"; cd "$(mktemp -d)" && git init -q
claude -p '/athel:start # My PRD …' --plugin-dir <repo>/plugin
claude -p '/athel:resume' --plugin-dir <repo>/plugin
```
