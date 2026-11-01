# Backlog (out-of-scope ideas, noted instead of built)

| Date | Idea | Why not now | Likely milestone |
|---|---|---|---|
| 2026-11-01 | Wire `PermissionRequest` / `PermissionDenied` hook events into the audit log | Not in doc 02's hook list; useful for full audit coverage | M1 |
| 2026-11-01 | Windows support (file locking via `LockFileEx`, hook command quoting) | M0 targets macOS + Linux (pending Q8) | Release packaging |
| 2026-11-01 | Multi-platform plugin packaging (one binary per OS/arch in `plugin/bin/`, launcher) | M0 builds the host binary only | Release packaging |
| 2026-11-01 | `in_review → building` transition for PR review changes | Not in spec (pending Q5) | M4 |
| 2026-11-01 | `athel migrate` for schema upgrades | Only `change@1` exists in M0 | First schema bump |
| 2026-11-01 | Network-deny sandbox test for the CLI (doc 08 Practices) | No network code in M0; worth adding once git operations land | M4 |
| 2026-11-01 | Cache compiled JSON Schemas across hook invocations (or skip config re-validation on the hot path when the config files are unchanged) | M0 hook p95 is 13 ms end to end, ~5 ms of it in-process schema compilation; well inside 50 ms, but M1 adds shell parsing and policy evaluation | M1 |
| 2026-11-01 | PowerShell command parsing for the policy engine | M1 treats PowerShell commands as unparseable (deny writes, approve reads); Athel's users run Bash on macOS/Linux | Windows support |
| 2026-11-01 | Delete the 23 merged M0 task branches on GitHub | All contained in main; owner has not asked yet | any time |
