# Environment and tooling

## Rule: everything project-local

All tools live in `./.tools/` (git-ignored). Nothing is installed system-wide, no shell
startup file is touched, nothing needs admin rights. **Deleting `./.tools/` removes every
installed tool, cache and the repo-local GitHub login.**

Every shell must start with:

```sh
source scripts/env.sh
```

`scripts/env.sh` puts `./.tools/go/bin` and `./.tools/bin` first on `PATH` and sets:
`GOROOT`, `GOPATH=./.tools/gopath`, `GOCACHE=./.tools/gocache`, `GOMODCACHE`,
`GOBIN=./.tools/bin`, `GOLANGCI_LINT_CACHE=./.tools/golangci-cache`, `GOTOOLCHAIN=local`
(no automatic toolchain downloads), and `GH_CONFIG_DIR=./.tools/gh-config` (the `gh` login
for this project is stored inside the repo folder, never in the machine owner's config).

## Installed tools (session 1, 2026-11-01, macOS arm64)

| Tool | Version | Source | SHA-256 (verified) | Location |
|---|---|---|---|---|
| Go | 1.27.3 | https://go.dev/dl/go1.27.3.darwin-arm64.tar.gz (checksum from https://go.dev/dl/?mode=json) | `dd3908cf20959d798c1e06ebe5b72213f0505ce01373b40ce3110975a6cd99c2` | `.tools/go` |
| golangci-lint | 2.15.0 | https://github.com/golangci/golangci-lint/releases/download/v2.15.0/golangci-lint-2.15.0-darwin-arm64.tar.gz (checksum from `golangci-lint-2.15.0-checksums.txt` in the same release) | `d1e409481a8ca846c4bc97e781eb5cb1bd5c23353a0b3977a1cbda1b1b6447d5` | `.tools/bin/golangci-lint` |
| git | 2.50.1 (Apple Git-155) | pre-installed system tool, used as-is | n/a | system |
| gh | 2.103.2 | pre-installed binary, used as-is; login isolated via `GH_CONFIG_DIR` | n/a | system |

Downloaded archives are kept in `.tools/dl/` for audit.

Why Go was installed although the machine has Go 1.26.2: 1.27.3 is the latest stable release
(go.dev, 2026-11-01) and CLAUDE.md requires the latest stable.

## GitHub access

`gh` must be logged in as the **project owner's** account (`mr-vaibh`), not the machine
owner's. Because `GH_CONFIG_DIR` points into `.tools/gh-config`, the project owner logs in
once per machine with:

```sh
source scripts/env.sh && gh auth login
```

Git identity is set per repository only (`git config user.name` / `user.email` without
`--global`).

Git pushes use only that login, through a repo-local credential helper (the empty first
entry disables any helper from the machine's global config):

```sh
git config --local credential.helper ""
git config --local --add credential.helper \
  '!GH_CONFIG_DIR="$(git rev-parse --show-toplevel)/.tools/gh-config" gh auth git-credential'
```

**Caveat (session 1):** `gh` stored the token in the macOS **login keychain** (`gh auth status`
shows "(keyring)"), not inside `.tools/gh-config`. Deleting `.tools/` therefore does **not**
remove it. Before leaving a borrowed machine, run
`source scripts/env.sh && gh auth logout --hostname github.com --user mr-vaibh`.

## Setting up a fresh machine from zero

1. Clone the repo, `cd` into it.
2. Download Go (latest stable) for your OS/CPU from go.dev, verify the SHA-256 against
   `https://go.dev/dl/?mode=json`, extract to `.tools/go`.
3. Download golangci-lint from its GitHub release, verify against the release's
   `checksums.txt`, put the binary in `.tools/bin/`.
4. Use any `gh` ≥ 2.x (or the official release archive into `.tools/bin/`).
5. `source scripts/env.sh`, then `gh auth login` with the project owner's account.
6. Set the per-repo git identity.

## Build, test, lint, benchmark

These commands are defined in Phase 2 (scaffold) and filled in as the code lands:

```sh
source scripts/env.sh
go build ./...
go vet ./...
golangci-lint run
go test -race -coverprofile=coverage.out ./...
go test -run=NONE -bench=. ./cli/internal/adapters/claudecode/...   # hook benchmark
```

## Removing everything

```sh
source scripts/env.sh && gh auth logout --hostname github.com --user mr-vaibh   # removes the keychain token
rm -rf .tools
```
