---
description: Ad-hoc blast radius for a proposed change before any code (UC-02) - what it would affect inside and outside the repo (consumers with real traffic, data, infrastructure, people, runtime health) and a deterministic risk level. Use for /athel:blast-radius or when the developer asks what a change would break.
argument-hint: "<proposed change>"
allowed-tools: Bash(athel:*), Agent, Read
---

# /athel:blast-radius — impact of a proposed change (doc 05 §Blast radius analysis)

The risk level is computed by the athel CLI from the findings (FR-BLAST-004); never state a risk
of your own. Never read or edit `.athel/` files except the ones named here.

1. If `$ARGUMENTS` is empty, say "Usage: /athel:blast-radius <proposed change>" and stop. If
   `features.discovery` is false in `athel --json config show`, say so and stop.
2. Run `athel --json --session ${CLAUDE_SESSION_ID} ask new --kind blast --question "$ARGUMENTS"`
   and note `ASK-NNN`.
3. **Change surface (step 1–2).** Delegate to the `athel:scout` subagent: "Which interfaces,
   events, schemas, configs and services would this change touch: $ARGUMENTS?" with
   `--ask ASK-NNN`. Classify each contract change as additive, behavioural or breaking (removed or
   renamed field, changed type or meaning, stricter validation, changed status codes, ordering or
   timing).
4. **Consumers (step 3).** For each changed contract, delegate one scout question:
   endpoint_callers for an endpoint, event_consumers for an event, with traffic per environment.
5. **Data, infra, people, runtime (steps 4–7).** Delegate scout questions for data ownership and
   table sizes, new config or secrets (names only), owners and active work (service_owner,
   active_work), and current health (service_health) of every affected service.
   Each scout answer replaces `.athel/asks/ASK-NNN/investigation.md`; read it after each run.
6. **Report (step 8).** Write an `athel/blast@1` document from the scouts' answers, starting
   from `athel --json blast template` (use `ask: ASK-NNN` and `mode: adhoc`): every
   consumer with its evidence and observed traffic (or `traffic: null` when none was seen), every
   migration with its row count (null when unknown), the flags, and the required `not_affected`
   list (what was ruled out and how). Leave `risk: null` and `rules_fired: []`. Save it:
   `athel --json --session ${CLAUDE_SESSION_ID} blast save --ask ASK-NNN --summary-text "<three lines>" - <<'EOF'`
   then the YAML, then `EOF`.
7. Show the risk and the rules that fired (from the save output), the impact table and the "Not
   affected (checked)" list from `.athel/asks/ASK-NNN/blast-radius.md`, and the gaps. HIGH means
   the plan needs a rollback task; CRITICAL needs the developer's explicit sign-off (doc 05).
