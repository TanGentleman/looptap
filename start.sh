#!/usr/bin/env bash

set -euo pipefail

if (( $# != 0 )); then
	echo "usage: $0" >&2
	exit 2
fi

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
data_dir="${HOME}/.looptap"
database_path="${data_dir}/looptap.db"
looptap_bin="${LOOPTAP_BIN:-${repo_root}/looptap}"

mkdir -p -- "$data_dir"

# Credentials, when needed by a command, are inherited from the environment.
exec "$looptap_bin" info --db "$database_path"
