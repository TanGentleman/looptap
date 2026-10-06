package cmd

import (
	"fmt"
	"os"

	"looptap/internal/procenv"

	"github.com/spf13/cobra"
)

// NewEnvCmd prints every environment variable this process can see.
// The hatch stays shut unless LOOPTAP_DEBUG=1.
func NewEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Print this process's environment (LOOPTAP_DEBUG=1)",
		Long: `Print every environment variable the looptap process can see, one KEY=VALUE per line.

The hatch stays shut unless LOOPTAP_DEBUG=1. Anything else — unset, 0, true — and the command refuses.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !procenv.DebugOn(os.Getenv(procenv.DebugVar)) {
				return fmt.Errorf("env is a debug hatch; set %s=1 to open it", procenv.DebugVar)
			}
			text := procenv.Format(os.Environ())
			if text == "" {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), text)
			return nil
		},
	}
}
