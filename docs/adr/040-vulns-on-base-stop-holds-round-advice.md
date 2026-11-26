# ADR 040: Vulnerabilities already on base; a "stop" answer holds its task; round-limit advice

Status: Accepted (2026-11-26; found by M5 full eval 4, under the owner's delegation for technical
choices)

## Context
Full eval 4 ran 9 sessions on main after #135: 5 were valid (`rename-now` 3 of 3,
`users-v2-profile` 2 of 3). ADR 039's fixes held: the identical-failure card came on time, every
migration was an up/down pair, and the PR-size question was asked once per change. Three new
Athel problems sank the other four runs:
1. **A vulnerability in Go itself was charged to the task (`phone-e164` 1–3).** The task added
   `nyaruka/phonenumbers`, so the security-scan gate ran govulncheck. govulncheck reported
   GO-2026-6859, a `net/http` flaw in this machine's Go 1.27.3, fixed in 1.27.4. The users
   service already reached it on base (`main.go` calls `http.ListenAndServe`). The gate said
   only "Vulnerability #1: GO-2026-6859", and its log kept gosec's output, not govulncheck's. No
   agent could tell which module it was, every library was already at its newest release, and
   five rounds went by. Doc 06 asks for "no new vulnerable dependency": this one was not new.
2. **A "stop" answer did not stop anything (`users-v2-profile` 1, `phone-e164` 1).** The
   round-limit card offers "a. Stop the task and look at it yourself: Athel writes nothing more
   for T until you answer again". After the developer chose (a), `/athel:build` offered the same
   task again, ran another round, hit the limit, and raised the same card: the developer was
   asked the same question twice. Since an answered card was not open, the build step could not
   end blocked on it either.
3. **The round-limit card recommended stopping while the loop made progress
   (`users-v2-profile` 1).** Rounds 1–4 fixed and verified every finding; QA raised a new, real
   one in round 5 (an encoded `..` in a proxied path), and the developer had already fixed it.
   The card still recommended "stop".

## Decision
1. **Only new vulnerabilities fail the gate.** When a task may change a dependency (its paths
   name a manifest or hold a glob), the gate baseline also runs govulncheck and keeps the IDs it
   finds (`baseline.vulns`). The gate fails only on IDs not on base. Those already there are
   named in the gate's summary ("already on base, not this change's: …"). With no audit on base
   (it could not run), every finding counts, as before.
2. **Every finding names its place and its fix.** The gate's detail reads, for example,
   "GO-2026-6859 in net/http@go1.27.3 (the Go toolchain), fixed in net/http@go1.27.4", or
   "with no fixed version".
3. **A "stop" answer holds its task.** A build card answered with an option labelled "Stop …"
   holds what it blocks, as an open card does. `build next` does not offer the task and lists the
   card under `stopped`, the question is not raised again, and the build step may end blocked on
   it. The developer resumes by answering the same card again with another option
   (`/athel:decide CHG-NNN D-n=b`), the "until you answer again" the card already promised. Any
   other answered card stays final.
4. **Advice follows progress.** At the round limit, the card recommends "allow more rounds" when
   every finding that still blocks was first raised in the last round (each earlier one was fixed
   and verified). When a finding has stayed open across rounds, it recommends "stop". The default
   if unanswered stays "stop", the safe choice.

## Alternatives rejected
- **Waive toolchain vulnerabilities automatically:** the security gate is waivable only by the
  security owner (doc 06). Here nothing needs waiving: the finding is not the change's.
- **Pin a newer Go toolchain in the task's go.mod:** that is a change to the repo's build the
  PRD did not ask for, and it would download a toolchain. The finding stays visible in the
  summary, and the developer can do it in its own change.
- **Let agents run govulncheck themselves:** the gate now gives them what they needed (the
  module and the fixed version). Agents stay inside tools.yaml.
- **Re-answer any card:** answers drive specs and plans already written. Only a stop is meant to
  be answered again.

## Consequences
- A task that adds a library in a repo already exposed to a toolchain flaw can pass, and the PR
  still shows the flaw in the gate's summary.
- Baselines of tasks that may change dependencies take one govulncheck run longer.
- "Stop" means stop: the build ends blocked, and the developer resumes it explicitly.

## Spec references
Doc 06 security-scan gate ("no new vulnerable dependency"), FR-BUILD-010; doc 05 cards; ADR 023,
ADR 031, ADR 039.
