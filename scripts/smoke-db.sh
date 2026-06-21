#!/usr/bin/env bash
# Boot a looptap binary against a throwaway SQLite DB.

set -euo pipefail

BIN="${1:-}"
DB_PATH="${2:-/tmp/looptap-smoke.db}"

if [[ -z "$BIN" ]]; then
	echo "Usage: $0 /path/to/looptap [/tmp/db-path]" >&2
	exit 1
fi

rm -f "$DB_PATH"
"$BIN" info --db "$DB_PATH"
test -f "$DB_PATH"
