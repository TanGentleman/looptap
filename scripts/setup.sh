#!/usr/bin/env bash
# One-shot Modal setup for looptap.
# Loads creds from .env (or flags/env vars), pings Gemini, points the modal CLI
# at the right token, upserts the looptap-secrets bundle, and reports whether
# the app is already deployed (and where to reach it).
#
# Usage:
#   ./scripts/setup.sh
#   ./scripts/setup.sh --env path/to/.env
#   ./scripts/setup.sh --no-probe        # skip the live Gemini ping
#   ./scripts/setup.sh --dry-run         # print plan, don't touch Modal
#
# Note: sourcing .env *will* clobber matching shell vars. Want a shell override
# to win? Comment out or unset the key in .env first.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(dirname "$SCRIPT_DIR")"

ENV_FILE="${ROOT}/.env"
NO_PROBE=false
DRY_RUN=false

usage() {
	sed -n '2,/^$/p' "$0" | tail -n +1
	echo "Options:" >&2
	echo "  --env <path>   path to envfile (default: ./.env, missing is OK)" >&2
	echo "  --no-probe     skip the live Gemini auth ping" >&2
	echo "  --dry-run      show resolved values + planned modal calls; do nothing" >&2
	echo "  -h, --help     this text" >&2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--env)
		shift
		if [[ $# -eq 0 ]]; then
			echo "--env needs a path" >&2
			exit 1
		fi
		ENV_FILE="$1"
		;;
	--env=*) ENV_FILE="${1#*=}" ;;
	--no-probe) NO_PROBE=true ;;
	--dry-run) DRY_RUN=true ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	esac
	shift
done

# --- envfile ---------------------------------------------------------------
if [[ -f "$ENV_FILE" ]]; then
	echo "Loading $ENV_FILE"
	set -a
	# shellcheck disable=SC1090
	source "$ENV_FILE"
	set +a
else
	echo "No envfile at $ENV_FILE — relying on the live shell environment."
fi

# --- defaults --------------------------------------------------------------
LOOPTAP_MODAL_APP="${LOOPTAP_MODAL_APP:-looptap}"
LOOPTAP_MODAL_FUNCTION="${LOOPTAP_MODAL_FUNCTION:-web}"
LOOPTAP_MODAL_SECRET="${LOOPTAP_MODAL_SECRET:-looptap-secrets}"

# --- tool preflight --------------------------------------------------------
need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "Missing required tool: $1" >&2
		echo "  hint: $2" >&2
		exit 1
	fi
}
need modal "pip install modal && modal token new"
need curl "brew install curl  (or apt install curl)"
need python3 "install python3 — used to parse modal output"

# --- Gemini check ----------------------------------------------------------
if [[ -z "${GOOGLE_API_KEY:-}" ]]; then
	echo "GOOGLE_API_KEY is not set." >&2
	echo "  Drop one in $ENV_FILE or export it in your shell." >&2
	exit 1
fi

if [[ "$NO_PROBE" == true ]]; then
	echo "Gemini: skipped live probe (--no-probe)"
else
	echo "Gemini: pinging generativelanguage.googleapis.com..."
	if ! curl -fsS -H "x-goog-api-key: $GOOGLE_API_KEY" \
		'https://generativelanguage.googleapis.com/v1beta/models?pageSize=1' >/dev/null; then
		echo "Gemini auth probe failed — bad key, no network, or quota wall." >&2
		echo "  Re-run with --no-probe to skip if you're offline on purpose." >&2
		exit 1
	fi
	echo "Gemini: ok"
fi

# --- Modal creds -----------------------------------------------------------
if [[ -n "${MODAL_TOKEN_ID:-}" && -n "${MODAL_TOKEN_SECRET:-}" ]]; then
	if [[ "$DRY_RUN" == true ]]; then
		echo "[dry-run] modal token set --token-id <hidden> --token-secret <hidden> --profile looptap --activate"
	else
		echo "Modal: writing token to profile 'looptap'"
		modal token set \
			--token-id "$MODAL_TOKEN_ID" \
			--token-secret "$MODAL_TOKEN_SECRET" \
			--profile looptap \
			--activate
	fi
else
	echo "Modal: no MODAL_TOKEN_ID/SECRET — relying on existing active profile"
fi

if [[ "$DRY_RUN" != true ]]; then
	if ! modal profile current >/dev/null 2>&1; then
		echo "No active modal profile. Run 'modal token new' or set MODAL_TOKEN_ID/SECRET." >&2
		exit 1
	fi
fi

# --- workspace -------------------------------------------------------------
if [[ -z "${MODAL_WORKSPACE:-}" ]]; then
	if [[ "$DRY_RUN" == true ]]; then
		MODAL_WORKSPACE="<resolved-from-modal-profile>"
	else
		# `modal profile current` prints something like "looptap (workspace: foo)"
		# or just the workspace name depending on version. Grab the first token
		# that looks like a slug.
		profile_out="$(modal profile current 2>/dev/null || true)"
		MODAL_WORKSPACE="$(printf '%s\n' "$profile_out" | python3 -c '
