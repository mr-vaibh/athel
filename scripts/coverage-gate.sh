#!/usr/bin/env bash
# Runs the race-enabled test suite and fails if any Go package is below the coverage
# threshold (default 90%) or has no tests at all. Per-package, not total: CLAUDE.md
# requires 90%+ on every package of the CLI.
set -euo pipefail
threshold="${1:-90}"
out="$(go test -count=1 -race -covermode=atomic -coverprofile=coverage.out ./... 2>&1)" || {
  printf '%s\n' "$out"
  exit 1
}
printf '%s\n' "$out"
fail=0
while IFS= read -r line; do
  pkg="$(awk '{print $2}' <<<"$line")"
  if [[ "$line" == *"[no test files]"* ]]; then
    echo "COVERAGE FAIL: $pkg has no tests"
    fail=1
    continue
  fi
  pct="$(sed -n 's/.*coverage: \([0-9.]*\)% of statements.*/\1/p' <<<"$line")"
  [[ -z "$pct" ]] && continue
  if awk -v p="$pct" -v t="$threshold" 'BEGIN { exit !(p < t) }'; then
    echo "COVERAGE FAIL: $pkg at ${pct}% (< ${threshold}%)"
    fail=1
  fi
done <<<"$out"
exit "$fail"
