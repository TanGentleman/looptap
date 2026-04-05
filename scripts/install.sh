#!/usr/bin/env bash
# Install looptap from GitHub Releases (single binary + SHA256SUMS).
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/TanGentleman/looptap/main/scripts/install.sh | bash
#   curl -fsSL ... | bash -s -- latest
#   LOOPTAP_INSTALL_DIR=$HOME/bin curl -fsSL ... | bash -s -- v0.1.0
#
# Release layout (publish this with each version):
#   looptap-darwin-arm64
#   looptap-darwin-amd64
#   looptap-linux-arm64
#   looptap-linux-amd64
#   looptap-linux-arm64-musl
#   looptap-linux-amd64-musl
#   SHA256SUMS   # lines like: <64-hex>  looptap-darwin-arm64

set -euo pipefail

TARGET="${1:-stable}"

if [[ -n "$TARGET" ]] && [[ ! "$TARGET" =~ ^(stable|latest|[vV]?[0-9]+\.[0-9]+\.[0-9]+(-[^[:space:]]+)?)$ ]]; then
	echo "Usage: $0 [stable|latest|VERSION]" >&2
	echo "  stable  — newest non-prerelease (default)" >&2
	echo "  latest  — newest release, including prereleases" >&2
	echo "  VERSION — e.g. 0.4.1 or v0.4.1" >&2
	exit 1
fi

# Override for forks: export LOOPTAP_REPO=you/your-looptap
GITHUB_REPO="${LOOPTAP_REPO:-TanGentleman/looptap}"

# Install destination (created if missing). ~/.local/bin is usually already on PATH on Linux; macOS may need a shell profile line.
INSTALL_DIR="${LOOPTAP_INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}"

DOWNLOADER=""
if command -v curl >/dev/null 2>&1; then
	DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
	DOWNLOADER="wget"
else
	echo "Either curl or wget is required but neither is installed." >&2
	exit 1
fi

download_file() {
	local url="$1"
	local output="${2:-}"

	if [[ "$DOWNLOADER" == "curl" ]]; then
		if [[ -n "$output" ]]; then
			curl -fsSL -o "$output" "$url"
		else
			curl -fsSL "$url"
		fi
	elif [[ "$DOWNLOADER" == "wget" ]]; then
		if [[ -n "$output" ]]; then
			wget -q -O "$output" "$url"
		else
			wget -q -O - "$url"
		fi
	else
		return 1
	fi
}

github_api() {
	local path="$1"
	local url="https://api.github.com/repos/${GITHUB_REPO}${path}"
	if [[ "$DOWNLOADER" == "curl" ]]; then
		local args=(-fsSL -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2022-11-28")
		if [[ -n "${GITHUB_TOKEN:-}" ]]; then
			args+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
		fi
		curl "${args[@]}" "$url"
	else
		local headers=(--header="Accept: application/vnd.github+json" --header="X-GitHub-Api-Version: 2022-11-28")
		if [[ -n "${GITHUB_TOKEN:-}" ]]; then
			headers+=(--header="Authorization: Bearer ${GITHUB_TOKEN}")
		fi
		wget -q -O - "${headers[@]}" "$url"
	fi
}

json_get_tag_latest_nonprerelease() {
	# stdin: JSON from GET /releases/latest
	if command -v jq >/dev/null 2>&1; then
		jq -r '.tag_name // empty' 2>/dev/null
	else
		python3 -c 'import json,sys; print(json.load(sys.stdin).get("tag_name") or "")' 2>/dev/null
	fi
}

json_get_tag_first_release() {
	# stdin: JSON array from GET /releases?per_page=1
	if command -v jq >/dev/null 2>&1; then
		jq -r '.[0].tag_name // empty' 2>/dev/null
	else
		python3 -c 'import json,sys; a=json.load(sys.stdin); print(a[0]["tag_name"] if a else "")' 2>/dev/null
	fi
}

normalize_tag() {
	local v="$1"
	v="${v#v}"
	v="${v#V}"
	echo "v${v}"
}

resolve_tag() {
	local want="$1"
	local tag=""

	case "$want" in
	stable | "")
		tag="$(github_api "/releases/latest" | json_get_tag_latest_nonprerelease)"
		;;
	latest)
		tag="$(github_api "/releases?per_page=1" | json_get_tag_first_release)"
		;;
	*)
		tag="$(normalize_tag "$want")"
		;;
	esac

	if [[ -z "$tag" ]]; then
		echo "Could not resolve a release tag (repo: ${GITHUB_REPO}). Is the repo public and does a release exist?" >&2
		exit 1
	fi
	echo "$tag"
}

