#!/usr/bin/env bash
PORT=$1
echo "Starting looptap on port $PORT"
eval "./looptap serve --port $PORT"
exit $?
