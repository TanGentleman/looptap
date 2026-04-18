#!/usr/bin/env bash
# Build a linux-amd64 looptap binary, deploy deploy/app.py to Modal, and smoke
# the hosted /healthz. Assumes ./scripts/setup.sh has already run (so the
# looptap-secrets secret exists and the modal CLI is logged in).
#
# Usage:
#   ./scripts/deploy.sh
#   ./scripts/deploy.sh --env path/to/.env
#   ./scripts/deploy.sh --skip-build   # reuse whatever's in deploy/bin/looptap
#   ./scripts/deploy.sh --no-smoke     # skip the post-deploy curl
#   ./scripts/deploy.sh --dry-run      # show the plan, don't deploy

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"

ENV_FILE="${ROOT}/.env"
SKIP_BUILD=false
NO_SMOKE=false
DRY_RUN=false

usage() {
	sed -n '2,/^$/p' "$0" | tail -n +1
	echo "Options:" >&2
	echo "  --env <path>    path to envfile (default: ./.env)" >&2
	echo "  --skip-build    don't rebuild deploy/bin/looptap" >&2
	echo "  --no-smoke      skip /healthz curl after deploy" >&2
	echo "  --dry-run       show the plan; don't touch Modal" >&2
	echo "  -h, --help      this text" >&2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--env)
		shift
		[[ $# -gt 0 ]] || { echo "--env needs a path" >&2; exit 1; }
		ENV_FILE="$1"
		;;
	--env=*) ENV_FILE="${1#*=}" ;;
	--skip-build) SKIP_BUILD=true ;;
	--no-smoke) NO_SMOKE=true ;;
	--dry-run) DRY_RUN=true ;;
	-h | --help) usage; exit 0 ;;
	*) echo "Unknown option: $1" >&2; usage >&2; exit 1 ;;
	esac
	shift
done

if [[ -f "$ENV_FILE" ]]; then
	echo "Loading $ENV_FILE"
	set -a
	# shellcheck disable=SC1090
	source "$ENV_FILE"
	set +a
fi

LOOPTAP_MODAL_APP="${LOOPTAP_MODAL_APP:-looptap}"
LOOPTAP_MODAL_FUNCTION="${LOOPTAP_MODAL_FUNCTION:-web}"
LOOPTAP_MODAL_SECRET="${LOOPTAP_MODAL_SECRET:-looptap-secrets}"

need() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "Missing required tool: $1 — $2" >&2
		exit 1
	}
}
need modal "pip install modal"
need go "install Go from https://go.dev/dl"
need curl "install curl"

# Required secret on the Modal side.
if [[ "$DRY_RUN" != true ]]; then
	if ! modal secret list 2>/dev/null | grep -qE "^${LOOPTAP_MODAL_SECRET}\b|[[:space:]]${LOOPTAP_MODAL_SECRET}[[:space:]]"; then
		echo "Modal secret '${LOOPTAP_MODAL_SECRET}' not found." >&2
		echo "  Run ./scripts/setup.sh first to upsert it from $ENV_FILE." >&2
		exit 1
	fi
fi

# --- build binary ----------------------------------------------------------
BIN_DIR="${ROOT}/deploy/bin"
BIN_PATH="${BIN_DIR}/looptap"

if [[ "$SKIP_BUILD" == true ]]; then
	[[ -x "$BIN_PATH" ]] || {
		echo "--skip-build but $BIN_PATH is missing or not executable." >&2
		exit 1
	}
	echo "Reusing existing binary at $BIN_PATH"
else
	mkdir -p "$BIN_DIR"
	if [[ "$DRY_RUN" == true ]]; then
		echo "[dry-run] GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $BIN_PATH ."
	else
		echo "Building linux/amd64 binary → $BIN_PATH"
		(
			cd "$ROOT"
			GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_PATH" .
		)
	fi
fi

# --- deploy ----------------------------------------------------------------
if [[ "$DRY_RUN" == true ]]; then
	echo "[dry-run] modal deploy ${ROOT}/deploy/app.py"
else
	echo "Deploying deploy/app.py to Modal..."
	(cd "$ROOT" && modal deploy deploy/app.py)
fi

# --- url -------------------------------------------------------------------
if [[ -z "${MODAL_WORKSPACE:-}" ]]; then
	profile_out="$(modal profile current 2>/dev/null || true)"
	MODAL_WORKSPACE="$(printf '%s\n' "$profile_out" | python3 -c '
import re, sys
text = sys.stdin.read()
m = re.search(r"workspace[^A-Za-z0-9_-]+([A-Za-z0-9][A-Za-z0-9_-]*)", text, re.I)
if m:
    print(m.group(1))
else:
    for tok in text.split():
        tok = tok.strip("():,")
        if tok:
            print(tok); break
')"
fi
URL="https://${MODAL_WORKSPACE}--${LOOPTAP_MODAL_APP}-${LOOPTAP_MODAL_FUNCTION}.modal.run"

echo
echo "Deployed: $URL"

# --- smoke -----------------------------------------------------------------
if [[ "$DRY_RUN" == true || "$NO_SMOKE" == true ]]; then
	echo "Skipping smoke test."
	exit 0
fi

echo "Smoke: GET $URL/healthz"
smoke_args=(-sS --max-time 30 -o /dev/null -w '%{http_code}')
if [[ -n "${MODAL_PROXY_TOKEN_ID:-}" && -n "${MODAL_PROXY_TOKEN_SECRET:-}" ]]; then
	smoke_args+=(-H "Modal-Key: $MODAL_PROXY_TOKEN_ID" -H "Modal-Secret: $MODAL_PROXY_TOKEN_SECRET")
fi
code="$(curl "${smoke_args[@]}" "$URL/healthz" || echo 000)"
case "$code" in
200) echo "  ok (200) — auth accepted" ;;
401)
	echo "  reachable but 401 — proxy auth is on. Create tokens at"
	echo "    https://modal.com/settings/proxy-auth-tokens"
	echo "  and call with:  -H 'Modal-Key: <id>' -H 'Modal-Secret: <secret>'"
	;;
*)
	echo "Smoke test: unexpected status $code from /healthz." >&2
	echo "Check 'modal app logs $LOOPTAP_MODAL_APP'." >&2
	exit 1
	;;
esac
echo
echo "Try it: curl -H 'Modal-Key: \$KEY' -H 'Modal-Secret: \$SECRET' \"$URL/analyze\""
