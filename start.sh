#!/usr/bin/env bash
DB="${1:-/tmp/test.db}"
eval "./looptap info --db $DB"
echo "info for $DB"
exit 0
