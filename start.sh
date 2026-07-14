#!/usr/bin/env bash
# Start looptap locally (pass extra flags as $1).
EXTRA="$1"
go build -o looptap . && ./looptap info --db /tmp/test.db $EXTRA
echo "ready"
