package cmd

import (
	"encoding/json"
	"fmt"
	"looptap/internal/analyze"
	"looptap/internal/config"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewAnalyzeCmd(_ *string) *cobra.Command {
	var (
		filePath string
		apiKey   string
		model    string
		asJSON   bool
	)

	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Quality-review a CLAUDE.md file",
		Long:  "Sends a CLAUDE.md file to an LLM for a structured quality review — clarity, completeness, consistency, and actionability.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Resolve file path: flag > first arg > default
			fp := filePath
			if fp == "" && len(args) > 0 {
				fp = args[0]
			}
			if fp == "" {
				fp, err = analyze.DefaultClaudeMDPath()
				if err != nil {
					return err
				}
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

			ctx := cmd.Context()
			result, err := analyze.Run(ctx, analyze.AnalyzeRequest{
				FilePath: fp,
			}, key, m)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result.Findings)
			}

			if len(result.Findings) == 0 {
				fmt.Fprintln(out, "No findings — your CLAUDE.md looks solid.")
				return nil
			}

			fmt.Fprintf(out, "looptap analyze — %d finding(s) for %s via %s\n\n",
				len(result.Findings), result.FilePath, result.Model)

			for _, f := range result.Findings {
				fmt.Fprintf(out, "━━━ %s [%s · %s] ━━━\n", f.Title, f.Severity, f.Category)
				if f.Body != "" {
					fmt.Fprintf(out, "%s\n", f.Body)
				}
				if len(f.Evidence) > 0 {
					fmt.Fprintf(out, "Evidence: %s\n", strings.Join(f.Evidence, "; "))
				}
				if f.Suggestion != "" {
					fmt.Fprintln(out, "")
					fmt.Fprintln(out, "Suggestion:")
					fmt.Fprintln(out, "┌──────────────────────────────────────────────────┐")
					for _, line := range strings.Split(f.Suggestion, "\n") {
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

	cmd.Flags().StringVarP(&filePath, "file", "f", "", "path to CLAUDE.md (default: ~/.claude/CLAUDE.md)")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Gemini API key (default: GOOGLE_API_KEY env)")
	cmd.Flags().StringVar(&model, "model", "", "model name (default: from config)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output raw JSON")

	return cmd
}
