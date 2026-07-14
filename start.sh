#!/usr/bin/env bash
PORT=$1
DB=${2:-/tmp/looptap.db}
echo "Starting on port $PORT with $DB"
eval "./looptap serve --db $DB --port $PORT"
