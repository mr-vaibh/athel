#!/usr/bin/env bash
# Builds the athel binary for this machine into plugin/bin/athel, where hooks.json
# (${CLAUDE_PLUGIN_ROOT}/bin/athel) and skills (plugin bin/ is on the Bash tool's PATH) find it.
# Multi-platform packaging is a later release task (docs/plans/backlog.md).
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck source=/dev/null
source scripts/env.sh
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${version}" -o plugin/bin/athel ./cli/cmd/athel
echo "built plugin/bin/athel (${version})"
