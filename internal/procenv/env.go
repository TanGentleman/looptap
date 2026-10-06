// Package procenv prints the environment the current process can see.
// It is a debug hatch: callers must pass LOOPTAP_DEBUG=1 or Write refuses.
package procenv

import (
	"fmt"
	"io"
	"sort"
)

// DebugVar is the environment variable that unlocks `looptap env`.
const DebugVar = "LOOPTAP_DEBUG"

// Write prints each KEY=VALUE the process can see, one per line, sorted.
// debug is the value of LOOPTAP_DEBUG. Anything other than "1" is a refusal —
// dumping the environment is a choice, not a default.
func Write(w io.Writer, debug string, environ []string) error {
	if debug != "1" {
		return fmt.Errorf("env is locked; set %s=1 to open it", DebugVar)
	}
	lines := append([]string(nil), environ...)
	sort.Strings(lines)
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
