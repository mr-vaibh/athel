# ADR 039: Identical failures per stage; plans that can pass their gates; one PR-size answer per change

Status: Accepted (2026-11-26; found by M5 full eval 3, under the owner's delegation for technical
choices)

## Context
Full eval 3 ran 9 sessions. All 3 `rename-now` runs were valid. Of the other 6, one was lost to a
network outage and one missed the planted bug. Four failed because of Athel:
1. **No card after identical failures (phone-e164 runs 1–3).** The full gates failed on
   `migration` on 5 commits in a row in run 3, and no FR-BUILD-010 card was raised. A cheap run,
   which never runs the migration gate, sat green between each pair of full runs. ADR 034
   counts the latest run of each commit, so each green cheap run reset the count. The build
   could neither finish nor end blocked; the Stop hook finally blocked the change.
2. **Tasks that no build could pass (phone-e164 run 1, users-v2-profile run 1).** A migration
   task and a system-check task each covered a criterion, but held no test file in their paths.
   The ac-tests gate (FR-BUILD-003) needs a changed test naming each covered criterion, and the
   scope hook rightly kept the developer inside the paths.
3. **A migration the gate cannot run down (phone-e164 runs 2 and 3).** The plan named
   `migrations/002_add_phone_e164.sql`, following the repo's older single-file migration. The
   migration gate (doc 06: "runs up and down on a disposable database") needs a
   `NNN_name.up.sql` with its `NNN_name.down.sql`, and the paths allowed neither.
4. **The same question per repo (users-v2-profile run 1).** The gateway PR (478 lines) and then
   the orders PR (410 lines) each raised the "over git.yaml's limit" card. The answer to the
   first allowed 600 lines, but only for the gateway PR.

The harness also hid `govulncheck` from the sessions: it looked only on PATH, not in Go's
install folder. So a task adding a dependency could not finish its audit (phone-e164 run 2).

## Decision
1. **Attempts per stage.** "3 identical failures in a row" compares attempts within the stage
   of the latest run. A run of the other stage neither counts nor resets the count. The rule of
   ADR 034 stays: runs again on one commit count once.
2. **Every criterion is testable where it is covered.** `plan set` refuses a code task whose
   covered criterion is not named by a test file in its paths, or in an earlier task of the
   same repo covering the same criterion. A glob may hold a test. The test check uses the
   ac-tests gate's own matcher, with the repo's test patterns. Deferred tasks are planned
   again in their own change.
3. **Migrations come in pairs.** `plan set` refuses a task that adds a SQL migration other than
   an `.up.sql` with its `.down.sql` in the same task. It uses the migration gate's own file
   rules. The architect's prompt states both rules.
4. **One size answer per change.** A "raise the limit" answer allows that many lines in every
   PR of the change. A later PR still over the raised limit asks again, with its own size.
5. **Harness.** The eval sandbox links Go's tools from PATH, else from `go env GOBIN`, else from
   `GOPATH/bin`.

## Alternatives rejected
- **Count every run, not each commit:** eval 6 showed re-runs on one commit raising the card
  with no fix tried (ADR 034).
- **Accept single-file migrations with in-file down sections (goose, dbmate):** a real need for
  some repos, but no repo Athel has met uses them yet. Recorded in the backlog.
- **Let the gate pass a migration with no down file:** the gate is never waivable in doc 06,
  and a migration that cannot be undone is exactly what it exists to stop.

## Consequences
- A stuck task raises its card after three failed attempts, whatever runs sit between them.
- Plans that could never pass their gates are refused before approval, with a fix named.
- A multi-repo change asks about PR size once, unless a PR outgrows the raised limit.

## Spec references
Doc 06 FR-BUILD-003, FR-BUILD-010 and the migration gate; doc 04 FR-GIT-009; doc 05 planning
rules; ADR 028, ADR 032, ADR 034.
