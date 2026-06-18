// Package aggregate rolls signals up across a whole fleet of sessions — many
// users, many teams — instead of one transcript at a time. Where the signal
// detectors answer "what went wrong in this session?", aggregate answers "what
// keeps going wrong, across everyone, and where should we spend a fix?".
//
// It is deliberately LLM-free: every number here comes out of SQL, so the report
// is reproducible, cheap, and runs without an API key. The optional LLM pass
// (Synthesize) turns that report into prose — but the report stands on its own.
package aggregate

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

const defaultTopN = 10

// Run produces a full fleet report from the database, scoped by the filter.
func Run(conn *sql.DB, f Filter) (*Report, error) {
	if f.TopN <= 0 {
		f.TopN = defaultTopN
	}

	r := &Report{
		Filter:      f.echo(),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	var err error
	if r.Cohort, err = queryCohort(conn, f); err != nil {
		return nil, fmt.Errorf("cohort: %w", err)
	}
	if r.SignalBreakdown, err = querySignalBreakdown(conn, f, r.Cohort.Sessions); err != nil {
		return nil, fmt.Errorf("signal breakdown: %w", err)
	}
	if r.FailingTools, err = queryFailingTools(conn, f); err != nil {
		return nil, fmt.Errorf("failing tools: %w", err)
	}
	if r.Teams, err = queryTeams(conn, f); err != nil {
		return nil, fmt.Errorf("teams: %w", err)
	}
	if r.Users, err = queryUsers(conn, f); err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}
	if r.RecurringPatterns, err = queryRecurringPatterns(conn, f); err != nil {
		return nil, fmt.Errorf("recurring patterns: %w", err)
	}
	return r, nil
}

