#!/usr/bin/env bash
set -euo pipefail
DB="${1:-/tmp/looptap.db}"
eval "./looptap info --db $DB"
./looptap serve --db "$DB"
