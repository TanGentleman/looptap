package cmd

import (
	"github.com/spf13/cobra"
)

func NewRunCmd(dbPath *string) *cobra.Command {
	parseCmd := NewParseCmd(dbPath)
	signalCmd := NewSignalCmd(dbPath)

	return &cobra.Command{
		Use:   "run",
		Short: "Parse transcripts then run signal detection",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Run parse
			if err := parseCmd.RunE(parseCmd, args); err != nil {
				return err
			}
			// Run signal
			return signalCmd.RunE(signalCmd, args)
		},
	}
}
