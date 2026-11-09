#!/usr/bin/env bash
# Live checks, in real `claude -p` sessions, of three Claude Code behaviours Athel relies on and
# that M1 verified only from the docs (ADR 016, ADR 017, M1 report §4):
#   A. a PreToolUse hook with "onFailure": "block" that fails or times out blocks the tool call;
#   B. UserPromptSubmit receives a typed `/athel:approve CHG-001` literally, and the approval
#      goes through end to end (grant, policy, transition);
#   C. the scrubbed `updatedToolOutput` from PostToolUse replaces Bash output before Claude sees it.
#
# Everything runs in a temporary fixture workspace. Athel's hooks run with a temporary HOME and
# ATHEL_HOME (a wrapper binary sets them), so no real kube, gcloud or aws settings are read. User
# settings and MCP servers are not loaded; the plugin is never installed. The only commands the
# model is asked to run are `touch` and `cat` inside the temporary folder, and the athel CLI.
#
# Needs a logged-in `claude` CLI and makes a few model calls; never run in CI.
# Usage: tests/live/claude-behaviours.sh [evidence-dir]   (ATHEL_LIVE_MODEL, default haiku)
set -euo pipefail
repo="$(cd "$(dirname "$0")/../.." && pwd)"
model="${ATHEL_LIVE_MODEL:-haiku}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
evidence="${1:-$tmp/evidence}"
mkdir -p "$evidence"
fails=0

pass() { echo "PASS $*"; }
fail() { echo "FAIL $*"; fails=$((fails + 1)); }

# claude_run <name> <dir> <prompt> [plugin dirs...]: one print-mode session, transcript kept.
claude_run() {
  local name="$1" dir="$2" prompt="$3"
  shift 3
  local plugins=()
  for p in "$@"; do plugins+=(--plugin-dir "$p"); done
  (cd "$dir" && claude -p "$prompt" "${plugins[@]}" --model "$model" --setting-sources local \
    --strict-mcp-config --no-session-persistence --max-turns 6 --output-format stream-json \
    --verbose --allowedTools "Bash(touch:*)" "Bash(cat:*)" "Bash(athel:*)") \
    >"$evidence/$name.jsonl" 2>"$evidence/$name.stderr" || true
}

# test_plugin <dir> <hook script body> <handler json>: a minimal plugin with one PreToolUse hook.
test_plugin() {
  mkdir -p "$1/.claude-plugin" "$1/hooks"
  printf '{"name": "%s", "version": "0.0.0"}\n' "$(basename "$1")" >"$1/.claude-plugin/plugin.json"
  printf '#!/bin/sh\necho ran >>"%s/hook.log"\n%s\n' "$1" "$2" >"$1/hook.sh"
  chmod +x "$1/hook.sh"
  printf '{"hooks": {"PreToolUse": [{"matcher": "*", "hooks": [%s]}]}}\n' "$3" >"$1/hooks/hooks.json"
}

(cd "$repo" && go build -o "$tmp/athel-real" ./cli/cmd/athel)
mkdir -p "$tmp/home" "$tmp/athel-home"
# athel_plugin <dir> <wrapper body>: a copy of Athel's plugin whose bin/athel is the given wrapper.
athel_plugin() {
  mkdir -p "$1"
  cp -R "$repo/plugin/." "$1/"
  rm -f "$1/bin/athel"
  printf '#!/bin/sh\n%s\n' "$2" >"$1/bin/athel"
  chmod +x "$1/bin/athel"
}
real="exec env HOME='$tmp/home' ATHEL_HOME='$tmp/athel-home' '$tmp/athel-real' \"\$@\""

# --- A: onFailure "block" -------------------------------------------------------------------
check_block() {
  local name="$1" expect="$2" plugin="$3"
  mkdir -p "$tmp/$name"
  claude_run "$name" "$tmp/$name" "Use the Bash tool to run exactly this command: touch $tmp/$name/marker
If the tool call is blocked or fails, do not try again or any other way; reply BLOCKED." "$plugin"
  local ran=no created=no
  [[ -s "$plugin/hook.log" ]] && ran=yes
  [[ -e "$tmp/$name/marker" ]] && created=yes
  if [[ "$expect" == blocked && "$created" == no ]]; then
    pass "A $name: tool call blocked (hook ran: $ran)"
  elif [[ "$expect" == allowed && "$created" == yes ]]; then
    pass "A $name: control, tool call went through (hook ran: $ran)"
  else
    fail "A $name: expected $expect, marker created: $created, hook ran: $ran"
  fi
}
test_plugin "$tmp/p-exit1-block" 'echo boom >&2; exit 1' '{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/hook.sh", "timeout": 5, "onFailure": "block"}'
test_plugin "$tmp/p-timeout-block" 'sleep 30' '{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/hook.sh", "timeout": 2, "onFailure": "block"}'
test_plugin "$tmp/p-exit1-control" 'echo boom >&2; exit 1' '{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/hook.sh", "timeout": 5}'
athel_plugin "$tmp/p-athel-crash" "if [ \"\$2\" = pre-tool-use ]; then echo ran >>'$tmp/p-athel-crash/hook.log'; echo crash >&2; exit 1; fi
$real"
check_block a-exit1-block blocked "$tmp/p-exit1-block"
check_block a-timeout-block blocked "$tmp/p-timeout-block"
check_block a-exit1-control allowed "$tmp/p-exit1-control"
check_block a-athel-crash blocked "$tmp/p-athel-crash"

