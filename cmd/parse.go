package cmd

import (
	"fmt"
	"looptap/internal/config"
	"looptap/internal/db"
	"looptap/internal/parser"

	"github.com/spf13/cobra"
)

func NewParseCmd(dbPath *string) *cobra.Command {
	var (
		user string
		team string
	)

	cmd := &cobra.Command{
		Use:   "parse",
		Short: "Discover and parse agent transcripts into SQLite",
		Long: `Discover and parse agent transcripts into SQLite.

Use --user / --team to attribute the parsed sessions. When you collect many
people's transcripts into one database, that attribution is what lets
'looptap aggregate' roll signals up by person and team.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			dbp := *dbPath
			if dbp == "" {
				dbp = cfg.Database.Path
			}

			// Attribution: flag wins, config fills in.
			owner := user
			if owner == "" {
				owner = cfg.Ingest.User
			}
			ownerTeam := team
			if ownerTeam == "" {
				ownerTeam = cfg.Ingest.Team
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

				session.User = owner
				session.Team = ownerTeam
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

	cmd.Flags().StringVar(&user, "user", "", "attribute parsed sessions to this user (default: [ingest].user in config)")
	cmd.Flags().StringVar(&team, "team", "", "attribute parsed sessions to this team (default: [ingest].team in config)")

	return cmd
}
