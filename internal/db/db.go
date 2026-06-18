package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection for looptap.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying sql.DB for advanced use.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		id          TEXT PRIMARY KEY,
		source      TEXT NOT NULL,
		project     TEXT,
		session_id  TEXT NOT NULL,
		started_at  TEXT,
		ended_at    TEXT,
		model       TEXT,
		total_turns INTEGER NOT NULL,
		tool_calls  INTEGER NOT NULL DEFAULT 0,
		git_branch  TEXT,
		raw_path    TEXT NOT NULL,
		file_hash   TEXT NOT NULL,
		owner_user  TEXT,
		owner_team  TEXT,
		parsed_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
		signaled_at TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS turns (
		session_id TEXT NOT NULL REFERENCES sessions(id),
		idx        INTEGER NOT NULL,
		role       TEXT NOT NULL,
		content    TEXT,
		timestamp  TEXT,
		tool_name  TEXT,
		is_error   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (session_id, idx)
	)`,
	`CREATE TABLE IF NOT EXISTS signals (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id      TEXT NOT NULL REFERENCES sessions(id),
		signal_type     TEXT NOT NULL,
		signal_category TEXT NOT NULL,
		turn_idx        INTEGER,
		confidence      REAL NOT NULL,
		evidence        TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_signals_type ON signals(signal_type)`,
	`CREATE INDEX IF NOT EXISTS idx_signals_session ON signals(session_id)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_source ON sessions(source)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project)`,
}

func (db *DB) migrate() error {
	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("executing migration: %w\nSQL: %s", err, m)
		}
	}
	return db.ensureSessionColumns()
}

// ensureSessionColumns brings older databases up to the current sessions shape.
// SQLite has no "ADD COLUMN IF NOT EXISTS", so we ask the table what it already
// has and only add what's missing — that keeps migrate() idempotent across runs.
func (db *DB) ensureSessionColumns() error {
	have, err := db.columnSet("sessions")
	if err != nil {
		return fmt.Errorf("inspecting sessions columns: %w", err)
	}

	// column name -> DDL to add it. Attribution arrived after the first schema,
	// so existing rows get NULL (read back as "" — an honest "unknown owner").
	additions := []struct{ name, ddl string }{
		{"owner_user", `ALTER TABLE sessions ADD COLUMN owner_user TEXT`},
		{"owner_team", `ALTER TABLE sessions ADD COLUMN owner_team TEXT`},
	}
	for _, a := range additions {
		if have[a.name] {
			continue
		}
		if _, err := db.conn.Exec(a.ddl); err != nil {
			return fmt.Errorf("adding column %s: %w", a.name, err)
		}
	}

	// Indexes live here (not in the migrations slice) because they reference
	// columns that only exist after the ALTERs above land.
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(owner_user)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_team ON sessions(owner_team)`,
	} {
		if _, err := db.conn.Exec(idx); err != nil {
			return fmt.Errorf("creating attribution index: %w", err)
		}
	}
	return nil
}

// columnSet returns the set of column names on a table.
func (db *DB) columnSet(table string) (map[string]bool, error) {
	rows, err := db.conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var (
			cid         int
			name, ctype string
			notNull     int
			dflt        sql.NullString
			pk          int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
}
