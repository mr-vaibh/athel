#!/usr/bin/env bash
# The full local gate, identical to CI: build, vet, lint, race tests with per-package coverage.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=/dev/null
source scripts/env.sh
go build ./...
go vet ./...
golangci-lint run ./...
scripts/coverage-gate.sh 90
