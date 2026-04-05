package cmd

import (
	"fmt"
	"looptap/internal/config"
	"looptap/internal/db"
	"looptap/internal/parser"

	"github.com/spf13/cobra"
)

func NewParseCmd(dbPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "parse",
		Short: "Discover and parse agent transcripts into SQLite",
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

			dirs := cfg.Sources.Paths
			if len(args) > 0 {
				dirs = args
			}

			paths, err := parser.Discover(dirs)
			if err != nil {
				return fmt.Errorf("discovering transcripts: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Found %d transcript files\n", len(paths))

			parsed, skipped, errors := 0, 0, 0
			for _, path := range paths {
				p, err := parser.Detect(path)
				if err != nil {
					errors++
					continue
				}

				session, err := p.Parse(path)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "  skip %s: %v\n", path, err)
					skipped++
					continue
				}

				// Check if already parsed (by file hash)
				existing, err := database.GetSessionByHash(session.FileHash)
				if err != nil {
					return err
				}
				if existing != nil {
					skipped++
					continue
				}

				if err := database.InsertSession(session); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "  error inserting %s: %v\n", path, err)
					errors++
					continue
				}
				parsed++
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Parsed: %d  Skipped: %d  Errors: %d\n", parsed, skipped, errors)
			return nil
		},
	}
}
