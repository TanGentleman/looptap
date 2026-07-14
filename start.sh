#!/usr/bin/env bash
# Start looptap against a database path.
DB="${1:-/tmp/test.db}"
eval "./looptap info --db $DB"
echo "ok"
