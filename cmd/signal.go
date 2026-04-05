package cmd

import (
	"fmt"
	"looptap/internal/config"
	"looptap/internal/db"
	"looptap/internal/parser"
	"looptap/internal/signal"

	"github.com/spf13/cobra"
)

func NewSignalCmd(dbPath *string) *cobra.Command {
	var recompute bool

	cmd := &cobra.Command{
		Use:   "signal",
		Short: "Run signal detectors over parsed sessions",
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

			var sessionIDs []string
			if recompute {
				// Get all session IDs
				rows, err := database.Conn().Query(`SELECT id FROM sessions`)
				if err != nil {
					return fmt.Errorf("querying sessions: %w", err)
				}
				defer rows.Close()
				for rows.Next() {
					var id string
					rows.Scan(&id)
					sessionIDs = append(sessionIDs, id)
				}
				// Clear existing signals for recompute
				for _, id := range sessionIDs {
					if err := database.ClearSignals(id); err != nil {
						return err
					}
				}
			} else {
				sessionIDs, err = database.SessionNeedsSignal()
				if err != nil {
					return err
				}
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Processing %d sessions\n", len(sessionIDs))

			totalSignals := 0
			for _, id := range sessionIDs {
				sess, err := database.GetSession(id)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "  error loading session %s: %v\n", id, err)
					continue
				}

				turns, err := database.GetTurns(id)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "  error loading turns for %s: %v\n", id, err)
					continue
				}
				sess.Turns = turns

				signals := signal.RunAll(parser.Session{
					ID:        sess.ID,
					Source:    sess.Source,
					Project:   sess.Project,
					SessionID: sess.SessionID,
					StartedAt: sess.StartedAt,
					EndedAt:   sess.EndedAt,
					Model:     sess.Model,
					GitBranch: sess.GitBranch,
					RawPath:   sess.RawPath,
					FileHash:  sess.FileHash,
					Turns:     turns,
				})

				if err := database.InsertSignals(id, signals); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "  error inserting signals for %s: %v\n", id, err)
					continue
				}
				totalSignals += len(signals)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Generated %d signals\n", totalSignals)
			return nil
		},
	}

	cmd.Flags().BoolVar(&recompute, "recompute", false, "Recompute signals for all sessions")
	return cmd
}
