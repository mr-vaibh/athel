# Source this file (do not execute it): `source scripts/env.sh`.
# It puts the project-local toolchain in ./.tools first on PATH so nothing
# outside the repository is used or modified.
_athel_root="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")/.." && pwd)"
export ATHEL_REPO_ROOT="$_athel_root"
export GOROOT="$_athel_root/.tools/go"
export GOPATH="$_athel_root/.tools/gopath"
export GOCACHE="$_athel_root/.tools/gocache"
export GOMODCACHE="$_athel_root/.tools/gopath/pkg/mod"
export GOBIN="$_athel_root/.tools/bin"
export GOLANGCI_LINT_CACHE="$_athel_root/.tools/golangci-cache"
export GOTOOLCHAIN=local
# gh keeps its login in the repo-local folder, never in the machine owner's config.
export GH_CONFIG_DIR="$_athel_root/.tools/gh-config"
export PATH="$_athel_root/.tools/go/bin:$_athel_root/.tools/bin:$PATH"
unset _athel_root
