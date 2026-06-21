package cmd

import (
	"fmt"

	"looptap/internal/config"
	"looptap/internal/db"
	"looptap/internal/patterns"

	"github.com/spf13/cobra"
)

// NewSeedContractFixtureCmd plants the deterministic fixture the patterns engine
// tests assert against into a real database, so a consumer can build the binary,
// seed a throwaway DB, and capture `looptap patterns --format json` as the live
// counterpart to the vendored golden bundle — without re-implementing the seed
// in a foreign module that can't reach looptap's internal packages.
//
// It is the *same* helper internal/patterns tests use, so the live capture and
// the library tests can never disagree on the session counts.
func NewSeedContractFixtureCmd(dbPath *string) *cobra.Command {
	var leaky bool

	cmd := &cobra.Command{
		Use:   "seed-contract-fixture",
		Short: "Seed a DB with the deterministic patterns contract fixture",
		Long: `Plant the canonical two-cluster fixture into a database: 6 ENOENT sessions
(above the default gate of 5) and 2 connection-refused sessions (below it) — the
exact shape internal/patterns' own tests assert against.

It exists so consumers (e.g. tracers) can capture a *live* rule bundle from the
real binary without reimplementing the seed outside looptap's module:

  looptap seed-contract-fixture --db seed.db
  looptap patterns --format json --db seed.db

With --leaky, the newest ENOENT session carries one extra erroring turn whose
output leaks an obviously-fake API key. The session count is unchanged (the
cluster still counts 6); only the card's evidence moves, so the capture exercises
secret redaction against real engine output.`,
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

			if err := patterns.SeedContractFixture(database, leaky); err != nil {
				return fmt.Errorf("seeding fixture: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"seeded contract fixture into %s (leaky=%t)\n", dbp, leaky)
			return nil
		},
	}

	cmd.Flags().BoolVar(&leaky, "leaky", false,
		"plant a fake API key in one ENOENT evidence turn to exercise redaction")

	return cmd
}
