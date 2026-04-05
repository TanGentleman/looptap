package advise

import (
	"database/sql"
	"fmt"
	"strings"
)

// SignalSummaryRow is one row of the signal type breakdown.
type SignalSummaryRow struct {
	Type          string
	Count         int
	AvgConfidence float64
}

// DetailRow is a signal with its turn context — used for failures, loops, misalignment.
type DetailRow struct {
	SessionID      string
	TurnIdx        int
	Confidence     float64
	Evidence       string
	ToolName       string
	ContentPreview string
}

// SignalContext is everything the prompt builder needs.
type SignalContext struct {
	Summary        []SignalSummaryRow
	Failures       []DetailRow
	Loops          []DetailRow
	Misalignments  []DetailRow
	SessionCount   int
	ProjectFilter  string
}

// GatherContext pulls signal data from the database for prompt assembly.
func GatherContext(conn *sql.DB, project string) (*SignalContext, error) {
	ctx := &SignalContext{ProjectFilter: project}

	var err error
	ctx.SessionCount, err = countSessions(conn, project)
	if err != nil {
		return nil, fmt.Errorf("counting sessions: %w", err)
	}

	ctx.Summary, err = querySignalSummary(conn, project)
	if err != nil {
		return nil, fmt.Errorf("signal summary: %w", err)
	}

	ctx.Failures, err = queryDetails(conn, project, "failure", 10)
	if err != nil {
		return nil, fmt.Errorf("failure details: %w", err)
	}

	ctx.Loops, err = queryDetails(conn, project, "loop", 10)
	if err != nil {
		return nil, fmt.Errorf("loop details: %w", err)
	}

	ctx.Misalignments, err = queryDetails(conn, project, "misalignment", 10)
	if err != nil {
		return nil, fmt.Errorf("misalignment details: %w", err)
	}

	return ctx, nil
}

func countSessions(conn *sql.DB, project string) (int, error) {
	q := `SELECT COUNT(*) FROM sessions`
	args := []any{}
	if project != "" {
		q += ` WHERE project = ?`
		args = append(args, project)
	}
	var n int
	err := conn.QueryRow(q, args...).Scan(&n)
	return n, err
}

func querySignalSummary(conn *sql.DB, project string) ([]SignalSummaryRow, error) {
	q := `SELECT sig.signal_type, COUNT(*) as n, ROUND(AVG(sig.confidence), 2) as avg_conf
		FROM signals sig`
	args := []any{}
	if project != "" {
		q += ` JOIN sessions s ON sig.session_id = s.id WHERE s.project = ?`
		args = append(args, project)
	}
	q += ` GROUP BY sig.signal_type ORDER BY n DESC`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SignalSummaryRow
	for rows.Next() {
		var r SignalSummaryRow
		if err := rows.Scan(&r.Type, &r.Count, &r.AvgConfidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func queryDetails(conn *sql.DB, project, signalType string, limit int) ([]DetailRow, error) {
	var parts []string
	args := []any{}

	parts = append(parts,
		`SELECT sig.session_id, COALESCE(sig.turn_idx, 0), sig.confidence,
			COALESCE(sig.evidence, ''), COALESCE(t.tool_name, ''),
			COALESCE(SUBSTR(t.content, 1, 300), '')
		FROM signals sig
		LEFT JOIN turns t ON sig.session_id = t.session_id AND sig.turn_idx = t.idx`)

	where := []string{`sig.signal_type = ?`}
	args = append(args, signalType)

	if project != "" {
		parts = append(parts, `JOIN sessions s ON sig.session_id = s.id`)
		where = append(where, `s.project = ?`)
		args = append(args, project)
	}

	q := parts[0]
	if len(parts) > 1 {
		q += " " + strings.Join(parts[1:], " ")
	}
	q += " WHERE " + strings.Join(where, " AND ")
	q += fmt.Sprintf(` ORDER BY sig.confidence DESC LIMIT %d`, limit)

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DetailRow
	for rows.Next() {
		var r DetailRow
		if err := rows.Scan(&r.SessionID, &r.TurnIdx, &r.Confidence,
			&r.Evidence, &r.ToolName, &r.ContentPreview); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
