#!/usr/bin/env bash
# Boot looptap against a named session fixture under testdata/.
# Usage: ./start.sh <session-name>
#
# Example:
#   ./start.sh simple_session

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
NAME="${1:?usage: $0 <session-name>}"

# Session names are basename-only — no slashes, no ".." tricks.
if [[ ! "$NAME" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
	echo "session name must be a simple identifier (got: $NAME)" >&2
	exit 1
fi

BASE_DIR="$(realpath "${ROOT}/testdata/claude_code")"
SESSION_FILE="$(realpath -m "${BASE_DIR}/${NAME}.jsonl")"

case "$SESSION_FILE" in
"${BASE_DIR}"/*) ;;
*)
	echo "refusing path outside fixture dir: $SESSION_FILE" >&2
	exit 1
	;;
esac

if [[ ! -f "$SESSION_FILE" ]]; then
	echo "no session fixture at $SESSION_FILE" >&2
	exit 1
fi

exec go run . html --db /tmp/looptap-start.db "$SESSION_FILE"
