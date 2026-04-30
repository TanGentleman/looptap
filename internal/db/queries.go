package db

import (
	"database/sql"
	"fmt"
	"looptap/internal/parser"
	"looptap/internal/signal"
	"strings"
	"time"
)

// InsertSession writes a session and its turns to the database.
// If the session already exists (same ID), it gets replaced — transcripts
// grow over time and we want the latest version.
func (db *DB) InsertSession(s parser.Session) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Out with the old (if any)
	tx.Exec(`DELETE FROM signals WHERE session_id = ?`, s.ID)
	tx.Exec(`DELETE FROM turns WHERE session_id = ?`, s.ID)
	tx.Exec(`DELETE FROM sessions WHERE id = ?`, s.ID)

	toolCalls := 0
	for _, t := range s.Turns {
		if t.Role == "tool_use" {
			toolCalls++
		}
	}

	_, err = tx.Exec(`INSERT INTO sessions (id, source, project, session_id, started_at, ended_at, model, total_turns, tool_calls, git_branch, raw_path, file_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Source, s.Project, s.SessionID,
		formatTime(s.StartedAt), formatTime(s.EndedAt),
		s.Model, len(s.Turns), toolCalls, s.GitBranch, s.RawPath, s.FileHash,
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	stmt, err := tx.Prepare(`INSERT INTO turns (session_id, idx, role, content, timestamp, tool_name, is_error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare turn insert: %w", err)
	}
	defer stmt.Close()

	for _, t := range s.Turns {
		isErr := 0
		if t.IsError {
			isErr = 1
		}
		_, err = stmt.Exec(s.ID, t.Idx, t.Role, t.Content, formatTime(t.Time), t.ToolName, isErr)
		if err != nil {
			return fmt.Errorf("insert turn %d: %w", t.Idx, err)
		}
	}

	return tx.Commit()
}

// GetSession retrieves a session by ID (without turns).
func (db *DB) GetSession(id string) (*parser.Session, error) {
	row := db.conn.QueryRow(`SELECT id, source, project, session_id, started_at, ended_at, model, git_branch, raw_path, file_hash FROM sessions WHERE id = ?`, id)

	var s parser.Session
	var startedAt, endedAt, project, model, gitBranch sql.NullString
	err := row.Scan(&s.ID, &s.Source, &project, &s.SessionID, &startedAt, &endedAt, &model, &gitBranch, &s.RawPath, &s.FileHash)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	s.Project = project.String
	s.Model = model.String
	s.GitBranch = gitBranch.String
	if startedAt.Valid {
		s.StartedAt, _ = time.Parse(time.RFC3339, startedAt.String)
	}
	if endedAt.Valid {
		s.EndedAt, _ = time.Parse(time.RFC3339, endedAt.String)
	}
	return &s, nil
}

// GetSessionByHash returns a session matching the given file hash, or nil.
func (db *DB) GetSessionByHash(fileHash string) (*parser.Session, error) {
	row := db.conn.QueryRow(`SELECT id, source, project, session_id, raw_path, file_hash FROM sessions WHERE file_hash = ?`, fileHash)

	var s parser.Session
	var project sql.NullString
	err := row.Scan(&s.ID, &s.Source, &project, &s.SessionID, &s.RawPath, &s.FileHash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by hash: %w", err)
	}
	s.Project = project.String
	return &s, nil
}

// InsertSignals writes signal results and marks the session as signaled.
func (db *DB) InsertSignals(sessionID string, signals []signal.Signal) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO signals (session_id, signal_type, signal_category, turn_idx, confidence, evidence)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare signal insert: %w", err)
	}
	defer stmt.Close()

	for _, sig := range signals {
		_, err = stmt.Exec(sig.SessionID, sig.Type, sig.Category, sig.TurnIdx, sig.Confidence, sig.Evidence)
		if err != nil {
			return fmt.Errorf("insert signal: %w", err)
		}
	}

	_, err = tx.Exec(`UPDATE sessions SET signaled_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("update signaled_at: %w", err)
	}

	return tx.Commit()
}

// SessionNeedsSignal returns session IDs that haven't been signaled yet.
func (db *DB) SessionNeedsSignal() ([]string, error) {
	rows, err := db.conn.Query(`SELECT id FROM sessions WHERE signaled_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("query unsignaled sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetTurns returns all turns for a session.
func (db *DB) GetTurns(sessionID string) ([]parser.Turn, error) {
	rows, err := db.conn.Query(`SELECT idx, role, content, timestamp, tool_name, is_error FROM turns WHERE session_id = ? ORDER BY idx`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query turns: %w", err)
	}
	defer rows.Close()

	var turns []parser.Turn
	for rows.Next() {
		var t parser.Turn
		var ts, toolName sql.NullString
		var isErr int
		if err := rows.Scan(&t.Idx, &t.Role, &t.Content, &ts, &toolName, &isErr); err != nil {
			return nil, err
		}
		t.ToolName = toolName.String
		t.IsError = isErr != 0
		if ts.Valid {
			t.Time, _ = time.Parse(time.RFC3339, ts.String)
		}
		turns = append(turns, t)
	}
	return turns, rows.Err()
}

// Stats holds database statistics for the info command.
type Stats struct {
	SessionCount int
	TurnCount    int
	SignalCount  int
	BySource     map[string]int
	BySignalType map[string]int
}

// GetStats returns aggregate database statistics.
func (db *DB) GetStats() (*Stats, error) {
	stats := &Stats{
		BySource:     make(map[string]int),
		BySignalType: make(map[string]int),
	}

	db.conn.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&stats.SessionCount)
	db.conn.QueryRow(`SELECT COUNT(*) FROM turns`).Scan(&stats.TurnCount)
	db.conn.QueryRow(`SELECT COUNT(*) FROM signals`).Scan(&stats.SignalCount)

	rows, err := db.conn.Query(`SELECT source, COUNT(*) FROM sessions GROUP BY source`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var source string
			var count int
			rows.Scan(&source, &count)
			stats.BySource[source] = count
		}
	}

	rows2, err := db.conn.Query(`SELECT signal_type, COUNT(*) FROM signals GROUP BY signal_type`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var sigType string
			var count int
			rows2.Scan(&sigType, &count)
			stats.BySignalType[sigType] = count
		}
	}

	return stats, nil
}

