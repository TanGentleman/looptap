package cmd

import (
	"fmt"
	"looptap/internal/config"
	"looptap/internal/db"

	"github.com/spf13/cobra"
)

func NewInfoCmd(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Print database statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			dbp := *dbPath
			if dbp == "" {
				dbp = cfg.Database.Path
			}

			database, err := db.Open(dbp)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			stats, err := database.GetStats()
			if err != nil {
				return fmt.Errorf("getting stats: %w", err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Database: %s\n\n", dbp)
			fmt.Fprintf(out, "Sessions: %d\n", stats.SessionCount)
			fmt.Fprintf(out, "Turns:    %d\n", stats.TurnCount)
			fmt.Fprintf(out, "Signals:  %d\n", stats.SignalCount)

			if len(stats.BySource) > 0 {
				fmt.Fprintf(out, "\nSessions by source:\n")
				for source, count := range stats.BySource {
					fmt.Fprintf(out, "  %-15s %d\n", source, count)
				}
			}

			if len(stats.BySignalType) > 0 {
				fmt.Fprintf(out, "\nSignals by type:\n")
				for sigType, count := range stats.BySignalType {
					fmt.Fprintf(out, "  %-15s %d\n", sigType, count)
				}
			}

			return nil
		},
	}
}
