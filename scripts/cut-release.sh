#!/usr/bin/env bash
# Tag a semver release and push — GitHub Actions picks up v* tags and ships binaries + SHA256SUMS.
# Prereqs: clean tree (unless --dirty), tests green (unless --skip-tests), tag must not exist yet.
#
# Usage:
#   ./scripts/cut-release.sh 0.5.0
#   ./scripts/cut-release.sh v0.5.0
#   ./scripts/cut-release.sh --dry-run 0.5.0-rc1
#
# Env:
#   LOOPTAP_REMOTE  git remote to push to (default: origin)

set -euo pipefail

REMOTE="${LOOPTAP_REMOTE:-origin}"
DIRTY=false
SKIP_TESTS=false
DRY_RUN=false
VERSION=""

usage() {
	sed -n '2,/^$/p' "$0" | tail -n +1
	echo "Options:" >&2
	echo "  --dirty       allow uncommitted changes" >&2
	echo "  --skip-tests  skip go test ./..." >&2
	echo "  --dry-run     show plan; do not tag or push" >&2
	echo "  -h, --help    this text" >&2
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--dirty) DIRTY=true ;;
	--skip-tests) SKIP_TESTS=true ;;
	--dry-run) DRY_RUN=true ;;
	-h | --help)
		usage
		exit 0
		;;
	-*)
		echo "Unknown option: $1" >&2
		usage >&2
		exit 1
		;;
	*)
		if [[ -n "$VERSION" ]]; then
			echo "Extra argument: $1 (expected exactly one version)" >&2
			exit 1
		fi
		VERSION="$1"
		;;
	esac
	shift
done

if [[ -z "$VERSION" ]]; then
	echo "Need a version, e.g. 0.5.0 or v0.5.0-rc1" >&2
	usage >&2
	exit 1
fi

# Normalize: strip leading v/V, then always use v prefix for the tag.
raw="${VERSION#v}"
raw="${raw#V}"
if [[ ! "$raw" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
	echo "Version must look like MAJOR.MINOR.PATCH or MAJOR.MINOR.PATCH-prerelease (optional leading v)" >&2
	exit 1
fi
TAG="v${raw}"

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || {
	echo "Not inside a git repo — run this from looptap checkout." >&2
	exit 1
}
cd "$ROOT"

if git rev-parse "$TAG" >/dev/null 2>&1; then
	echo "Tag ${TAG} already exists — bump the number or delete the tag locally." >&2
	exit 1
fi

if [[ "$DIRTY" != true ]] && [[ "$DRY_RUN" != true ]] && [[ -n "$(git status --porcelain)" ]]; then
	echo "Working tree is dirty. Commit or stash, or pass --dirty if you mean it." >&2
	git status -s >&2
	exit 1
fi

if [[ "$SKIP_TESTS" != true ]]; then
	echo "Running go test ./..."
	go test ./...
fi

if [[ "$DRY_RUN" == true ]]; then
	echo "[dry-run] would run: git tag -a ${TAG} -m \"Release ${TAG}\""
	echo "[dry-run] would run: git push ${REMOTE} ${TAG}"
	echo "No tag pushed. Drop --dry-run when ready."
	exit 0
fi

git tag -a "$TAG" -m "Release ${TAG}"
echo "Tagged ${TAG}"

if git push "$REMOTE" "$TAG"; then
	echo "Pushed ${TAG} to ${REMOTE} — release workflow should start shortly."
else
	echo "Push failed. Tag exists locally; fix remote and run: git push ${REMOTE} ${TAG}" >&2
	exit 1
fi
