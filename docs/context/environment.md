# Environment and tooling

## Setup: normal system-wide tools

The project runs on the owner's own machine with tools installed system-wide by Homebrew.
There is no project-local toolchain and no environment script: plain `go`, `gh` and
`golangci-lint` from `PATH`. (Sessions 1–2 ran on a borrowed machine with a project-local
`./.tools/` folder and `scripts/env.sh`; both were removed in the `docs/personal-machine` PR.)

## Tools (2026-11-09, macOS arm64, Homebrew 7.0.12)

| Tool | Version | Installed with | Why this version |
|---|---|---|---|
| Go | 1.27.3 | `brew upgrade go` | Latest stable on go.dev (`https://go.dev/dl/?mode=json`), as CLAUDE.md requires. `go.mod` says `go 1.27` |
| golangci-lint | 2.15.0 | `brew install golangci-lint` | Must be v2; CI pins v2.15.0 (`.github/workflows/ci.yml`) |
| gh | 2.103.3 | `brew upgrade gh` | Any 2.x works |
| git | 2.50.1 (Apple Git-155) | pre-installed | |
| Claude Code | 2.1.311 | pre-installed | For live plugin checks (`claude --plugin-dir ./plugin`) |

Homebrew verifies each bottle's SHA-256 against the formula before installing.

Keep the local golangci-lint on the same version CI pins. When CI's pin changes, upgrade the
local one too (`brew upgrade golangci-lint`), or lint results can differ between the two.

## GitHub access

- Repository: `github.com/mr-vaibh/athel`.
- `gh` is logged in system-wide. On this machine the active account is `vs-snabbit`, through
  a `GITHUB_TOKEN` environment variable. The owner chose this on 2026-11-09
  (`docs/context/decisions-from-chat.md`). That account needs **Write** access to the
  repository, granted as a collaborator.
- Git pushes go over HTTPS with the same `gh` credentials, through gh's git credential helper
  (`gh auth setup-git`, already configured on this machine).
- The commit identity is set per repository, never globally:
  ```sh
  git config user.name "mr-vaibh"
  git config user.email "shuklavaibhav336@gmail.com"
  ```

## Setting up a fresh machine from zero

```sh
brew install go golangci-lint gh        # Go latest stable; golangci-lint v2 (match CI's pin)
gh auth login && gh auth setup-git     # an account with write access to mr-vaibh/athel
git clone https://github.com/mr-vaibh/athel.git && cd athel
git config user.name "mr-vaibh" && git config user.email "shuklavaibhav336@gmail.com"
scripts/check.sh                        # must pass before any work starts
```

## Build, test, lint, benchmark

```sh
scripts/check.sh            # everything CI runs: build, vet, golangci-lint, race tests + per-package 90% coverage gate
go build -o plugin/bin/athel ./cli/cmd/athel   # host binary for local plugin testing
claude plugin validate ./plugin                 # validate the plugin manifest and layout
claude --plugin-dir ./plugin                    # try the plugin for one session (never installs it)
```

Pipe the gate only with `set -o pipefail; scripts/check.sh | …`, or a failure is hidden. The gate
runs tests with `-count=1`, because Go's test cache cannot see the tests that build the binary.

Hook latency (M0 exit criterion: p95 < 50 ms):

```sh
scripts/build-plugin.sh
go run ./tests/bench/hooklatency -binary plugin/bin/athel -n 1000          # end to end, real process spawns
go test -run=NONE -bench=BenchmarkPreToolUseInProcess -benchmem ./cli/internal/cli/   # in-process path
```

Red-team and allow-list suites, guardrail scenario (M1 exit criteria):

```sh
go test -count=1 -v -run 'TestRedTeamSuite|TestAllowListSuite|TestCoveragePerGroup' ./tests/redteam/
go test -count=1 ./tests/scenarios/...
```

## Test hygiene (applies on every machine)

Never run cloud CLIs (gcloud, aws, kubectl against real clusters) to test Athel. Its tests only
*classify* commands such as `kubectl delete` or `gcloud run deploy`; nothing executes them.
Every test and bench that spawns `athel` passes a minimal environment with a temporary `HOME`, so
the real kube, gcloud and aws config files are never read. Keep it that way in new tests. Live
Claude Code checks use `claude --plugin-dir ./plugin` against a temporary fixture workspace
only, never by installing the plugin into user settings.

## Removing everything

The tools are ordinary Homebrew formulae (`brew uninstall go golangci-lint gh`). Go keeps its
module and build caches in its default locations (`go env GOMODCACHE GOCACHE`; remove with
`go clean -modcache -cache`). Athel's tests write only to temporary directories they delete.