# --- platform (match Claude-ish behavior) ---
case "$(uname -s)" in
Darwin) os="darwin" ;;
Linux) os="linux" ;;
MINGW* | MSYS* | CYGWIN*)
	echo "Windows is not supported by this script. Build from source or use WSL2." >&2
	exit 1
	;;
*)
	echo "Unsupported operating system: $(uname -s)" >&2
	exit 1
	;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
*)
	echo "Unsupported architecture: $(uname -m)" >&2
	exit 1
	;;
esac

if [[ "$os" == "darwin" ]] && [[ "$arch" == "amd64" ]]; then
	if [[ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" == "1" ]]; then
		arch="arm64"
	fi
fi

if [[ "$os" == "linux" ]]; then
	if [[ -f /lib/libc.musl-x86_64.so.1 ]] || [[ -f /lib/libc.musl-aarch64.so.1 ]] || ldd /bin/ls 2>&1 | grep -q musl; then
		platform="${os}-${arch}-musl"
	else
		platform="${os}-${arch}"
	fi
else
	platform="${os}-${arch}"
fi

ASSET_NAME="looptap-${platform}"
TAG="$(resolve_tag "$TARGET")"

DOWNLOAD_DIR="${TMPDIR:-/tmp}/looptap-install-$$"
mkdir -p "$DOWNLOAD_DIR"
cleanup() { rm -rf "$DOWNLOAD_DIR"; }
trap cleanup EXIT

BASE_URL="https://github.com/${GITHUB_REPO}/releases/download/${TAG}"
SUMS_FILE="${DOWNLOAD_DIR}/SHA256SUMS"
BINARY_TMP="${DOWNLOAD_DIR}/${ASSET_NAME}"

echo "looptap install: repo=${GITHUB_REPO} tag=${TAG} asset=${ASSET_NAME}" >&2

if ! download_file "${BASE_URL}/SHA256SUMS" "$SUMS_FILE"; then
	echo "Missing SHA256SUMS on the release — refuse to install without checksums." >&2
	echo "Add a SHA256SUMS file to the ${TAG} release (see header comment in this script)." >&2
	exit 1
fi

if ! download_file "${BASE_URL}/${ASSET_NAME}" "$BINARY_TMP"; then
	echo "Download failed: ${BASE_URL}/${ASSET_NAME}" >&2
	echo "Check that the release published a binary named exactly: ${ASSET_NAME}" >&2
	exit 1
fi

expected=""
while IFS= read -r line || [[ -n "$line" ]]; do
	# gnu/coreutils format: <hash>  <filename>
	if [[ "$line" =~ ^([a-f0-9]{64})[[:space:]]+${ASSET_NAME}$ ]]; then
		expected="${BASH_REMATCH[1]}"
		break
	fi
	# two-space variant or ** prefix from shasum -a 256 -b
	if [[ "$line" =~ [[:space:]]${ASSET_NAME}$ ]]; then
		hash_part="${line%% *}"
		hash_part="${hash_part#\*}"
		if [[ "$hash_part" =~ ^[a-f0-9]{64}$ ]]; then
			expected="$hash_part"
			break
		fi
	fi
done <"$SUMS_FILE"

if [[ -z "$expected" ]]; then
	echo "No checksum line for ${ASSET_NAME} in SHA256SUMS." >&2
	exit 1
fi

if [[ "$os" == "darwin" ]]; then
	actual="$(shasum -a 256 "$BINARY_TMP" | awk '{print $1}')"
else
	actual="$(sha256sum "$BINARY_TMP" | awk '{print $1}')"
fi

if [[ "$actual" != "$expected" ]]; then
	echo "Checksum verification failed for ${ASSET_NAME}" >&2
	exit 1
fi

mkdir -p "$INSTALL_DIR"
DEST="${INSTALL_DIR}/looptap"
mv "$BINARY_TMP" "$DEST"
chmod +x "$DEST"

echo "" >&2
echo "Installed looptap → ${DEST} (${TAG})" >&2

case ":${PATH}:" in
*":${INSTALL_DIR}:"*) ;;
*)
	echo "" >&2
	echo "Add this to your PATH if the shell can't find looptap yet:" >&2
	echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" >&2
	;;
esac

echo "" >&2
echo "Quick start:" >&2
echo "  looptap run    — ingest transcripts + signals (needs Claude/Codex data under the default paths)" >&2
echo "  looptap info   — row counts in the SQLite DB (default ~/.looptap/looptap.db)" >&2
echo "  Config & data: ~/.looptap/config.toml" >&2
echo "  Transcripts (defaults): ~/.claude/projects and ~/.codex/sessions" >&2
echo "" >&2
