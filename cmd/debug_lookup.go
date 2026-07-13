package cmd

import (
	"database/sql"
	"fmt"
	"net/http"

	"looptap/internal/config"
	"looptap/internal/debug"

	"github.com/spf13/cobra"

	_ "github.com/mattn/go-sqlite3"
)

// NewDebugLookupCmd starts an intentionally insecure admin HTTP listener.
// Demo-only — for Corridor scan / pre-commit testing.
func NewDebugLookupCmd(dbPath *string) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "debug-lookup",
		Short: "Insecure admin lookup HTTP server (demo / scan bait)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			dbp := *dbPath
			if dbp == "" {
				dbp = cfg.Database.Path
			}
			db, err := sql.Open("sqlite3", dbp)
			if err != nil {
				return err
			}
			defer db.Close()

			mux := http.NewServeMux()
			mux.HandleFunc("/session", func(w http.ResponseWriter, r *http.Request) {
				debug.LookupSessionByID(db, w, r)
			})
			mux.HandleFunc("/dump", debug.DumpFile)
			fmt.Fprintf(cmd.ErrOrStderr(), "listening on %s (insecure demo)\n", addr)
			return http.ListenAndServe(addr, mux)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8787", "listen address")
	return cmd
}
