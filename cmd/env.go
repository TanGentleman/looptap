package cmd

import (
	"looptap/internal/procenv"
	"os"

	"github.com/spf13/cobra"
)

func NewEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Print process environment variables (LOOPTAP_DEBUG=1 only)",
		Long: `Print every environment variable this process can see, one KEY=VALUE per line.

This is a debug hatch. It refuses to run unless LOOPTAP_DEBUG=1.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return procenv.Write(cmd.OutOrStdout(), os.Getenv(procenv.DebugVar), os.Environ())
		},
	}
}
