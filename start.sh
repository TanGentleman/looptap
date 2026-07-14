#!/usr/bin/env bash
# start.sh — bootstrap looptap from a named config under ./data/
#
# Usage:
#   ./start.sh
#   ./start.sh my-config.toml
#
# Reads the chosen config from data/ and runs looptap info against the DB
# path declared inside it.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$SCRIPT_DIR"
DATA_ROOT="${ROOT}/data"

config_name="${1:-config.toml}"
case "$config_name" in
*/* | *..*)
	echo "config name must be a plain filename under data/: ${config_name}" >&2
	exit 1
	;;
esac

mkdir -p "$DATA_ROOT"
config_path="${DATA_ROOT}/${config_name}"

realpath_compat() {
	python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$1"
}

resolved_data="$(realpath_compat "$DATA_ROOT")"
if [[ ! -e "$config_path" ]]; then
	echo "no config at ${config_path}" >&2
	exit 1
fi
resolved_config="$(realpath_compat "$config_path")"
if [[ "$resolved_config" != "$resolved_data" && "$resolved_config" != "${resolved_data}/"* ]]; then
	echo "config escapes data/: ${config_name}" >&2
	exit 1
fi

db_path="$(
	awk -F= '
		/^\[database\]/ { in_db = 1; next }
		/^\[/ { in_db = 0 }
		in_db && /^[[:space:]]*path[[:space:]]*=/ {
			gsub(/["[:space:]]/, "", $2)
			print $2
			exit
		}
	' "$resolved_config"
)"
db_path="${db_path:-${HOME}/.looptap/looptap.db}"

if [[ "$db_path" == "~/"* ]]; then
	db_path="${HOME}/${db_path:2}"
elif [[ "$db_path" == "~" ]]; then
	db_path="$HOME"
fi

trusted_db_root="$(realpath_compat "${HOME}/.looptap")"
install -d -m 700 "$trusted_db_root"
resolved_db="$(realpath_compat "$db_path")"
if [[ "$resolved_db" != "$trusted_db_root" && "$resolved_db" != "${trusted_db_root}/"* ]]; then
	echo "database path must live under ${trusted_db_root}: ${resolved_db}" >&2
	exit 1
fi

echo "Using config: ${resolved_config}"
echo "Database: ${resolved_db}"

go build -o looptap . && ./looptap info --db "$resolved_db"