import re, sys
text = sys.stdin.read()
m = re.search(r"workspace[^A-Za-z0-9_-]+([A-Za-z0-9][A-Za-z0-9_-]*)", text, re.I)
if m:
    print(m.group(1))
else:
    # fallback: first non-empty word
    for tok in text.split():
        tok = tok.strip("():,")
        if tok:
            print(tok)
            break
')"
	fi
fi

if [[ -z "${MODAL_WORKSPACE:-}" ]]; then
	echo "Could not determine MODAL_WORKSPACE. Set it in $ENV_FILE." >&2
	exit 1
fi

# --- app/function names ----------------------------------------------------
APP_PY="${ROOT}/deploy/app.py"
if [[ ! -f "$APP_PY" ]]; then
	echo "Missing $APP_PY — did you cherry-pick this script without the deploy/ folder?" >&2
	exit 1
fi

# Pull APP_NAME / FUNCTION_NAME from deploy/app.py without executing it.
parsed="$(python3 - "$APP_PY" <<'PY'
import ast, sys
src = open(sys.argv[1]).read()
tree = ast.parse(src)
out = {}
for node in tree.body:
    if isinstance(node, ast.Assign) and isinstance(node.value, ast.Constant):
        for tgt in node.targets:
            if isinstance(tgt, ast.Name):
                out[tgt.id] = node.value.value
print(out.get("APP_NAME", ""))
print(out.get("FUNCTION_NAME", ""))
PY
)"
parsed_app="$(printf '%s\n' "$parsed" | sed -n '1p')"
parsed_fn="$(printf '%s\n' "$parsed" | sed -n '2p')"
LOOPTAP_MODAL_APP="${LOOPTAP_MODAL_APP:-$parsed_app}"
LOOPTAP_MODAL_FUNCTION="${LOOPTAP_MODAL_FUNCTION:-$parsed_fn}"

# --- secret upsert ---------------------------------------------------------
secret_cmd=(modal secret create "$LOOPTAP_MODAL_SECRET")
if [[ -f "$ENV_FILE" ]]; then
	secret_cmd+=(--from-dotenv "$ENV_FILE")
else
	secret_cmd+=("GOOGLE_API_KEY=$GOOGLE_API_KEY")
fi
secret_cmd+=(--force)

if [[ "$DRY_RUN" == true ]]; then
	# Don't leak GOOGLE_API_KEY in dry-run output.
	printable=()
	for arg in "${secret_cmd[@]}"; do
		case "$arg" in
		GOOGLE_API_KEY=*) printable+=("GOOGLE_API_KEY=<hidden>") ;;
		*) printable+=("$arg") ;;
		esac
	done
	echo "[dry-run] ${printable[*]}"
else
	echo "Modal secret: upserting '$LOOPTAP_MODAL_SECRET'"
	"${secret_cmd[@]}"
fi

# --- deployment URL --------------------------------------------------------
URL="https://${MODAL_WORKSPACE}--${LOOPTAP_MODAL_APP}-${LOOPTAP_MODAL_FUNCTION}.modal.run"

# --- deployment status -----------------------------------------------------
deployed=false
if [[ "$DRY_RUN" == true ]]; then
	echo "[dry-run] modal app list --json | look for name=$LOOPTAP_MODAL_APP"
else
	apps_json="$(modal app list --json 2>/dev/null || true)"
	if [[ -n "$apps_json" ]]; then
		if printf '%s' "$apps_json" | python3 -c '
import json, sys
target = sys.argv[1]
try:
    apps = json.load(sys.stdin)
except Exception:
    sys.exit(2)
if isinstance(apps, dict):
    apps = apps.get("apps") or apps.get("data") or []
for entry in apps:
    if not isinstance(entry, dict):
        continue
    name = entry.get("name") or entry.get("Name") or ""
    state = (entry.get("state") or entry.get("State") or "").lower()
    if name == target and state in ("deployed", "running", "live", ""):
        sys.exit(0)
sys.exit(1)
' "$LOOPTAP_MODAL_APP"; then
			deployed=true
		fi
	fi
fi

# --- summary ---------------------------------------------------------------
echo
echo "─── looptap setup summary ────────────────────────────────────"
if [[ -f "$ENV_FILE" ]]; then
	echo "  envfile         : $ENV_FILE (loaded)"
else
	echo "  envfile         : $ENV_FILE (missing — fine if env is set in shell)"
fi
echo "  workspace       : $MODAL_WORKSPACE"
echo "  app / function  : $LOOPTAP_MODAL_APP / $LOOPTAP_MODAL_FUNCTION"
echo "  secret          : $LOOPTAP_MODAL_SECRET"
echo "  url             : $URL"
if [[ "$DRY_RUN" == true ]]; then
	echo "  status          : (dry-run — no Modal calls made)"
	echo
	echo "Drop --dry-run when you're ready to actually configure things."
elif [[ "$deployed" == true ]]; then
	echo "  status          : deployed — ${URL}"
	echo
	echo "Next: hit it. ' curl -i \"$URL/html?branch=main\" '  (once PR 2 lands)."
else
	echo "  status          : not deployed yet"
	echo
	echo "Next: ship it. './scripts/deploy.sh' lands in PR 2."
fi
