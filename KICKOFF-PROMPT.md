Read CLAUDE.md fully, especially "Environment: borrowed machine". Do the start-of-session
routine. Since this is the first session, also create docs/context/background.md and
docs/context/environment.md and set up the local tools. Then begin the work below.

You are the lead engineer building Athel, a spec-driven agentic development framework
shipped as a Claude Code plugin with a Go helper CLI called `athel`. CLAUDE.md's rules
apply to everything below.

# Source of truth
docs/spec/ contains 8 PDF documents, read in this order:
01 Master PRD · 02 Master TRD · 03 Discovery and Integrations · 04 Safety, Guardrails and Git
05 Spec, Blast Radius and Planning · 06 Build Loop, Review and Quality
07 Verification, UAT and Learning · 08 Implementation Plan

Docs 01, 02 and 08 govern this milestone. Docs 03 to 07 describe later milestones; read them
anyway, because M0's state model, schemas and CLI must not block what they need later.

# Your mission now: milestone M0 (Skeleton), and nothing beyond it
M0 builds the foundation every later milestone stands on. Its exit criteria (doc 08) are:
1. A change can be created, paused, and resumed in a brand-new Claude Code session with no
   context lost.
2. Helper CLI test coverage is 90% or higher.
3. The hook path (Claude Code → hook → `athel` binary → decision) is benchmarked under 50 ms
   at p95.

## M0 deliverables
A. Repository layout as in doc 08 ("How Athel itself is engineered"): plugin/, cli/,
   schemas/, tests/, docs/, .github/workflows/. Go code under cli/ using internal/ packages
   (state, config, schema, lock, ids, hookio, adapters/claudecode, errors).

B. Plugin skeleton (doc 02, "Plugin layout"):
   - .claude-plugin/plugin.json: name athel, version, minimum tested Claude Code version.
   - skills/start, skills/status, skills/resume, each with SKILL.md (invoked as
     /athel:start, /athel:status, /athel:resume). Skills call the CLI for every state change;
     they never edit state files directly.
   - hooks/hooks.json wiring the events listed in doc 02 ("Runtime model", hooks table) to
     `athel hook <event>`.
   - Empty but valid placeholders only where Claude Code requires them; no agents yet.

C. Helper CLI `athel` (doc 02, ADR 003 and "Workflow engine"). Proposed command surface;
   adjust in your plan if you find a better design, and explain why:
   - athel init                      create .athel/ with config and folders
   - athel change new --title ...    create a change (CHG-NNN) from a PRD path or text
   - athel change list | show <id>
   - athel state show <id>
   - athel state enter|exit <id> <step> [--result ok|blocked|failed]
   - athel state transition <id> <to> --by human|athel --reason ...
   - athel lock acquire|refresh|release|status <id>
   - athel config validate | show [--effective]
   - athel schema validate <file>
   - athel hook <event>              reads the hook JSON on stdin, writes the response JSON
   - athel version
   Every command supports --json output for skills to parse. Exit codes are documented.

D. State model (doc 02, "State model"):
   - The .athel/ tree as specified (config/, knowledge/, lessons/, repos/, audit/, changes/).
     Create only what M0 uses, but reserve the paths.
   - change.yaml with schema athel/change@1, validated on every read and write.
   - Lifecycle exactly as doc 02: draft → investigating → specified → awaiting_decisions →
     planned → plan_approved → building → ready_for_pr → in_review → verifying → uat → done,
     plus blocked (from any working state, returns to its origin) and cancelled (terminal,
     from any state that is not done). UAT change requests return to specified; a failed
     verification returns to building. Transitions are a fixed table in code; illegal ones
     fail with E-STATE.
   - Human-only transitions (into plan_approved and done, and waivers) require --by human.
     Record in M0 that hook-level enforcement of "human-originated" arrives in M1; design
     the data so M1 can enforce it.
   - Append-only transition history with from, to, by, at, reason, and the hash of the files
     that justified it.
   - Checkpoints inside steps, so resume restarts at the last checkpoint.
   - Locks: one writer per change, session ID, 30-minute TTL, heartbeat refresh, read-only
     access for a second session, explicit take-over that is logged.
   - Budgets in change.yaml (cost, time, tool calls), with 80% warning and 100% block
     states recorded. M0 stores and reports them; nothing consumes them yet.

E. Configuration (doc 02, "Configuration and policy files"):
   - Four layers: built-in defaults (embedded in the binary), org (~/.athel/org/),
     user (~/.athel/user.yaml), project (.athel/config/). On this borrowed machine, tests
     must use temporary directories for the org and user layers, never the real home folder.
   - A lower layer may only make rules stricter unless the higher layer marks a rule
     overridable: true. Implement this as a generic merge engine with tests, since M1 builds
     policy on top of it.
   - athel.yaml for M0 settings. tools.yaml, git.yaml, gates.yaml and environments.yaml are
     M1 and later; define their schemas only if needed to reserve structure.
   - The effective config's hash is computable for later audit records.

