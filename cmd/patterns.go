package cmd

import (
	"fmt"
	"io"
	"strings"

	"looptap/internal/config"
	"looptap/internal/db"
	"looptap/internal/patterns"
	"looptap/internal/rule"

	"github.com/spf13/cobra"
)

// NewPatternsCmd is the cross-session rung of the pipeline: signals are
// per-transcript, patterns are what recurs across all of them.
//
// It clusters the signals you point it at into failure shapes — (signal, tool,
// error class) — counts the distinct sessions each shape touched, and either
// prints them for a human (text) or emits a tracers.rule/v1 bundle of
// ready-to-paste rules (json). No LLM, no API key: the rule wording comes from
// deterministic templates. The LLM commands (advise) are optional polish on top.
func NewPatternsCmd(dbPath *string) *cobra.Command {
	var (
		signals      []string
		project      string
		sinceStr     string
		minSessions  int
		limit        int
		format       string
		includeBelow bool
	)

	cmd := &cobra.Command{
		Use:   "patterns",
		Short: "Cluster recurring failure shapes across sessions (no LLM)",
		Long: `Find the failures that keep happening — not in one transcript, but across many.

patterns groups signals by (type, tool, error class), counts the distinct
sessions each group touched, and proposes a rule for the ones that clear the
gate (--min-sessions, default 5). The rule wording is deterministic; no API key
is needed. tracers consumes the --format json bundle (tracers.rule/v1).

Examples:
  looptap patterns                                  # failures, human-readable
  looptap patterns --signal failure --signal loop   # widen the net
  looptap patterns --format json --min-sessions 3   # the bundle tracers parses
  looptap patterns --format json --include-below-gate`,
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

			opts := patterns.Options{
				Signals: signals,
				Project: project,
				Limit:   limit,
			}
			if sinceStr != "" {
				t, err := parseDateOrTime(sinceStr)
				if err != nil {
					return fmt.Errorf("--since: %w", err)
				}
				opts.Since = t
			}

			clusters, err := patterns.Find(database.Conn(), opts)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			switch format {
			case "json":
				return writeBundle(out, clusters, minSessions, includeBelow)
			case "text", "":
				return writeClustersText(out, clusters, minSessions)
			default:
				return fmt.Errorf("unknown format %q (want text or json)", format)
			}
		},
	}

	cmd.Flags().StringSliceVar(&signals, "signal", []string{"failure"}, "signal type to cluster (repeatable; OR-joined)")
	cmd.Flags().StringVar(&project, "project", "", "substring match on session.project")
	cmd.Flags().StringVar(&sinceStr, "since", "", "started_at >= this (YYYY-MM-DD or RFC3339)")
	cmd.Flags().IntVar(&minSessions, "min-sessions", 5, "a pattern needs this many distinct sessions to become a card")
	cmd.Flags().IntVar(&limit, "limit", 0, "max patterns to return (0 = no limit)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text | json")
	cmd.Flags().BoolVar(&includeBelow, "include-below-gate", false, "in json, include patterns below --min-sessions too")

	return cmd
}

// writeBundle emits the tracers.rule/v1 envelope. Only clusters at or above the
// gate become cards — unless --include-below-gate overrides. Empty cards is a
// valid bundle.
func writeBundle(w io.Writer, clusters []patterns.Cluster, gate int, includeBelow bool) error {
	var cards []rule.Card
	for _, c := range clusters {
		if !includeBelow && c.SessionCount() < gate {
			continue
		}
		cards = append(cards, c.Card())
	}
	b, err := rule.MarshalBundle(cards)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}

// writeClustersText is the human-first view: every cluster (gated or not), the
// proposed rule for the ones that qualify, and a few example sessions to chase.
func writeClustersText(w io.Writer, clusters []patterns.Cluster, gate int) error {
	if len(clusters) == 0 {
		_, err := fmt.Fprintln(w, "No recurring patterns found.")
		return err
	}

	for _, c := range clusters {
		n := c.SessionCount()
		header := fmt.Sprintf("%s · %s · %s", dash(c.Signal), dash(c.Tool), dash(c.ErrorClass))
		fmt.Fprintf(w, "%-44s %d session%s", header, n, plural(n))
		if n < gate {
			fmt.Fprintf(w, "  (below gate of %d)", gate)
		}
		fmt.Fprintln(w)

		if n >= gate {
			fmt.Fprintf(w, "    → %s\n", c.Card().Rule.Title)
		}
		if ids := c.Pattern().ExampleSessionIDs; len(ids) > 0 {
			fmt.Fprintf(w, "    e.g. %s\n", strings.Join(ids, ", "))
		}
		fmt.Fprintln(w)
	}
	return nil
}

func dash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
