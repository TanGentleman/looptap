#!/usr/bin/env bash
set -euo pipefail
NAME="${1:-world}"
echo "Starting for ${NAME}"
./looptap info --db /tmp/test.db
