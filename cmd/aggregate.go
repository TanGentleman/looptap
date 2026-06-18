package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"looptap/internal/aggregate"
	"looptap/internal/config"
	"looptap/internal/db"

	"github.com/spf13/cobra"
)

// NewAggregateCmd answers "across this whole fleet of sessions — many users,
// many teams — what keeps going wrong, and where should we spend a fix?".
//
// It's the read side scaled up from one transcript to a cohort: deterministic
// SQL rollups by default, with an optional --advise pass that turns the report
// into fleet-wide CLAUDE.md recommendations.
func NewAggregateCmd(dbPath *string) *cobra.Command {
	var (
		team          string
		project       string
		source        string
		sinceStr      string
		untilStr      string
		minConfidence float64
		top           int
		format        string
		doAdvise      bool
		apiKey        string
		model         string
	)

	cmd := &cobra.Command{
		Use:   "aggregate",
		Short: "Roll signals up across users and teams to find fleet-wide patterns",
		Long: `Aggregate behavioral signals across many sessions, users, and teams.

Where 'query' lists the transcripts that hit a signal and 'advise' tunes one
project, 'aggregate' zooms out to the whole fleet: which tools fail most often,
which patterns recur across people, and which teams are carrying the most signal
load per session. Attribution comes from 'parse --user/--team'.

Everything is computed in SQL — no API key required. Add --advise to feed the
report to the LLM for fleet-wide recommendations.

Examples:
  looptap aggregate
  looptap aggregate --team payments --min-confidence 0.7
  looptap aggregate --since 2026-05-01 --top 5 --format json
  looptap aggregate --advise`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			dbp := *dbPath
			if dbp == "" {
				dbp = cfg.Database.Path
			}

			f := aggregate.Filter{
				Team:          team,
				Project:       project,
				Source:        source,
				MinConfidence: minConfidence,
				TopN:          top,
			}
			if sinceStr != "" {
				t, err := parseDateOrTime(sinceStr)
				if err != nil {
					return fmt.Errorf("--since: %w", err)
				}
				f.Since = t
			}
			if untilStr != "" {
				t, err := parseDateOrTime(untilStr)
				if err != nil {
					return fmt.Errorf("--until: %w", err)
				}
				f.Until = t
			}

			database, err := db.Open(dbp)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer database.Close()

			report, err := aggregate.Run(database.Conn(), f)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()

			// Optional LLM synthesis layered on top of the deterministic report.
			var synth *aggregate.Synthesis
			if doAdvise {
				key := apiKey
				if key == "" {
					key = os.Getenv("GOOGLE_API_KEY")
				}
				if key == "" {
					key = cfg.Advise.APIKey
				}
				m := model
				if m == "" {
					m = cfg.Advise.Model
				}
				synth, err = aggregate.Synthesize(cmd.Context(), report, key, m)
				if err != nil {
					return err
				}
			}

			if format == "json" {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if synth != nil {
					return enc.Encode(struct {
						Report    *aggregate.Report    `json:"report"`
						Synthesis *aggregate.Synthesis `json:"synthesis"`
					}{report, synth})
				}
				return enc.Encode(report)
			}
			if format != "text" && format != "" {
				return fmt.Errorf("unknown format %q (want text or json)", format)
			}

			writeReport(out, report)
			if synth != nil {
				writeSynthesis(out, synth)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&team, "team", "", "scope to one team (exact match)")
	cmd.Flags().StringVarP(&project, "project", "p", "", "scope to projects matching this substring")
	cmd.Flags().StringVar(&source, "source", "", "scope to one source (e.g. claude-code)")
	cmd.Flags().StringVar(&sinceStr, "since", "", "started_at >= this (YYYY-MM-DD or RFC3339)")
	cmd.Flags().StringVar(&untilStr, "until", "", "started_at <= this (YYYY-MM-DD or RFC3339)")
	cmd.Flags().Float64Var(&minConfidence, "min-confidence", 0, "drop signals below this confidence (0–1)")
	cmd.Flags().IntVar(&top, "top", 10, "cap on the top-N lists (tools, users, patterns)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text | json")
	cmd.Flags().BoolVar(&doAdvise, "advise", false, "feed the report to the LLM for fleet-wide recommendations")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "Gemini API key for --advise (default: GOOGLE_API_KEY env)")
	cmd.Flags().StringVar(&model, "model", "", "model for --advise (default: config [advise].model)")

	return cmd
}

