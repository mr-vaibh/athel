# Athel — standing rules for every session

Athel is a spec-driven agentic development framework shipped as a Claude Code plugin,
with a Go helper CLI named `athel`. The specification in docs/specs/ (8 PDF files,
01-master-prd.pdf to 08-implementation-plan.pdf) is the source of truth. Read the relevant spec
sections before changing anything.

## Non-negotiable rules
- Work only inside this repository. Never read or write files outside it.
- No network access except: downloading required tools from their official sources
  (go.dev, github.com/cli/cli releases, golangci-lint releases) with checksum
  verification, fetching Go modules, using GitHub through the owner's gh login, and
  reading official Claude Code docs (code.claude.com/docs).
- Never commit secrets, tokens, credentials or personal data.
- Never push to main directly after the initial commit; work on feature branches and
  open PRs with `gh pr create`. Never force-push. Never rewrite pushed history.
- Never guess when the spec is unclear or contradictory: stop and ask, with options
  and a recommendation.
- Never silently deviate from the spec. If a deviation is needed, write an ADR in
  docs/adr/ and tell me in the same message.
- Never weaken, skip or delete a test, lint rule or gate to make something pass.
- Stay inside the current milestone's scope. Note out-of-scope ideas in
  docs/plans/backlog.md instead of building them.

## Engineering standards
- Go: latest stable release; standard library first; every dependency justified in an ADR.
- Test-first: write the failing test, then the code. Table-driven tests.
- Coverage: 90%+ on every package of the CLI; every error path tested.
- `go vet`, `golangci-lint run`, `go test -race ./...` must pass before every commit.
- Errors: typed, wrapped with context, carrying the error codes from doc 02
  (E-POLICY, E-AUTH, E-TOOL, E-STATE, E-GATE, E-BUDGET, E-CONFLICT, E-INTEGRITY).
- No panics in normal control flow. No global mutable state. No TODOs, dead code,
  commented-out code or debug prints in commits.
- All writes to state files are atomic (write to temp file in the same directory,
  fsync, rename). Concurrent CLI invocations must be safe (file locking).
- Deterministic behaviour: IDs, timestamps, hashes and ordering come from the CLI,
  never from the model. Time and randomness are injected so tests are reproducible.
- Small functions, precise names, comments explain why, not what.

## Git conventions
- Branches: feat/<milestone>-<short-name>, e.g. feat/m0-state-machine.
- Commits: conventional format (feat|fix|refactor|test|docs|chore(scope): summary),
  one logical change each.
- Each PR: what and why, spec sections and requirement IDs covered, how it was
  tested, anything deferred.

## When you finish a piece of work
Report: what changed, which requirement IDs it covers, commands you ran and their
results, and open questions. Keep it short and factual.

---

# Tools, documentation and session rhythm

## Tools
Required: git, Go (latest stable), gh (GitHub CLI), golangci-lint v2 (the version CI pins).
Others only if the approved plan needs them. They are installed system-wide with Homebrew;
`docs/context/environment.md` records versions and how to set up a fresh machine. Ask the
owner before installing or upgrading anything.

## Why documentation matters
Claude Code's chat history does not travel between machines or sessions. Everything
needed to continue must be written into the repository, committed and pushed.

## Required documentation (keep these current, commit and push them)
1. `docs/context/background.md`, written once and kept updated:
   - What Athel is, why it exists, who it is for, in plain words.
   - Key decisions already made by the project owner (also in the spec): Claude Code
     only; Go helper CLI; GitHub and GitHub Actions; Athel never deploys to any
     environment, it writes deploy-and-test guides and verifies read-only; humans merge
     and deploy; Facets via the Praxis read CLI, with the Raptor write CLI always
     blocked; local clones are read-only, Athel works in its own cache clones and
     worktrees; headless runs are supported; 90% coverage gate; tests follow each repo's
     existing test style; lessons always need human confirmation; it will later be used
     at the owner's company with many microservices.
   - Where everything lives: spec, plans, ADRs, journal.
2. `docs/plans/HANDOFF.md`, rewritten at the end of every session and before any break:
   - Current milestone and phase, and status of every task in the plan.
   - What was done this session, with PR links and commit hashes.
   - Work in progress: branch names, what is finished, what is not.
   - The exact next step, written so a new session can start it immediately.
   - Open questions waiting for the owner.
   - Every instruction or decision the owner gave in chat that is not yet written
     anywhere else, quoted or closely paraphrased.
   - Anything surprising learned (tool quirks, Claude Code behaviour, spec gaps).
3. `docs/journal/YYYY-MM-DD-session-N.md`, one file per session, append-only:
   - Goals of the session, what was done step by step, commands that mattered and their
     results, problems hit and how they were solved, decisions made and by whom.
4. `docs/context/decisions-from-chat.md`: an append-only log of every decision the owner
   makes in chat (date, question, answer, where it was applied). Never let a chat-only
   decision be lost.
5. `docs/context/environment.md`: tools, versions, how to set up a fresh machine from
   zero, how to run build, tests, lint and benchmarks.
6. ADRs in `docs/adr/` for design decisions.

## Session rhythm
- Start of session: run the tool check (`git --version`, `go version`, `gh --version`,
  `golangci-lint --version`), read CLAUDE.md,
  docs/context/background.md, docs/plans/HANDOFF.md, the latest journal entry and the
  current plan, check open PRs with `gh pr list`, then tell me in 5 lines where we are
  and what you will do next.
- During the session: update the journal as you go, not only at the end. Commit small
  and often; push every branch after each meaningful step.
- End of session, or when I say "wrap up": finish or safely park the current step,
  update HANDOFF.md, the journal and decisions-from-chat.md, run the full test suite,
  commit, push every branch, and confirm `git status` is clean and nothing is
  unpushed. Tell me it is safe to stop.