// ClearSignals removes all signals for a session (for recompute).
func (db *DB) ClearSignals(sessionID string) error {
	_, err := db.conn.Exec(`DELETE FROM signals WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("clear signals: %w", err)
	}
	_, err = db.conn.Exec(`UPDATE sessions SET signaled_at = NULL WHERE id = ?`, sessionID)
	return err
}

// QueryFilter narrows which sessions QuerySessions returns.
//
// Zero-value fields are ignored. Signals is OR-matched: a session is included
// if any of its signals matches any of the listed types. When Signals is set,
// only matching signals come back in the result — the caller asked for those,
// not the whole grab bag.
type QueryFilter struct {
	Signals       []string  // signal types (OR). empty = any.
	MinConfidence float64   // confidence >= this. 0 = no floor.
	Source        string    // exact match on sessions.source.
	Project       string    // substring match on sessions.project.
	Since         time.Time // started_at >= Since.
	Until         time.Time // started_at <= Until.
	Limit         int       // max sessions returned. 0 = no limit.
}

// SessionMatch is one session that passed the filter, with the matching signals attached.
type SessionMatch struct {
	SessionID string        `json:"session_id"`
	Source    string        `json:"source"`
	Project   string        `json:"project,omitempty"`
	RawPath   string        `json:"raw_path"`
	FileHash  string        `json:"file_hash"`
	StartedAt string        `json:"started_at,omitempty"`
	EndedAt   string        `json:"ended_at,omitempty"`
	Signals   []MatchSignal `json:"signals"`
}

// MatchSignal is the signal subset surfaced by QuerySessions.
type MatchSignal struct {
	Type       string  `json:"type"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"`
	TurnIdx    *int    `json:"turn_idx,omitempty"`
	Evidence   string  `json:"evidence,omitempty"`
}

// QuerySessions returns sessions matching the filter, each with its matching signals.
// Results are ordered by started_at DESC (then session id, for determinism).
func (db *DB) QuerySessions(f QueryFilter) ([]SessionMatch, error) {
	var where []string
	var args []any

	if len(f.Signals) > 0 {
		placeholders := strings.Repeat("?,", len(f.Signals))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, fmt.Sprintf("sig.signal_type IN (%s)", placeholders))
		for _, t := range f.Signals {
			args = append(args, t)
		}
	}
	if f.MinConfidence > 0 {
		where = append(where, "sig.confidence >= ?")
		args = append(args, f.MinConfidence)
	}
	if f.Source != "" {
		where = append(where, "s.source = ?")
		args = append(args, f.Source)
	}
	if f.Project != "" {
		where = append(where, "s.project LIKE ?")
		args = append(args, "%"+f.Project+"%")
	}
	if !f.Since.IsZero() {
		where = append(where, "s.started_at >= ?")
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		where = append(where, "s.started_at <= ?")
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}

	q := `SELECT s.id, s.source, s.project, s.raw_path, s.file_hash, s.started_at, s.ended_at,
		sig.signal_type, sig.signal_category, sig.confidence, sig.turn_idx, sig.evidence
		FROM sessions s
		JOIN signals sig ON sig.session_id = s.id`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	// Order by start desc so newest sessions surface first; signal id keeps
	// signal ordering inside a session stable across runs.
	q += " ORDER BY s.started_at DESC, s.id, sig.id"

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var out []SessionMatch
	byID := make(map[string]int) // session id -> index in out

	for rows.Next() {
		var (
			id, source, rawPath, fileHash string
			project, startedAt, endedAt   sql.NullString
			sigType, sigCat               string
			confidence                    float64
			turnIdx                       sql.NullInt64
			evidence                      sql.NullString
		)
		if err := rows.Scan(&id, &source, &project, &rawPath, &fileHash,
			&startedAt, &endedAt,
			&sigType, &sigCat, &confidence, &turnIdx, &evidence); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}

		idx, ok := byID[id]
		if !ok {
			if f.Limit > 0 && len(out) >= f.Limit {
				continue
			}
			out = append(out, SessionMatch{
				SessionID: id,
				Source:    source,
				Project:   project.String,
				RawPath:   rawPath,
				FileHash:  fileHash,
				StartedAt: startedAt.String,
				EndedAt:   endedAt.String,
			})
			idx = len(out) - 1
			byID[id] = idx
		}

		ms := MatchSignal{
			Type:       sigType,
			Category:   sigCat,
			Confidence: confidence,
			Evidence:   evidence.String,
		}
		if turnIdx.Valid {
			i := int(turnIdx.Int64)
			ms.TurnIdx = &i
		}
		out[idx].Signals = append(out[idx].Signals, ms)
	}
	return out, rows.Err()
}

func formatTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