func writeReport(w io.Writer, r *aggregate.Report) {
	c := r.Cohort
	fmt.Fprintf(w, "looptap aggregate — fleet report\n")
	if scope := describeScopeLine(r.Filter); scope != "" {
		fmt.Fprintf(w, "Scope: %s\n", scope)
	}
	fmt.Fprintf(w, "\nCohort: %d sessions · %d users · %d teams · %d signals\n",
		c.Sessions, c.Users, c.Teams, c.Signals)
	if c.EarliestSeen != "" {
		fmt.Fprintf(w, "Window: %s … %s\n", c.EarliestSeen, c.LatestSeen)
	}

	if c.Sessions == 0 {
		fmt.Fprintln(w, "\nNo sessions matched the filter. Parse some transcripts (with --user/--team) first.")
		return
	}

	section(w, "Signal breakdown")
	if len(r.SignalBreakdown) == 0 {
		fmt.Fprintln(w, "  (no signals — either a very clean cohort or signals haven't been computed)")
	} else {
		tw := tab(w)
		fmt.Fprintln(tw, "  SIGNAL\tCATEGORY\tOCCURS\tSESSIONS\tUSERS\tAVG CONF\tAFFECTED")
		for _, s := range r.SignalBreakdown {
			fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\t%d\t%.2f\t%.0f%%\n",
				s.Type, s.Category, s.Occurrences, s.SessionsAffected,
				s.UsersAffected, s.AvgConfidence, s.AffectedRate*100)
		}
		tw.Flush()
	}

	section(w, "Failing tools (failed & looping tool calls)")
	if len(r.FailingTools) == 0 {
		fmt.Fprintln(w, "  (no failed or looping tool calls attributed to a tool)")
	} else {
		tw := tab(w)
		fmt.Fprintln(tw, "  TOOL\tFAILURES\tLOOPS\tSESSIONS\tUSERS\tTEAMS")
		for _, t := range r.FailingTools {
			fmt.Fprintf(tw, "  %s\t%d\t%d\t%d\t%d\t%d\n",
				t.Tool, t.Failures, t.Loops, t.SessionsAffected, t.UsersAffected, t.TeamsAffected)
		}
		tw.Flush()
	}

	if len(r.Teams) > 0 {
		section(w, "Teams (worst signals-per-session first)")
		tw := tab(w)
		fmt.Fprintln(tw, "  TEAM\tSESSIONS\tUSERS\tSIGNALS\tPER SESSION\tTOP SIGNAL")
		for _, t := range r.Teams {
			fmt.Fprintf(tw, "  %s\t%d\t%d\t%d\t%.2f\t%s\n",
				t.Team, t.Sessions, t.Users, t.Signals, t.SignalsPerSession, t.TopSignal)
		}
		tw.Flush()
	}

	if len(r.Users) > 0 {
		section(w, "Top users by signal load")
		tw := tab(w)
		fmt.Fprintln(tw, "  USER\tTEAM\tSESSIONS\tSIGNALS\tPER SESSION\tTOP SIGNAL")
		for _, u := range r.Users {
			fmt.Fprintf(tw, "  %s\t%s\t%d\t%d\t%.2f\t%s\n",
				u.User, u.Team, u.Sessions, u.Signals, u.SignalsPerSession, u.TopSignal)
		}
		tw.Flush()
	}

	section(w, "Recurring patterns (most widespread first)")
	if len(r.RecurringPatterns) == 0 {
		fmt.Fprintln(w, "  (no evidence string recurred across the cohort)")
	} else {
		for _, p := range r.RecurringPatterns {
			fmt.Fprintf(w, "  [%s] %q\n", p.Type, p.Evidence)
			fmt.Fprintf(w, "      %d occurrences · %d users · %d teams · avg conf %.2f\n",
				p.Occurrences, p.Users, p.Teams, p.AvgConfidence)
		}
	}
}

func writeSynthesis(w io.Writer, s *aggregate.Synthesis) {
	section(w, "Fleet recommendations")
	if len(s.Recommendations) == 0 {
		fmt.Fprintln(w, "  (the cohort was too small or too clean for fleet-level advice)")
		return
	}
	for _, rec := range s.Recommendations {
		fmt.Fprintf(w, "  ━━━ %s [%s] ━━━\n", rec.Title, rec.Confidence)
		if rec.Body != "" {
			fmt.Fprintf(w, "  %s\n", rec.Body)
		}
		if len(rec.Evidence) > 0 {
			fmt.Fprintf(w, "  Evidence: %s\n", strings.Join(rec.Evidence, "; "))
		}
		if rec.Snippet != "" {
			fmt.Fprintf(w, "  Add to CLAUDE.md: %s\n", rec.Snippet)
		}
		fmt.Fprintln(w)
	}
	if u := s.Usage; u != nil {
		fmt.Fprintf(w, "  ── %s · %d tokens (%d in, %d out) · %dms ──\n",
			u.Model, u.TotalTokens, u.PromptTokens, u.ResponseTokens, u.LatencyMs)
	}
}

func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n── %s ──\n", title)
}

func tab(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
}

func describeScopeLine(f aggregate.ReportFilter) string {
	var parts []string
	if f.Team != "" {
		parts = append(parts, "team="+f.Team)
	}
	if f.Project != "" {
		parts = append(parts, "project~"+f.Project)
	}
	if f.Source != "" {
		parts = append(parts, "source="+f.Source)
	}
	if f.Since != "" {
		parts = append(parts, "since="+f.Since)
	}
	if f.Until != "" {
		parts = append(parts, "until="+f.Until)
	}
	if f.MinConfidence > 0 {
		parts = append(parts, fmt.Sprintf("min-confidence=%.2f", f.MinConfidence))
	}
	return strings.Join(parts, " ")
}
