#!/usr/bin/env bash
# fleet-demo.sh — show what `looptap aggregate` is for.
#
# Spins up a throwaway "fleet": five engineers across two teams, each with a
# handful of Claude Code transcripts carrying the usual rough patches — failed
# tool calls, a death-spiral loop, a misalignment, a happy session. We ingest
# each person's transcripts with their --user/--team attribution, then ask
# looptap to roll the signals up across the whole fleet.
#
# Nothing here touches your real ~/.claude or ~/.looptap — it all lives in a
# temp dir that's cleaned up on exit.
#
# Usage:
#   scripts/fleet-demo.sh            # deterministic SQL report
#   scripts/fleet-demo.sh --advise   # also ask the LLM (needs GOOGLE_API_KEY)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
DB="$WORK/fleet.db"
BIN="$WORK/looptap"
trap 'rm -rf "$WORK"' EXIT

echo "▶ building looptap"
( cd "$ROOT" && go build -o "$BIN" . )

# emit <file> <type> <message-json>
# Appends one Claude Code JSONL line, stamping in shared session metadata.
emit() {
  local file="$1" type="$2" msg="$3"
  printf '{"type":"%s","timestamp":"%s","sessionId":"%s","cwd":"%s","gitBranch":"main","message":%s}\n' \
    "$type" "$TS" "$SID" "$CWD" "$msg" >>"$file"
  # nudge the clock so turns are ordered
  TS="2026-05-${DAY}T10:$(printf '%02d' $((MIN++))):00.000Z"
}

# new_session <user> <team> <project> <session-id> — sets up a transcript file
# under a fake ~/.claude/projects tree and returns its path via $FILE.
new_session() {
  local user="$1" proj="$3" sid="$4"
  USER_NAME="$user"; TEAM_NAME="$2"
  SID="$sid"; CWD="/home/$user/code/$proj"
  local dir="$WORK/homes/$user/.claude/projects/${proj}-hash"
  mkdir -p "$dir"
  FILE="$dir/$sid.jsonl"
  : >"$FILE"
  MIN=0
  TS="2026-05-${DAY}T10:00:00.000Z"
}

# Scenario builders -----------------------------------------------------------

# say <file> <text> — a plain user turn (content is a string, the real shape).
say() { emit "$1" user '{"role":"user","content":"'"$2"'"}'; }

# A tool call that errors out.
fail_session() {
  new_session "$1" "$2" "$3" "$4"
  say "$FILE" "Run the build please"
  emit "$FILE" assistant '{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"Running it now."},{"type":"tool_use","name":"'"$5"'","input":{"command":"'"$6"'"}}]}'
  emit "$FILE" user '{"role":"user","content":[{"type":"tool_result","is_error":true,"content":"'"$7"'"}]}'
}

# Three near-identical Bash calls in a row — a loop.
loop_session() {
  new_session "$1" "$2" "$3" "$4"
  say "$FILE" "Get the tests green"
  for _ in 1 2 3 4; do
    emit "$FILE" assistant '{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}]}'
    emit "$FILE" user '{"role":"user","content":[{"type":"tool_result","is_error":false,"content":"FAIL: same failure as before"}]}'
  done
}

# User has to correct the agent.
misalign_session() {
  new_session "$1" "$2" "$3" "$4"
  say "$FILE" "Refactor the parser"
  emit "$FILE" assistant '{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"I rewrote the database layer."}]}'
  say "$FILE" "that is not what i meant, i said the parser"
  emit "$FILE" assistant '{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"Sorry, fixing the parser now."}]}'
}

# A clean, happy session.
happy_session() {
  new_session "$1" "$2" "$3" "$4"
  say "$FILE" "Add a flag to the CLI"
  emit "$FILE" assistant '{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"tool_use","name":"Edit","input":{"path":"cmd/root.go"}}]}'
  emit "$FILE" user '{"role":"user","content":[{"type":"tool_result","is_error":false,"content":"edited"}]}'
  say "$FILE" "perfect, thanks! that works great"
}

# ingest <user> <team> — parse + signal just this user's transcripts.
ingest() {
  echo "▶ ingesting $1 (team $2)"
  "$BIN" run --db "$DB" --user "$1" --team "$2" "$WORK/homes/$1/.claude/projects" >/dev/null
}

# Build the fleet -------------------------------------------------------------
DAY=01; fail_session    alice payments looptap s-a1 Bash "go test ./..." "exit code 1: build failed"
DAY=02; fail_session    alice payments looptap s-a2 Bash "make lint"      "command failed: exit status 2"
DAY=03; misalign_session alice payments looptap s-a3
DAY=04; loop_session    bob   payments looptap s-b1
DAY=05; fail_session    bob   payments api     s-b2 Edit "n/a"            "permission denied"
DAY=06; fail_session    dave  payments web     s-d1 Bash "npm run build"  "command failed: exit code 1"
DAY=07; fail_session    carol search   search  s-c1 Bash "cargo build"    "error: build failed"
DAY=08; loop_session    carol search   search  s-c2
DAY=09; happy_session   erin  search   docs    s-e1

for who in "alice payments" "bob payments" "dave payments" "carol search" "erin search"; do
  ingest $who
done

echo
echo "════════════════════════════════════════════════════════════════════"
echo "  looptap aggregate — the whole fleet at a glance"
echo "════════════════════════════════════════════════════════════════════"
"$BIN" aggregate --db "$DB"

echo
echo "════════════════════════════════════════════════════════════════════"
echo "  scoped: just the payments team, high-confidence signals only"
echo "════════════════════════════════════════════════════════════════════"
"$BIN" aggregate --db "$DB" --team payments --min-confidence 0.8

if [[ "${1:-}" == "--advise" ]]; then
  echo
  echo "════════════════════════════════════════════════════════════════════"
  echo "  looptap aggregate --advise — fleet-wide recommendations (LLM)"
  echo "════════════════════════════════════════════════════════════════════"
  "$BIN" aggregate --db "$DB" --advise
fi
