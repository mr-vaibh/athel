---
description: The ship step of an Athel change - once every task is built, check the real blast radius against the plan, present side findings for the developer to decide, and mark the draft PRs ready for human review (doc 06, doc 07). Athel never merges. Use for /athel:ship CHG-NNN.
argument-hint: "CHG-NNN"
allowed-tools: Bash(athel:*)
---

# /athel:ship — from built to in review

Never read or edit `.athel/` files; use the `athel` CLI, which prints one JSON object per
command. Athel never merges and never deploys: humans do.

1. If `$ARGUMENTS` is not one change ID, say "Usage: /athel:ship CHG-NNN" and stop.
2. Run `athel --json state show $ARGUMENTS`.
   - **`building`:** run `athel --json build next $ARGUMENTS`. If a task is not done, say the
     next step is `/athel:build $ARGUMENTS` and stop. Otherwise run
     `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to ready_for_pr --by athel --reason "every task done"`.
   - **`ready_for_pr` or `in_review`:** continue.
   - **`blocked` from `ready_for_pr`:** if `athel --json decide list $ARGUMENTS --open` shows a
     card, show it with the `/athel:decide` line and stop. Otherwise run
     `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to ready_for_pr --by athel --reason "resume shipping"`
     and continue.
   - **Anything else:** say the change is not built yet and stop.

   The `ready_for_pr` guard checks that every task is done and every repo is pushed with its
   draft PR and a clean branch lint. It also compares each repo's real diff with the planned
   blast radius. If it refuses, show what it lists and stop. Drift means the spec, the blast
   radius and the plan must be updated and approved again (FR-BLAST-005).
3. **Start the step:** `athel --json --session ${CLAUDE_SESSION_ID} state enter $ARGUMENTS ship`.
4. **The actual blast radius, per repo.** Run
   `athel --json --session ${CLAUDE_SESSION_ID} blast check $ARGUMENTS <repo> --base <base_head> --head <branch>`,
   with the base commit and branch from `state show` (`repos`). It saves the actual report for
   the PR. Show the change surface in a few lines.
5. **Side findings (doc 07).** Run `athel --json finding list $ARGUMENTS`.
   - With none, say "No issues found outside this change."
   - Otherwise show the `summary` line, then each finding ranked, with its ID, category,
     severity, location, effort and fix risk, marking quick wins.
   - Ask the developer to choose by typing
     `/athel:triage $ARGUMENTS SF-1=fix_pr SF-2=ticket …` (or `all=<choice>`). Never choose for
     them.
   - Shipping does not wait: undecided findings appear in the PR body as "not decided yet".
6. **After the developer's `/athel:triage`:**
   - For each finding chosen as `fix_pr`, run
     `athel --json --session ${CLAUDE_SESSION_ID} finding fix $ARGUMENTS SF-n` and give its new
     change ID. It goes through the same steps as any change, starting with `/athel:spec`.
   - For each `ticket`, show a ticket draft for the developer to file: a title, the evidence,
     the location and the suggested fix. Athel never creates a ticket itself.
7. **Rehearse the merge order** when the change has more than one PR (`athel --json set show
   $ARGUMENTS` lists more than one repo that is not deferred):
   `athel --json --session ${CLAUDE_SESSION_ID} set verify $ARGUMENTS`. Athel merges the PRs in
   the stated order in throwaway clones and checks every state in between; it never pushes. If
   `result` is `fail`, show the failing step (what was merged, which gate or the system check,
   its last lines), end the step blocked with that as the reason, and stop: the plan or the code
   must change.
8. **Mark the PRs ready, per repo:**
   - `athel --json --session ${CLAUDE_SESSION_ID} pr update $ARGUMENTS <repo>` refreshes the
     body with the gates, waivers, findings and the rehearsal;
   - then `athel --json --session ${CLAUDE_SESSION_ID} pr ready $ARGUMENTS <repo>`. Athel refuses
     until every PR of the set is open and the rehearsal passed on exactly these heads.
9. **Hand over:**
   - `athel --json --session ${CLAUDE_SESSION_ID} state transition $ARGUMENTS --to in_review --by athel --reason "PRs ready for review"`;
   - then `athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS ship --result ok`.
10. **Show, on one screen:**
   - each PR link, in merge order, with what each merges after;
   - the gates per task, and any waivers with their expiry;
   - the side findings and their choices.

   Say the PRs wait for human review and merge. Athel will not merge or deploy.

If a step fails, show `error.code`, `error.message` and `next_action`, end the step with
`athel --json --session ${CLAUDE_SESSION_ID} state exit $ARGUMENTS ship --result blocked --reason "<the message>"`,
and stop.