F. Hooks in M0:
   - `athel hook pre-tool-use` returns ALLOW for everything. This is a STUB: mark it loudly
     in code, in the hook's output reason, and in docs/plans/M0-plan.md. The real policy
     engine is M1.
   - SessionStart loads the active change summary; PreCompact writes a checkpoint;
     SessionEnd releases this session's locks; Stop and SubagentStop are wired but only log
     in M0. Stop hooks must carry loop protection (stop_hook_active or an attempt counter).
   - Each hook invocation appends one line to .athel/audit/hook-events.jsonl. The full
     hash-chained audit log is M1; keep the record format forward compatible.
   - Hook payload parsing and response formatting live in ONE adapter package
     (internal/adapters/claudecode), so Claude Code changes are fixed in one place.
   - Do not install the plugin into this machine's Claude Code user settings. Test it
     project-locally only.

G. Quality and CI:
   - GitHub Actions workflow: build, vet, golangci-lint, test with -race and coverage,
     fail under 90%.
   - Go benchmark plus an end-to-end timing script for the hook path, reporting p50/p95/p99
     for at least 1,000 invocations with realistic payloads.
   - Fuzz tests for change.yaml loading and hook JSON parsing.
   - A scripted demo for exit criterion 1: create a change, move it through a few states,
     end the session, start a fresh session, run /athel:resume, show nothing was lost.
   - ADRs in docs/adr/: at least 001 (plugin), 002 (state as files), 003 (Go CLI),
     010 (worktrees, recorded now even though used later), plus one ADR per dependency.

## Explicitly out of scope for M0
Policy engine, shell command parsing, environment detection, secret scrubbing, hash-chained
audit, scout and integrations, specs, blast radius, decision cards, plans, git operations,
worktrees, subagents, gates, verification, UAT, lessons. If you find M0 needs a small piece
of these, stop and ask first.

# How to work: four phases with hard stops

## Phase 1: Understand and plan (no code). STOP at the end.
1. Read CLAUDE.md and all 8 spec PDFs completely: every page, in page ranges if a file is
   long, including the diagrams. Do not skim or stop at the first pages.
2. Check the current official Claude Code docs for: plugin.json fields, plugin folder layout,
   skill format and frontmatter, hook events, hook input JSON, hook output and permission
   decision format, exit codes, stop_hook_active. List every place where the official docs
   differ from my spec, with your proposed resolution.
3. Write docs/plans/M0-plan.md containing:
   - Every M0 deliverable mapped to spec sections and requirement or feature IDs.
   - The CLI command list with flags, outputs and exit codes.
   - The Go package layout and the responsibility of each package.
   - The full state transition table.
   - Every dependency you propose, why, and its alternative in the standard library.
   - An ordered task list where each task is one reviewable PR of roughly 400 changed
     lines or less, with its tests named up front.
   - Risks and how you will handle them.
   - Questions: every ambiguity or contradiction you found in the spec, each with
     2 to 3 options and your recommendation.
4. Commit the plan and docs/context files, push, and update HANDOFF.md and the journal.
5. Show me a summary of the plan and the questions, then STOP and wait for my approval.

## Phase 2: Scaffold. STOP at the end.
Repository layout, go.mod, CI workflow, lint config, scripts/env.sh, empty plugin skeleton,
ADRs 001 to 003 and 010. Open a PR. Show me the PR and wait for my approval before Phase 3.

## Phase 3: Build, task by task from the approved plan
For each task:
1. Create a branch, write failing tests first, implement until green.
2. Run vet, lint, race tests and coverage. All must pass.
3. Review your own diff as a strict senior reviewer: spec fit, error paths, naming,
   tests, no leftovers. Fix what you find.
4. Open a PR with the description format from CLAUDE.md.
5. Update the journal and HANDOFF.md, push.
6. Give me a 3 to 5 line update, then continue to the next task unless I say stop.
Stop and ask instead of continuing if: the spec is unclear, a task grows past about 400
lines, a design decision is hard to undo, or something already merged needs to change.

## Phase 4: Prove the exit criteria
1. Run the full test suite with coverage per package.
2. Run the hook benchmark and report p50/p95/p99.
3. Run the resume demo in a fresh Claude Code session and show the output.
4. Write docs/plans/M0-report.md: what was built, each exit criterion with the exact
   commands and output that prove it, deviations from the spec (with ADR links), known
   limitations, and the first things M1 needs.
5. Tell me M0 is complete only if every exit criterion is proven. Otherwise, say which one
   is missing and why.

# How to ask me questions
Ask in batches, never one at a time. For each question: the question in one line, why it
matters, 2 to 3 options with consequences, your recommendation and why, and what you will
assume if I don't answer. Only ask when the answer changes what you build. Log every answer
I give in docs/context/decisions-from-chat.md.

Start now.
