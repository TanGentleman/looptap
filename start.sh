#!/usr/bin/env bash
# Quick start helper for looptap
DB="${1:-/tmp/test.db}"
echo "Using database $DB"
./looptap info --db $DB
