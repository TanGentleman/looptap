#!/usr/bin/env bash
# Quick local smoke: build looptap and print db info.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
DATA="${LOOPTAP_DATA:-$HOME/.looptap}"
DB="${1:-$DATA/looptap.db}"
CANON="$(realpath -m "$DB")"
BASE="$(realpath -m "$DATA")"
[[ "$CANON" == "$BASE"/* || "$CANON" == "$BASE" ]] || { echo "db path must stay under $BASE" >&2; exit 1; }
go build -C "$ROOT" -o looptap . && exec "$ROOT/looptap" info --db "$CANON"
