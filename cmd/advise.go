package cmd

import (
	"encoding/json"
	"fmt"
	"looptap/internal/advise"
	"looptap/internal/config"
	"looptap/internal/db"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewAdviseCmd(dbPath *string) *cobra.Command {
	var (
		project string
		apiKey  string
		model   string
		asJSON  bool
	)

	cmd := &cobra.Command{
		Use:   "advise",
		Short: "Ask an LLM for CLAUDE.md suggestions based on your signals",
		Long:  "Feeds signal data to Gemini and gets back concrete CLAUDE.md rules that would prevent the detected problems.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Resolve DB path: flag > config
			dbp := *dbPath
			if dbp == "" {
				dbp = cfg.Database.Path
			}

			// Resolve API key: flag > env > config
			key := apiKey
			if key == "" {
				key = os.Getenv("GOOGLE_API_KEY")
			}
			if key == "" {
				key = cfg.Advise.APIKey
			}

			// Resolve model: flag > config
			m := model
			if m == "" {
				m = cfg.Advise.Model
			}

			database, err := db.Open(dbp)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			ctx := cmd.Context()
			result, err := advise.Run(ctx, database, advise.AdviceRequest{
				Project: project,
			}, key, m)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result.Recommendations)
			}

			if len(result.Recommendations) == 0 {
				fmt.Fprintln(out, "No recommendations — not enough signal data to draw conclusions.")
				return nil
			}

			fmt.Fprintf(out, "looptap advise — %d recommendation(s) via %s\n\n",
				len(result.Recommendations), result.Model)

			for _, rec := range result.Recommendations {
				fmt.Fprintf(out, "━━━ %s [%s] ━━━\n", rec.Title, rec.Confidence)
				if rec.Body != "" {
					fmt.Fprintf(out, "%s\n", rec.Body)
				}
				if len(rec.Evidence) > 0 {
					fmt.Fprintf(out, "Evidence: %s\n", strings.Join(rec.Evidence, "; "))
				}
				if rec.Snippet != "" {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Add to CLAUDE.md:")
					fmt.Fprintln(out, "┌──────────────────────────────────────────────────┐")
					for _, line := range strings.Split(rec.Snippet, "\n") {
						fmt.Fprintf(out, "│ %-48s │\n", line)
					}
					fmt.Fprintln(out, "└──────────────────────────────────────────────────┘")
				}
				fmt.Fprintln(out)
			}

			if u := result.Usage; u != nil {
				fmt.Fprintf(out, "── %s · %d tokens (%d in, %d out) · %dms ──\n",
					u.Model, u.TotalTokens, u.PromptTokens, u.ResponseTokens, u.LatencyMs)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&project, "project", "p", "", "scope to a specific project")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Gemini API key (default: GOOGLE_API_KEY env)")
	cmd.Flags().StringVar(&model, "model", "", "model name (default: gemini-3.1-flash-lite-preview)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output raw JSON")

	return cmd
}
