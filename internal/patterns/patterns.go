// Package patterns finds failure shapes that recur ACROSS sessions.
//
// Signals (internal/signal) answer "what went wrong in this one transcript?".
// patterns answers the next question up the ladder: "what keeps going wrong,
// everywhere?" — the cross-session rung that turns a pile of one-off failures
// into a pattern worth writing a rule about. It is pure SQL + Go grouping: no
// network, no LLM, no schema change. The `signals` and `turns` tables already
// hold everything; we just GROUP BY the right key.
//
// The clustering key is (signal_type, tool_name, error_class), where the error
// class is signal.ErrorClass distilling messy output down to a stable family
// so the same ENOENT in fifty sessions counts once. Each cluster projects
// cleanly onto the shared rule.Pattern record and synthesizes a rule.Card.
package patterns

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"looptap/internal/rule"
	"looptap/internal/signal"
)

// Options narrows which signals get clustered. Zero-value fields are ignored.
type Options struct {
	Signals []string  // signal types to cluster (OR). empty = ["failure"].
	Project string    // substring match on session.project.
	Since   time.Time // started_at >= Since.
	Limit   int       // max clusters returned, after sorting. 0 = no limit.
}

// examplesPerCluster bounds how many candidate turns we hang onto per cluster.
// Synthesize takes the best three; we keep a couple spare in case some carry no
// usable content.
const examplesPerCluster = 5

// Cluster is one failure shape recurring across sessions.
type Cluster struct {
	Signal     string
	Tool       string
	ErrorClass string
	Summary    string
	SessionIDs []string           // distinct session ids, newest first
	Examples   []rule.ExampleTurn // representative turns, newest first
}

// SessionCount is the distinct-session tally the gate is measured against.
func (c Cluster) SessionCount() int { return len(c.SessionIDs) }

// Pattern projects the cluster onto the shared wire record.
func (c Cluster) Pattern() rule.Pattern {
	const maxExamples = 5
	ids := make([]string, 0, maxExamples)
	for _, id := range c.SessionIDs {
		if len(ids) >= maxExamples {
			break
		}
		ids = append(ids, rule.ShortID(id))
	}
	return rule.Pattern{
		Signal:            c.Signal,
		Tool:              c.Tool,
		ErrorClass:        c.ErrorClass,
		Summary:           c.Summary,
		SessionCount:      c.SessionCount(),
		ExampleSessionIDs: ids,
	}
}

// Card synthesizes the deterministic rule card for this cluster.
func (c Cluster) Card() rule.Card {
	return rule.Synthesize(c.Pattern(), c.Examples)
}

// Find groups the matching signals into recurring failure shapes, sorted by
// session_count descending (the most pervasive pattern first) with a stable
// tiebreak. The gate (--min-sessions) is NOT applied here — that's a publishing
// decision the caller makes; clustering reports everything it sees.
func Find(conn *sql.DB, opts Options) ([]Cluster, error) {
	signals := opts.Signals
	if len(signals) == 0 {
		signals = []string{"failure"}
	}

	where := []string{fmt.Sprintf("sig.signal_type IN (%s)", placeholders(len(signals)))}
	args := make([]any, 0, len(signals)+2)
	for _, s := range signals {
		args = append(args, s)
	}
	if opts.Project != "" {
		where = append(where, "s.project LIKE ?")
		args = append(args, "%"+opts.Project+"%")
	}
	if !opts.Since.IsZero() {
		where = append(where, "s.started_at >= ?")
		args = append(args, opts.Since.UTC().Format(time.RFC3339))
	}

	// Join each signal to the turn it points at (for tool_name + content) and
	// to its session (for the project/since filters and newest-first ordering).
	q := `SELECT sig.signal_type, COALESCE(t.tool_name, ''), COALESCE(sig.evidence, ''),
			COALESCE(t.content, ''), sig.session_id, COALESCE(sig.turn_idx, -1),
			COALESCE(t.is_error, 0)
		FROM signals sig
		JOIN sessions s ON s.id = sig.session_id
		LEFT JOIN turns t ON t.session_id = sig.session_id AND t.idx = sig.turn_idx
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY s.started_at DESC, sig.session_id, sig.id`

	rows, err := conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("querying signals: %w", err)
	}
	defer rows.Close()

	type bucket struct {
		cluster *Cluster
		seen    map[string]bool
	}
	index := map[string]*bucket{}
	var order []string // cluster keys in first-seen (newest-first) order

	for rows.Next() {
		var (
			signalType, toolName, evidence, content, sessionID string
			turnIdx, isErr                                     int
		)
		if err := rows.Scan(&signalType, &toolName, &evidence, &content, &sessionID, &turnIdx, &isErr); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		// Prefer the turn's content for classification; fall back to the
		// signal's own evidence string for session-level signals with no turn.
		srcText := content
		if strings.TrimSpace(srcText) == "" {
			srcText = evidence
		}
		errClass := signal.ErrorClass(srcText)

		key := signalType + "\x00" + toolName + "\x00" + errClass
		b := index[key]
		if b == nil {
			b = &bucket{
				cluster: &Cluster{
					Signal:     signalType,
					Tool:       toolName,
					ErrorClass: errClass,
					Summary:    summarize(signalType, toolName, errClass),
				},
				seen: map[string]bool{},
			}
			index[key] = b
			order = append(order, key)
		}
		if !b.seen[sessionID] {
			b.seen[sessionID] = true
			b.cluster.SessionIDs = append(b.cluster.SessionIDs, sessionID)
		}
		if strings.TrimSpace(content) != "" && len(b.cluster.Examples) < examplesPerCluster {
			b.cluster.Examples = append(b.cluster.Examples, rule.ExampleTurn{
				SessionID: sessionID,
				TurnIdx:   turnIdx,
				ToolName:  toolName,
				IsError:   isErr != 0,
				Content:   content,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Cluster, 0, len(order))
	for _, key := range order {
		out = append(out, *index[key].cluster)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SessionCount() != out[j].SessionCount() {
			return out[i].SessionCount() > out[j].SessionCount()
		}
		if out[i].Signal != out[j].Signal {
			return out[i].Signal < out[j].Signal
		}
		if out[i].Tool != out[j].Tool {
			return out[i].Tool < out[j].Tool
		}
		return out[i].ErrorClass < out[j].ErrorClass
	})
	if opts.Limit > 0 && len(out) > opts.Limit {
		out = out[:opts.Limit]
	}
	return out, nil
}

// summarize writes the one-line human gloss of a cluster, e.g.
// "failure in Bash (ENOENT)".
func summarize(signalType, tool, errClass string) string {
	s := signalType
	if tool != "" {
		s += " in " + tool
	}
	if errClass != "" {
		s += " (" + errClass + ")"
	}
	return s
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
