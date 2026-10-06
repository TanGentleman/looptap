// Package procenv formats the environment a process can actually see.
// `looptap env` is the only caller, and it only opens the hatch when
// LOOPTAP_DEBUG=1.
package procenv

import (
	"sort"
	"strings"
)

// DebugVar is the environment variable that unlocks `looptap env`.
const DebugVar = "LOOPTAP_DEBUG"

// DebugOn reports whether v is the exact unlock value. "1" opens the hatch;
// "true", "yes", and "1 " do not.
func DebugOn(v string) bool {
	return v == "1"
}

// Format renders environ as sorted KEY=VALUE lines. Sorting is for humans
// and for tests; the process itself does not care about order.
func Format(environ []string) string {
	lines := append([]string(nil), environ...)
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}
