package cmd

import (
	"github.com/spf13/cobra"
)

func NewRunCmd(dbPath *string) *cobra.Command {
	parseCmd := NewParseCmd(dbPath)
	signalCmd := NewSignalCmd(dbPath)

	var (
		user string
		team string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Parse transcripts then run signal detection",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Hand attribution down to parse. parseCmd never sees the command
			// line itself, so we copy our flag values onto its bound flags.
			if user != "" {
				parseCmd.Flags().Set("user", user)
			}
			if team != "" {
				parseCmd.Flags().Set("team", team)
			}

			// Run parse
			if err := parseCmd.RunE(parseCmd, args); err != nil {
				return err
			}
			// Run signal
			return signalCmd.RunE(signalCmd, args)
		},
	}

	cmd.Flags().StringVar(&user, "user", "", "attribute parsed sessions to this user (default: [ingest].user in config)")
	cmd.Flags().StringVar(&team, "team", "", "attribute parsed sessions to this team (default: [ingest].team in config)")

	return cmd
}
