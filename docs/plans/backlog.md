# Backlog (out-of-scope ideas, noted instead of built)

| Date | Idea | Why not now | Likely milestone |
|---|---|---|---|
| 2026-11-01 | Wire `PermissionRequest` / `PermissionDenied` hook events into the audit log | Not in doc 02's hook list; useful for full audit coverage | M1 |
| 2026-11-01 | Windows support (file locking via `LockFileEx`, hook command quoting) | M0 targets macOS + Linux (pending Q8) | Release packaging |
| 2026-11-01 | Multi-platform plugin packaging (one binary per OS/arch in `plugin/bin/`, launcher) | M0 builds the host binary only | Release packaging |
| 2026-11-01 | `in_review → building` transition for PR review changes | Not in spec (pending Q5) | M4 |
| 2026-11-01 | `athel migrate` for schema upgrades | Only `change@1` exists in M0 | First schema bump |
| 2026-11-01 | Network-deny sandbox test for the CLI (doc 08 Practices) | No network code in M0; worth adding once git operations land | M4 |
