# ADR 031: The build step ends blocked only on an open card

Status: Accepted (2026-11-21; found by full build eval 2, under the owner's delegation for
technical choices)

## Context
Doc 06's build loop leaves a task as a decision card: the round limit, a dispute twice, the
third identical failure (and, in Athel, an overlap or an oversized PR). `/athel:build` ends the
step blocked on such a card, and the developer answers it with `/athel:decide`.

`athel state exit <id> build --result blocked` accepted any reason. In full build eval 2,
`saved-addresses` run 3, after four review rounds the build session judged that a major
`security.authz` finding needed a waiver and ended the step blocked with that as its reason. No
card was open, so the developer had nothing to answer and the change stayed `blocked`. The
developer agent's prompt mentioned disputes only for findings it was sure were wrong.

## Decision
- `state exit … build --result blocked` is refused unless a card for the build is open (its step
  is `build`, or it blocks `build`). The refusal names the way on: fix the findings in the next
  round, dispute one with `athel review dispute`, or let the round limit raise its card.
- `/athel:build` says never to end the step blocked without a card.
- The developer agent disputes a finding that the approved spec rules out or that the task's
  paths cannot reach, citing the spec, instead of waiting for a waiver only the developer can
  grant.

Other steps keep their blocked exits as they are: their reasons (an expired login, a missing
tool) are things the developer fixes outside Athel.

## Alternatives rejected
- **Raise a card automatically when the build blocks with a reason:** the card would carry a
  model's free text as its question and options, which `decision@1` cards never do.
- **Let the session block and rely on the developer to notice:** this is the dead end the evals
  found.

## Consequences
- Every blocked build has a card to answer; the round limit still bounds the loop.
- A build session can no longer stop the build with a reason only. A real outside problem (a
  tool down) ends the step as `failed`, which remains allowed.

## Spec references
Doc 06 FR-BUILD-010 and §The per-task build loop, round rules 3 and 5; doc 05 §Decision cards;
ADR 029.
