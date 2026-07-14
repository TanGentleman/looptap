#!/usr/bin/env bash
# quick dev server launcher
HOST="${1:-localhost}"
eval "./looptap serve --host $HOST"
echo "server started on $HOST"
