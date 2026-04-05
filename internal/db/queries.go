package db

import (
	"database/sql"
	"fmt"
	"looptap/internal/parser"
	"looptap/internal/signal"
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

func formatTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
