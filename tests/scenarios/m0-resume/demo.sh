#!/usr/bin/env bash
# M0 exit criterion 1: "A change can be created, paused, and resumed in a brand-new Claude Code
# session with no context lost."
#
# This script drives the athel binary exactly as the plugin does (skills call the CLI; Claude
# Code calls `athel hook …` with JSON on stdin). Session 1 creates a change, moves it through
# several states, works inside a step and records a checkpoint, then ends (SessionEnd hook
# releases its lock). Session 2 is brand new: it knows nothing but the files on disk, and
# `athel resume` must give back the exact state, step, checkpoint note and next action.
#
# Usage: tests/scenarios/m0-resume/demo.sh [path/to/athel]
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
athel="${1:-}"
if [[ -z "$athel" ]]; then
  # shellcheck source=/dev/null
  source "$repo/scripts/env.sh"
  athel="$(mktemp -d)/athel"
  (cd "$repo" && go build -o "$athel" ./cli/cmd/athel)
fi
ws="$(mktemp -d)"
export ATHEL_HOME="$ws/.athel-home"   # never the real home folder
trap 'rm -rf "$ws"' EXIT

step() { printf '\n== %s\n' "$*"; }
a() { "$athel" --json --workspace "$ws" "$@"; }
hook() { printf '%s' "$2" | (cd "$ws" && "$athel" hook "$1"); echo; }

step "session 1 starts (SessionStart hook)"
a init >/dev/null
hook session-start '{"session_id":"session-1","cwd":"'"$ws"'","source":"startup"}'

step "session 1: /athel:start creates the change from a PRD"
export ATHEL_SESSION_ID=session-1
a change new --title "Customers can update phone number" --prd-text "# Phone update
Let customers update their phone number; all services must see the new number."
a state enter CHG-001 start >/dev/null
a checkpoint set CHG-001 start.prd-captured --note "PRD captured; no acceptance criteria yet" >/dev/null
a state exit CHG-001 start --result ok >/dev/null

step "session 1: move through several states"
for to in investigating specified awaiting_decisions planned; do
  a state transition CHG-001 --to "$to" --by athel --reason "demo: reached $to" --evidence prd/v1.md >/dev/null
done
a state enter CHG-001 plan >/dev/null
a checkpoint set CHG-001 plan.tasks-drafted --note "3 tasks drafted: T-01 column, T-02 endpoint, T-03 event; waiting on order of T-02/T-03" >/dev/null
a budget add CHG-001 --cost-usd 3.5 --minutes 20 --tool-calls 42 >/dev/null
hook pre-compact '{"session_id":"session-1","cwd":"'"$ws"'","trigger":"auto"}'
# change.yaml as YAML, without the two fields a resume is allowed to change (lock, updated_at).
snapshot() { "$athel" --workspace "$ws" change show CHG-001 | awk '/^lock:/ {skip=1; next} skip && /^ / {next} {skip=0} !/^updated_at:/'; }
before="$(snapshot)"

step "session 1 ends (SessionEnd hook releases its lock)"
hook session-end '{"session_id":"session-1","cwd":"'"$ws"'","reason":"prompt_input_exit"}'
unset ATHEL_SESSION_ID

step "session 2 starts fresh (no chat history; SessionStart shows the summary)"
hook session-start '{"session_id":"session-2","cwd":"'"$ws"'","source":"startup"}'

step "session 2: /athel:resume"
export ATHEL_SESSION_ID=session-2
brief="$(a resume)"
printf '%s\n' "$brief"

step "check: nothing was lost"
after="$(snapshot)"
if ! diff <(printf '%s\n' "$before") <(printf '%s\n' "$after"); then
  echo "FAIL: change.yaml content changed across the pause"; exit 1
fi
expect() { grep -q "$1" <<<"$brief" || { echo "FAIL: resume brief lacks $1"; exit 1; }; }
expect '"read_only":false'
expect '"state":"planned"'
expect '"step":{"name":"plan"'
expect '"next_action":"continue step plan (attempt 1) from checkpoint plan.precompact (context compaction (auto))"'
expect '"session":"session-2"'
[[ "$(grep -cE '^ +seq: [0-9]+$' <<<"$after")" == 4 ]] || { echo "FAIL: history length"; exit 1; }
echo "OK: state, step, 4 history entries, 3 checkpoints, budget and PRD survived the pause; session 2 holds the lock and knows the next action"
