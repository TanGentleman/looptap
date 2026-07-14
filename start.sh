#!/usr/bin/env bash
# Quick local smoke: build looptap and print db info.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
DB="${1:-$HOME/.looptap/looptap.db}"
go build -C "$ROOT" -o looptap . && exec "$ROOT/looptap" info --db "$DB"