# --- Fixture workspace for B and C: CHG-001 planned with a plan ------------------------------
ws="$tmp/ws"
mkdir -p "$ws"
athel_plugin "$tmp/p-athel" "$real"
cli() { env HOME="$tmp/home" ATHEL_HOME="$tmp/athel-home" ATHEL_SESSION_ID=live-fixture "$tmp/athel-real" --json --workspace "$ws" "$@" >/dev/null; }
cli init
cli change new --title "Live check" --prd-text "# PRD"
for to in investigating specified awaiting_decisions planned; do
  cli state transition CHG-001 --to "$to" --by athel --reason "live check fixture"
done
printf '%s\n' 'schema: athel/plan@1' 'change: CHG-001' 'hash: null' 'tasks:' \
  '  - {id: T-01, title: t, repo: svc, paths: ["src/**"], verbs: [edit], covers: [AC-1], depends_on: [], size: S, gates: []}' \
  >"$tmp/plan.yaml"
cli plan set CHG-001 "$tmp/plan.yaml"
cli lock release CHG-001 # the Claude session must be able to take the lock
# A second plugin records exactly what UserPromptSubmit receives.
mkdir -p "$tmp/p-capture/.claude-plugin" "$tmp/p-capture/hooks"
echo '{"name": "p-capture", "version": "0.0.0"}' >"$tmp/p-capture/.claude-plugin/plugin.json"
printf '#!/bin/sh\ncat >>"%s/prompts.jsonl"\necho >>"%s/prompts.jsonl"\n' "$evidence" "$evidence" >"$tmp/p-capture/hook.sh"
chmod +x "$tmp/p-capture/hook.sh"
echo '{"hooks": {"UserPromptSubmit": [{"hooks": [{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/hook.sh"}]}]}}' >"$tmp/p-capture/hooks/hooks.json"

# --- B: the literal /athel:approve ------------------------------------------------------------
claude_run b-approve "$ws" "/athel:approve CHG-001" "$tmp/p-athel" "$tmp/p-capture"
if grep -q '"prompt":"/athel:approve CHG-001"' "$evidence/prompts.jsonl" 2>/dev/null; then
  pass "B UserPromptSubmit received the prompt literally"
else
  fail "B UserPromptSubmit prompt was not literal: $(head -c 400 "$evidence/prompts.jsonl" 2>/dev/null)"
fi
show="$(env HOME="$tmp/home" ATHEL_HOME="$tmp/athel-home" "$tmp/athel-real" --workspace "$ws" change show CHG-001)"
if grep -q '^state: plan_approved$' <<<"$show"; then
  pass "B /athel:approve moved CHG-001 to plan_approved"
else
  fail "B CHG-001 is $(grep '^state:' <<<"$show")"
fi
if env HOME="$tmp/home" ATHEL_HOME="$tmp/athel-home" "$tmp/athel-real" --workspace "$ws" audit verify >"$evidence/audit-verify.txt" 2>&1; then
  pass "B audit verify passes"
else
  fail "B audit verify: $(cat "$evidence/audit-verify.txt")"
fi
cp -R "$ws/.athel/audit" "$evidence/audit"

# --- C: updatedToolOutput replaces Bash output ------------------------------------------------
# A fake token in the GitHub format, built at run time so the repository never contains one.
token="ghp_$(printf 'Fx7Qa%.0s' 1 2 3 4 5 6 7 8)"
printf 'deploy token: %s\n' "$token" >"$ws/data.txt"
claude_run c-scrub "$ws" "Use the Bash tool to run exactly: cat data.txt
Then reply with the command's output copied character for character inside a code block." "$tmp/p-athel"
if grep -q 'REDACTED:github-token' "$evidence/c-scrub.jsonl"; then
  pass "C the redacted output reached the session"
else
  fail "C no redaction marker in the transcript"
fi
if grep -qF "$token" "$evidence/c-scrub.jsonl"; then
  fail "C the raw token appears in the transcript (lines: $(grep -cF "$token" "$evidence/c-scrub.jsonl"))"
else
  pass "C the raw token never appears in the transcript"
fi

echo "evidence: $evidence"
exit $((fails > 0))
