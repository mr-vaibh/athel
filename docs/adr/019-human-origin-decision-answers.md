# ADR 019: Decision cards are answered only by the developer's own prompt

Status: Accepted (2026-11-10; M3 task 04, under the owner's delegation for technical choices)

## Context
Doc 05 FR-DEC-004 says card answers are recorded "with who answered and when". The model runs
the `athel` CLI, so a plain `athel decide answer` would let the architect answer its own cards,
or change the developer's answer on the way to the CLI. ADR 017 solved the same problem for plan
approval, and the M2 roots grant solved it for search roots: the developer's literal prompt,
seen only by the UserPromptSubmit hook, becomes a one-time grant.

## Decision
- **Prompt.** `/athel:decide CHG-NNN D-1=b D-2='free text' skip=D-3`. Every word after the change
  is an answer: an option key, free text (single-quoted, without quotes or shell-expansion
  characters), or `skip=D-n` for "should not have asked". Any other word means it is not a
  decide prompt.
- **Grant.** The hook creates a grant keyed by the change and the exact answers, sorted, so
  order does not matter but content does. The hook tells the model the exact
  `athel decide answer …` command to run.
- **Engine.** `athel decide answer` is a human-only call (rule H). It is allowed only from the
  main agent, with a valid grant for exactly these answers in this session. A subagent or a
  changed answer is denied with `E-POLICY-HUMAN-ORIGIN`.
- **CLI.** The CLI consumes the grant, audits `grant_used`, and applies the answers all or
  nothing. Answers record `by: human` and the grant ID. Free text is stored with
  `checked: false` for the architect's conflict re-check (doc 05 edge case).
- **Drafts.** Cards come in as drafts. A draft that carries its own answer, ID, presentation
  time or feedback is refused.
- **Safe defaults.** `athel decide tick` applies a `default_safe` card's default after 4 hours
  (recorded `by: default`) and lists cards due a reminder after 2 hours, using the CLI's clock.
  The decide and plan skills run it first. SessionStart does not write, so applying a default
  is always an explicit, audited CLI step.

## Alternatives rejected
- **A PreToolUse `ask`** on `decide answer`: the click approves a command, not specific answers,
  and it does not work headless.
- **A grant per change rather than per answer set**: the model could change "b" to "a" after
  the developer answered.
- **Applying defaults from SessionStart**: hooks would write change state as a side effect of
  opening a session.

## Consequences
- Answering takes one prompt from the developer, as approving a plan does.
- Red-team cases cover:
  - answering without the developer;
  - a subagent answering;
  - a command smuggled into an answer;
  - marking a card unnecessary without the developer.

## Spec references
Doc 05 FR-DEC-001…004 and the "User never answers" and "User answers … outside the options"
edge cases; doc 02 §Transition rules; ADR 017; M2 plan Q7.
