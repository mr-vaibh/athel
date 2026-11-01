# ADR 001: Ship Athel as a Claude Code plugin

Status: Accepted (2026-11-01, from doc 02 §Key design decisions)

## Context
Athel needs an agent loop, tool access, permissions and authentication to the developer's
logged-in tools. Building those again would be the largest and riskiest part of the system.

## Decision
Athel is a Claude Code plugin: skills hold the method, subagents do role work, hooks and a
helper CLI enforce rules. Claude Code is the reasoning and coding engine; Athel owns method,
rules, state and safety. The plugin lives in `plugin/` (layout from doc 02 §Plugin layout).

## Alternatives rejected
- Standalone orchestrator calling the model API: re-implements the agent loop, tools and auth.
- Fork of an existing framework: inherits a single-repo model that does not fit (doc 01).

## Consequences
- Athel depends on Claude Code's plugin, skill and hook contracts. All Claude Code-specific
  parsing lives in one adapter package (`cli/internal/adapters/claudecode`), and plugin.json
  records the minimum tested Claude Code version under `metadata.athel` (plugin.json has no
  native field for it; see M0-plan §3 D1).
- Plugin subagents cannot carry hooks, MCP servers or permission modes (confirmed in the
  official docs), so all enforcement sits in plugin-level `hooks/hooks.json`.
- Local testing uses `claude --plugin-dir ./plugin`; the plugin is never installed into user
  settings during development on a borrowed machine.

## Spec references
doc 02 ADR 001, §Plugin layout, §Cross-cutting Compatibility; doc 01 §Vision.
