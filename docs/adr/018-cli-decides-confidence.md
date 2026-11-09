# ADR 018: The CLI decides every claim's confidence label

Status: Accepted (2026-11-09; agent decision under the owner's delegation, M2 PR 14)

## Context
FR-DISC-003 defines the three labels by evidence: `confirmed` needs two independent source
systems, `likely` one strong source, and `unverified` means inference, naming or docs. M2 PR 05
made the CLI refuse any label *above* what a claim's evidence allows (the ceiling, M2 plan §4.3).
Labels at or below the ceiling were left to the model.

In the first full eval run (`tests/evals/history/m2-2026-11-09T134454Z.json`), the scout labelled
an expected claim *below* its evidence in 10 of 60 runs. Doc 08's exit criterion is "confidence
labelling 100% correct". The same run also showed one case where a lower label was right: the
code calls `/legacy-profile` but the metrics show zero traffic. The ceiling counted that metric
as support because evidence had no way to say "this source is against the claim".

The owner was asked, did not want to decide technical design ("Just do whatever you think is
best"), and delegated it (docs/context/decisions-from-chat.md, 2026-11-09).

## Decision
- **Contradicting evidence.** An evidence source may carry `supports: false` (optional, default
  true). Such a source is shown to the reader but never raises the claim. Any contradiction caps
  the claim at `unverified`, and the disagreement belongs under drift (FR-DISC-006).
- **Exact labels.** `athel investigation save` refuses any label other than the one the rules give,
  in both directions, with E-STATE-CONFIDENCE. The error names the right label and why. The
  scout still writes the label, so the saved file reads on its own, but it no longer decides it.
  Labels thus come from the CLI deterministically, like IDs and timestamps (CLAUDE.md).
- **Claim direction.** For predicates that are easy to write backwards, the CLI checks the
  subject's or object's kind and refuses a mismatch with E-STATE-INVALID. For example,
  `produced_by` and `consumed_by` take an event or topic as subject, and `owned_by` takes a
  team or person as object. The run wrote `users produced_by users.updated.v1` in 4 of 6
  event-consumers runs.
- `supports` is added to `investigation@1` in place rather than as `@2`. ADR 002 asks for a
  version bump when a schema changes, but `investigation@1` is new in the unmerged M2 stack, so
  no released document or reader exists. The field is optional, so every document valid before
  stays valid.

## Alternatives rejected
- **Prompt-only:** tell the scout to label at its ceiling. The model stays the judge, so the
  100% bar stays out of reach.
- **Refuse under-labels without `supports`:** this would force a wrong `likely` on a contradicted
  claim (the dead-endpoint case).
- **The CLI writes the label itself and the scout omits it:** the label field would mean
  different things in a document before and after save, and the scout would not learn why.

## Consequences
- The scout must list every source it used and mark contradicting ones. A wrong label costs a
  save retry, not a wrong answer.
- The eval grades labels by EC-2 as written (expected claims, exact). The all-claims ceiling
  match is now 100% by construction and stays a reported metric.
- The CLI's rules about what counts as independent evidence become the single source of truth.
  Changing them changes labels everywhere, which is intended.

## Spec references
Doc 03 FR-DISC-002, FR-DISC-003, FR-DISC-006; doc 08 M2 exit criteria; M2 plan §4.3, EC-2;
ADR 002.

## Follow-up (2026-11-09, second full eval run)
The second run (`m2-2026-11-09T162052Z.json`) showed the scout marking `supports: false` too
widely: a query that returned nothing, and a repo manifest's version against the version the
cluster runs. The mechanism is unchanged. Its guidance, in the scout prompt and the template, now
says:
- A source contradicts a claim only when it observes the claim's own fact and shows it false.
- A source about another fact is drift. A manifest states the desired version, not the running
  one.
- An empty query is a gap, not evidence.
