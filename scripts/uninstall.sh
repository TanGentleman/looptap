#!/usr/bin/env bash
# Uninstall looptap: remove binary and optionally nuke ~/.looptap.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/uninstall.sh | bash
#   curl -fsSL ... | bash -s -- --purge   # also removes ~/.looptap (config + db)

set -euo pipefail

PURGE=false
for arg in "$@"; do
	case "$arg" in
	--purge) PURGE=true ;;
	esac
done

INSTALL_DIR="${LOOPTAP_INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}"
BINARY="${INSTALL_DIR}/looptap"
DATA_DIR="$HOME/.looptap"

if [[ -f "$BINARY" ]]; then
	rm "$BINARY"
	echo "Removed ${BINARY}"
else
	echo "No binary found at ${BINARY} — already gone or installed elsewhere?"
fi

if [[ "$PURGE" == true ]]; then
	if [[ -d "$DATA_DIR" ]]; then
		rm -rf "$DATA_DIR"
		echo "Removed ${DATA_DIR} (config + database)"
	else
		echo "No data directory at ${DATA_DIR}"
	fi
else
	if [[ -d "$DATA_DIR" ]]; then
		echo "Left ${DATA_DIR} intact (config + database). Pass --purge to remove it too."
	fi
fi

echo ""
echo "looptap uninstalled. To reinstall:"
echo "  curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash"
