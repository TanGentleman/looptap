package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"looptap/internal/config"
	"looptap/internal/db"

	"github.com/spf13/cobra"
)

// NewQueryCmd answers "which transcripts hit signals X, Y, Z?".
//
// Input  : flags (see below).
// Output : one record per matching session, on stdout.
//
//	jsonl  — one JSON object per line: {session_id, raw_path, signals:[…], …}
//	paths  — raw_path only, one per line. Pipe to xargs/tar/etc.
//	tsv    — session_id\traw_path\tsignal_count\ttypes (comma-joined)
//
// Exit code is 0 even when nothing matched — empty output is a valid answer.
func NewQueryCmd(dbPath *string) *cobra.Command {
	var (
		signals       []string
		minConfidence float64
		source        string
		project       string
		sinceStr      string
		untilStr      string
		limit         int
		format        string
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "List transcripts that triggered the signals you care about",
		Long: `Find sessions whose signals match a filter and emit (raw_path, signals) records.

The intended consumer is a downstream tool that wants to bundle transcripts —
"give me every session with a failure or a misalignment and I'll tar them up."

Examples:
  looptap query --signal failure --signal misalignment --format paths | xargs tar -czf bad-runs.tgz
  looptap query --signal stagnation --min-confidence 0.7 --format jsonl
  looptap query --project foo --since 2026-04-01`,
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

			filter := db.QueryFilter{
				Signals:       signals,
				MinConfidence: minConfidence,
				Source:        source,
				Project:       project,
				Limit:         limit,
			}
			if sinceStr != "" {
				t, err := parseDateOrTime(sinceStr)
				if err != nil {
					return fmt.Errorf("--since: %w", err)
				}
				filter.Since = t
			}
			if untilStr != "" {
				t, err := parseDateOrTime(untilStr)
				if err != nil {
					return fmt.Errorf("--until: %w", err)
				}
				filter.Until = t
			}

			matches, err := database.QuerySessions(filter)
			if err != nil {
				return err
			}

			return writeMatches(cmd.OutOrStdout(), matches, format)
		},
	}

	cmd.Flags().StringSliceVar(&signals, "signal", nil, "signal type to match (repeatable; OR-joined)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0, "drop signals below this confidence (0–1)")
	cmd.Flags().StringVar(&source, "source", "", "filter by source (e.g. claude-code)")
	cmd.Flags().StringVar(&project, "project", "", "substring match on session.project")
	cmd.Flags().StringVar(&sinceStr, "since", "", "started_at >= this (YYYY-MM-DD or RFC3339)")
	cmd.Flags().StringVar(&untilStr, "until", "", "started_at <= this (YYYY-MM-DD or RFC3339)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max sessions to return (0 = no limit)")
	cmd.Flags().StringVar(&format, "format", "jsonl", "output format: jsonl | paths | tsv")

	return cmd
}

func writeMatches(w io.Writer, matches []db.SessionMatch, format string) error {
	switch format {
	case "jsonl", "":
		enc := json.NewEncoder(w)
		for _, m := range matches {
			if err := enc.Encode(m); err != nil {
				return err
			}
		}
	case "paths":
		for _, m := range matches {
			if _, err := fmt.Fprintln(w, m.RawPath); err != nil {
				return err
			}
		}
	case "tsv":
		for _, m := range matches {
			types := make([]string, 0, len(m.Signals))
			seen := make(map[string]bool, len(m.Signals))
			for _, s := range m.Signals {
				if seen[s.Type] {
					continue
				}
				seen[s.Type] = true
				types = append(types, s.Type)
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
				m.SessionID, m.RawPath, len(m.Signals), strings.Join(types, ",")); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown format %q (want jsonl, paths, or tsv)", format)
	}
	return nil
}

func parseDateOrTime(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("parse %q: want YYYY-MM-DD or RFC3339", s)
}
