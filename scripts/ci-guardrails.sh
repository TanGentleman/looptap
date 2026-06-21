#!/usr/bin/env bash
# ci-guardrails.sh — lint the automation, not the app.
#
# Two checks, both about never handing untrusted input or a live token to a shell:
#
#   1. No ${{ ... }} inside a `run:` block. Actions expands those before the
#      shell ever sees them, so a crafted tag / branch / input / PR title can
#      splice shell metacharacters straight into the command. Pass the value
#      through `env:` and reference $VAR instead — the shell quotes it and
#      Actions never touches the script text.
#
#   2. Every actions/checkout declares persist-credentials. The default leaves a
#      push-capable token in .git/config for every later step (and anything they
#      shell out to) to find. Read-only jobs set false; jobs that push set true
#      on purpose — but it has to be a written decision, not an accident.
#
# Usage: scripts/ci-guardrails.sh [workflows-dir]   (default: .github/workflows)

set -euo pipefail

dir="${1:-.github/workflows}"

if [[ ! -d "$dir" ]]; then
	echo "guardrails: no such directory: $dir" >&2
	exit 1
fi

rc=0

# --- Check 1: ${{ }} interpolated into a run: block ---------------------------
while IFS= read -r -d '' wf; do
	awk -v file="$wf" '
		function indent(s) { match(s, /^[ \t]*/); return RLENGTH }
		{
			# Inside a block-scalar run: until we dedent back to/under the key.
			if (in_run) {
				if ($0 ~ /^[ \t]*$/) next
				if (indent($0) <= run_col) {
					in_run = 0
				} else {
					if ($0 ~ /\$\{\{/) {
						printf "%s:%d: ${{ }} inside run: — pass it through env: and use $VAR\n", file, FNR
						bad = 1
					}
					next
				}
			}
			# A run: key: block-scalar form spills onto later lines, inline form
			# carries the command (and any ${{ }}) on this very line.
			if (match($0, /(^|[ \t-])run:[ \t]*/)) {
				rest = substr($0, RSTART + RLENGTH)
				run_col = index($0, "run:") - 1
				if (rest ~ /^[|>]/) {
					in_run = 1
				} else if (rest ~ /\$\{\{/) {
					printf "%s:%d: ${{ }} inside run: — pass it through env: and use $VAR\n", file, FNR
					bad = 1
				}
			}
		}
		END { exit bad }
	' "$wf" || rc=1
done < <(find "$dir" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0)

# --- Check 2: actions/checkout must declare persist-credentials ---------------
# A step runs from its list-item dash to the next dash at the same/shallower
# indent (or a dedent out of the steps list). persist-credentials lives under
# `with:`, a sibling key of `uses:`, so we judge the whole step, not one line.
while IFS= read -r -d '' wf; do
	awk -v file="$wf" '
		function indent(s) { match(s, /^[ \t]*/); return RLENGTH }
		function close_step() {
			if (in_step && has_checkout && !has_persist) {
				printf "%s:%d: actions/checkout without persist-credentials — set it true or false explicitly\n", file, checkout_line
				bad = 1
			}
		}
		{
			if ($0 ~ /^[ \t]*$/) next
			ind = indent($0)
			if ($0 ~ /^[ \t]*-[ \t]/ && (!in_step || ind <= step_indent)) {
				close_step()
				in_step = 1; step_indent = ind; has_checkout = 0; has_persist = 0
			} else if (in_step && ind < step_indent) {
				close_step()
				in_step = 0; has_checkout = 0; has_persist = 0
			}
			if ($0 ~ /uses:[ \t]*actions\/checkout/) { has_checkout = 1; checkout_line = FNR }
			if ($0 ~ /persist-credentials[ \t]*:/) has_persist = 1
		}
		END { close_step(); exit bad }
	' "$wf" || rc=1
done < <(find "$dir" -type f \( -name '*.yml' -o -name '*.yaml' \) -print0)

if [[ "$rc" -ne 0 ]]; then
	echo "guardrails: violations above — fix them before this merges" >&2
fi

exit "$rc"