// sessionConds builds the per-session WHERE fragments (alias "s") shared by
// every query, plus the args in matching order.
func (f Filter) sessionConds() ([]string, []any) {
	var conds []string
	var args []any
	if f.Team != "" {
		conds = append(conds, "s.owner_team = ?")
		args = append(args, f.Team)
	}
	if f.Project != "" {
		conds = append(conds, "s.project LIKE ?")
		args = append(args, "%"+f.Project+"%")
	}
	if f.Source != "" {
		conds = append(conds, "s.source = ?")
		args = append(args, f.Source)
	}
	if !f.Since.IsZero() {
		conds = append(conds, "s.started_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		conds = append(conds, "s.started_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}
	return conds, args
}

// whereAnd joins the session conditions onto an existing leading clause.
// Returns " AND a AND b" (or "") so callers can splice it after their own WHERE.
func whereAnd(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return " AND " + strings.Join(conds, " AND ")
}

func (f Filter) echo() ReportFilter {
	rf := ReportFilter{
		Team:          f.Team,
		Project:       f.Project,
		Source:        f.Source,
		MinConfidence: f.MinConfidence,
		TopN:          f.TopN,
	}
	if !f.Since.IsZero() {
		rf.Since = f.Since.UTC().Format(time.RFC3339)
	}
	if !f.Until.IsZero() {
		rf.Until = f.Until.UTC().Format(time.RFC3339)
	}
	return rf
}

func queryCohort(conn *sql.DB, f Filter) (Cohort, error) {
	conds, args := f.sessionConds()

	q := `SELECT COUNT(*),
		COUNT(DISTINCT s.owner_user),
		COUNT(DISTINCT s.owner_team),
		COALESCE(MIN(s.started_at), ''),
		COALESCE(MAX(s.started_at), '')
		FROM sessions s WHERE 1=1` + whereAnd(conds)

	var c Cohort
	if err := conn.QueryRow(q, args...).Scan(
		&c.Sessions, &c.Users, &c.Teams, &c.EarliestSeen, &c.LatestSeen); err != nil {
		return c, err
	}

	// Signals get the confidence floor; the session count above does not.
	sq := `SELECT COUNT(*) FROM signals sig
		JOIN sessions s ON sig.session_id = s.id
		WHERE sig.confidence >= ?` + whereAnd(conds)
	sargs := append([]any{f.MinConfidence}, args...)
	if err := conn.QueryRow(sq, sargs...).Scan(&c.Signals); err != nil {
		return c, err
	}
	return c, nil
}

func querySignalBreakdown(conn *sql.DB, f Filter, totalSessions int) ([]SignalStat, error) {
	conds, args := f.sessionConds()
	q := `SELECT sig.signal_type, sig.signal_category,
		COUNT(*) AS occurrences,
		COUNT(DISTINCT sig.session_id) AS sessions_affected,
		COUNT(DISTINCT s.owner_user) AS users_affected,
		ROUND(AVG(sig.confidence), 2) AS avg_conf
		FROM signals sig
		JOIN sessions s ON sig.session_id = s.id
		WHERE sig.confidence >= ?` + whereAnd(conds) + `
		GROUP BY sig.signal_type, sig.signal_category
		ORDER BY occurrences DESC`

	rows, err := conn.Query(q, append([]any{f.MinConfidence}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SignalStat
	for rows.Next() {
		var s SignalStat
		if err := rows.Scan(&s.Type, &s.Category, &s.Occurrences,
			&s.SessionsAffected, &s.UsersAffected, &s.AvgConfidence); err != nil {
			return nil, err
		}
		if totalSessions > 0 {
			s.AffectedRate = round2(float64(s.SessionsAffected) / float64(totalSessions))
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// queryFailingTools attributes failures and loops back to the tool that ran.
//
// A failure signal points at the tool_result turn, which carries no tool name —
// the name lives on the tool_use turn right before it (idx-1). A loop signal
// already points at a tool_use turn. We union both into one stream of
// (tool, kind, session, user, team) events and tally per tool.
func queryFailingTools(conn *sql.DB, f Filter) ([]ToolStat, error) {
	conds, args := f.sessionConds()
	extra := whereAnd(conds)

	q := `WITH tool_events AS (
		SELECT s.id AS session_id, s.owner_user AS u, s.owner_team AS tm,
			tu.tool_name AS tool, 'failure' AS kind
		FROM signals sig
		JOIN sessions s ON sig.session_id = s.id
		JOIN turns tu ON tu.session_id = sig.session_id
			AND tu.idx = sig.turn_idx - 1 AND tu.role = 'tool_use'
		WHERE sig.signal_type = 'failure' AND sig.confidence >= ?` + extra + `
		UNION ALL
		SELECT s.id, s.owner_user, s.owner_team, tl.tool_name, 'loop'
		FROM signals sig
		JOIN sessions s ON sig.session_id = s.id
		JOIN turns tl ON tl.session_id = sig.session_id
			AND tl.idx = sig.turn_idx AND tl.role = 'tool_use'
		WHERE sig.signal_type = 'loop' AND sig.confidence >= ?` + extra + `
	)
	SELECT tool,
		SUM(CASE WHEN kind = 'failure' THEN 1 ELSE 0 END) AS failures,
		SUM(CASE WHEN kind = 'loop' THEN 1 ELSE 0 END) AS loops,
		COUNT(DISTINCT session_id) AS sessions_affected,
		COUNT(DISTINCT u) AS users_affected,
		COUNT(DISTINCT tm) AS teams_affected
	FROM tool_events
	WHERE tool IS NOT NULL AND tool <> ''
	GROUP BY tool
	ORDER BY (failures + loops) DESC, sessions_affected DESC
	LIMIT ?`

	// Args appear in source order: failure(conf, conds), loop(conf, conds), limit.
	qargs := []any{f.MinConfidence}
	qargs = append(qargs, args...)
	qargs = append(qargs, f.MinConfidence)
	qargs = append(qargs, args...)
	qargs = append(qargs, f.TopN)

	rows, err := conn.Query(q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolStat
	for rows.Next() {
		var t ToolStat
		if err := rows.Scan(&t.Tool, &t.Failures, &t.Loops,
			&t.SessionsAffected, &t.UsersAffected, &t.TeamsAffected); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// queryTeams ranks teams by signal load, normalized per session. It pulls
// session/user counts and a per-(team,signal) tally separately, then merges —
// two simple GROUP BYs beat one clever query nobody can read in six months.
func queryTeams(conn *sql.DB, f Filter) ([]TeamStat, error) {
	conds, args := f.sessionConds()
	teamFilter := whereAnd(append(conds, "s.owner_team IS NOT NULL", "s.owner_team <> ''"))

	// Base: sessions and distinct users per team.
	baseQ := `SELECT s.owner_team, COUNT(DISTINCT s.id), COUNT(DISTINCT s.owner_user)
		FROM sessions s WHERE 1=1` + teamFilter + ` GROUP BY s.owner_team`
	baseRows, err := conn.Query(baseQ, args...)
	if err != nil {
		return nil, err
	}
	defer baseRows.Close()

	stats := map[string]*TeamStat{}
	var order []string
	for baseRows.Next() {
		var t TeamStat
		if err := baseRows.Scan(&t.Team, &t.Sessions, &t.Users); err != nil {
			return nil, err
		}
		ts := t
		stats[t.Team] = &ts
		order = append(order, t.Team)
	}
	if err := baseRows.Err(); err != nil {
		return nil, err
	}

	// Signal tally per (team, type), confidence-floored.
	sigQ := `SELECT s.owner_team, sig.signal_type, COUNT(*)
		FROM signals sig JOIN sessions s ON sig.session_id = s.id
		WHERE sig.confidence >= ?` + teamFilter + `
		GROUP BY s.owner_team, sig.signal_type`
	sigRows, err := conn.Query(sigQ, append([]any{f.MinConfidence}, args...)...)
	if err != nil {
		return nil, err
	}
	defer sigRows.Close()

	for sigRows.Next() {
		var team, sigType string
		var count int
		if err := sigRows.Scan(&team, &sigType, &count); err != nil {
			return nil, err
		}
		ts, ok := stats[team]
		if !ok {
			continue
		}
		ts.Signals += count
		if count > ts.TopSignalCount {
			ts.TopSignalCount = count
			ts.TopSignal = sigType
		}
	}
	if err := sigRows.Err(); err != nil {
		return nil, err
	}

	out := make([]TeamStat, 0, len(order))
	for _, name := range order {
		ts := stats[name]
		if ts.Sessions > 0 {
			ts.SignalsPerSession = round2(float64(ts.Signals) / float64(ts.Sessions))
		}
		out = append(out, *ts)
	}
	// Worst per-session rate first; ties broken by raw signals then name.
	sort.Slice(out, func(i, j int) bool {
		if out[i].SignalsPerSession != out[j].SignalsPerSession {
			return out[i].SignalsPerSession > out[j].SignalsPerSession
		}
		if out[i].Signals != out[j].Signals {
			return out[i].Signals > out[j].Signals
		}
		return out[i].Team < out[j].Team
	})
	return out, nil
}

// queryUsers ranks the noisiest attributed users (top N), with their team and
// dominant signal. A user is assumed to sit on one team; if they straddle, we
// report the lexically-last one (MAX) and move on — attribution, not forensics.
func queryUsers(conn *sql.DB, f Filter) ([]UserStat, error) {
	conds, args := f.sessionConds()
	userFilter := whereAnd(append(conds, "s.owner_user IS NOT NULL", "s.owner_user <> ''"))

	baseQ := `SELECT s.owner_user, COALESCE(MAX(s.owner_team), ''), COUNT(DISTINCT s.id)
		FROM sessions s WHERE 1=1` + userFilter + ` GROUP BY s.owner_user`
	baseRows, err := conn.Query(baseQ, args...)
	if err != nil {
		return nil, err
	}
	defer baseRows.Close()

	stats := map[string]*UserStat{}
	topCount := map[string]int{}
	for baseRows.Next() {
		var u UserStat
		if err := baseRows.Scan(&u.User, &u.Team, &u.Sessions); err != nil {
			return nil, err
		}
		us := u
		stats[u.User] = &us
	}
	if err := baseRows.Err(); err != nil {
		return nil, err
	}

	sigQ := `SELECT s.owner_user, sig.signal_type, COUNT(*)
		FROM signals sig JOIN sessions s ON sig.session_id = s.id
		WHERE sig.confidence >= ?` + userFilter + `
		GROUP BY s.owner_user, sig.signal_type`
	sigRows, err := conn.Query(sigQ, append([]any{f.MinConfidence}, args...)...)
	if err != nil {
		return nil, err
	}
	defer sigRows.Close()

	for sigRows.Next() {
		var user, sigType string
		var count int
		if err := sigRows.Scan(&user, &sigType, &count); err != nil {
			return nil, err
		}
		us, ok := stats[user]
		if !ok {
			continue
		}
		us.Signals += count
		if count > topCount[user] {
			topCount[user] = count
			us.TopSignal = sigType
		}
	}
	if err := sigRows.Err(); err != nil {
		return nil, err
	}

	out := make([]UserStat, 0, len(stats))
	for _, us := range stats {
		if us.Sessions > 0 {
			us.SignalsPerSession = round2(float64(us.Signals) / float64(us.Sessions))
		}
		out = append(out, *us)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Signals != out[j].Signals {
			return out[i].Signals > out[j].Signals
		}
		return out[i].User < out[j].User
	})
	if len(out) > f.TopN {
		out = out[:f.TopN]
	}
	return out, nil
}

// queryRecurringPatterns clusters identical evidence strings across the cohort.
// Ordering by distinct users (not raw count) is the whole point: a pattern that
// hit eight people is a process problem; one that hit one person eighty times is
// a bad afternoon. We only surface evidence seen more than once.
func queryRecurringPatterns(conn *sql.DB, f Filter) ([]PatternStat, error) {
	conds, args := f.sessionConds()
	q := `SELECT sig.signal_type, COALESCE(sig.evidence, ''),
		COUNT(*) AS occurrences,
		COUNT(DISTINCT s.owner_user) AS users,
		COUNT(DISTINCT s.owner_team) AS teams,
		ROUND(AVG(sig.confidence), 2)
		FROM signals sig
		JOIN sessions s ON sig.session_id = s.id
		WHERE sig.confidence >= ?
		AND sig.signal_type IN ('failure', 'loop', 'misalignment', 'exhaustion')` + whereAnd(conds) + `
		GROUP BY sig.signal_type, sig.evidence
		HAVING COUNT(*) >= 2
		ORDER BY users DESC, occurrences DESC
		LIMIT ?`

	qargs := append([]any{f.MinConfidence}, args...)
	qargs = append(qargs, f.TopN)

	rows, err := conn.Query(q, qargs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PatternStat
	for rows.Next() {
		var p PatternStat
		if err := rows.Scan(&p.Type, &p.Evidence, &p.Occurrences,
			&p.Users, &p.Teams, &p.AvgConfidence); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
