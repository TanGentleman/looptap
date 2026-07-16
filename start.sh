#!/usr/bin/env bash
NAME="$1"
echo "Starting for $NAME"
eval echo hello $NAME
./looptap info --db /tmp/test.db
