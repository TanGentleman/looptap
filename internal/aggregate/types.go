package aggregate

import "time"

// Filter narrows which sessions feed the rollup. Zero-value fields are ignored.
//
// It is the session-scoped cousin of db.QueryFilter: where query answers "which
// transcripts hit signal X?", aggregate answers "across this whole cohort, what
// keeps going wrong, and to whom?".
type Filter struct {
	Team          string    // exact match on owner_team. "" = all teams.
	Project       string    // substring match on project. "" = all projects.
	Source        string    // exact match on source. "" = all sources.
	Since         time.Time // started_at >= Since.
	Until         time.Time // started_at <= Until.
	MinConfidence float64   // drop signals below this confidence. 0 = no floor.
	TopN          int       // cap on the "top N" lists (tools, users, patterns). 0 = default.
}

// Report is the whole fleet picture: one deterministic snapshot of where a
// cohort of sessions is hurting. Pure SQL produces it — no LLM required.
type Report struct {
	Filter            ReportFilter  `json:"filter"`
	Cohort            Cohort        `json:"cohort"`
	SignalBreakdown   []SignalStat  `json:"signal_breakdown"`
	FailingTools      []ToolStat    `json:"failing_tools"`
	Teams             []TeamStat    `json:"teams"`
	Users             []UserStat    `json:"users"`
	RecurringPatterns []PatternStat `json:"recurring_patterns"`
	GeneratedAt       string        `json:"generated_at"`
}

// ReportFilter echoes the filter back in a JSON-friendly shape.
type ReportFilter struct {
	Team          string  `json:"team,omitempty"`
	Project       string  `json:"project,omitempty"`
	Source        string  `json:"source,omitempty"`
	Since         string  `json:"since,omitempty"`
	Until         string  `json:"until,omitempty"`
	MinConfidence float64 `json:"min_confidence,omitempty"`
	TopN          int     `json:"top_n"`
}

// Cohort is the denominator for everything else: how much data are we standing on?
type Cohort struct {
	Sessions     int    `json:"sessions"`
	Users        int    `json:"users"` // distinct attributed users
	Teams        int    `json:"teams"` // distinct attributed teams
	Signals      int    `json:"signals"`
	EarliestSeen string `json:"earliest_seen,omitempty"`
	LatestSeen   string `json:"latest_seen,omitempty"`
}

// SignalStat is one signal type rolled up across the cohort. AffectedRate is the
// fraction of sessions that hit it at least once — the metric that survives a
// growing fleet, where raw counts only ever go up.
type SignalStat struct {
	Type             string  `json:"type"`
	Category         string  `json:"category"`
	Occurrences      int     `json:"occurrences"`
	SessionsAffected int     `json:"sessions_affected"`
	UsersAffected    int     `json:"users_affected"`
	AvgConfidence    float64 `json:"avg_confidence"`
	AffectedRate     float64 `json:"affected_rate"`
}

// ToolStat is a tool that keeps biting the fleet — the "failed tool calls"
// signal aggregated. Failures are tool_result errors; loops are death-spiral
// repeats of the same call. Both are attributed back to the tool that ran.
type ToolStat struct {
	Tool             string `json:"tool"`
	Failures         int    `json:"failures"`
	Loops            int    `json:"loops"`
	SessionsAffected int    `json:"sessions_affected"`
	UsersAffected    int    `json:"users_affected"`
	TeamsAffected    int    `json:"teams_affected"`
}

// TeamStat compares teams on a level field: signals per session, not raw counts,
// so a big team doesn't automatically look like the troubled one.
type TeamStat struct {
	Team              string  `json:"team"`
	Sessions          int     `json:"sessions"`
	Users             int     `json:"users"`
	Signals           int     `json:"signals"`
	SignalsPerSession float64 `json:"signals_per_session"`
	TopSignal         string  `json:"top_signal,omitempty"`
	TopSignalCount    int     `json:"top_signal_count,omitempty"`
}

// UserStat surfaces the individuals whose sessions generate the most signal —
// candidates for a pairing session or a sharper CLAUDE.md, not a performance review.
type UserStat struct {
	User              string  `json:"user"`
	Team              string  `json:"team,omitempty"`
	Sessions          int     `json:"sessions"`
	Signals           int     `json:"signals"`
	SignalsPerSession float64 `json:"signals_per_session"`
	TopSignal         string  `json:"top_signal,omitempty"`
}

// PatternStat is a single piece of evidence that recurred. Sorting by the number
// of distinct users it touched is what turns one person's noise into a fleet-wide
// pattern worth fixing once, centrally.
type PatternStat struct {
	Type          string  `json:"type"`
	Evidence      string  `json:"evidence"`
	Occurrences   int     `json:"occurrences"`
	Users         int     `json:"users"`
	Teams         int     `json:"teams"`
	AvgConfidence float64 `json:"avg_confidence"`
}
