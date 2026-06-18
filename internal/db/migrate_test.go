package db

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// oldSchema is the sessions table as it shipped before attribution — no
// owner_user / owner_team. ensureSessionColumns has to upgrade it in place.
const oldSchema = `CREATE TABLE sessions (
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
	parsed_at   TEXT,
	signaled_at TEXT
)`

func TestEnsureSessionColumns_UpgradesOldDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// Stand up a pre-attribution database directly, with one row in it.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (id, source, session_id, total_turns, raw_path, file_hash)
		VALUES ('old1', 'claude-code', 's', 3, '/p', 'h')`); err != nil {
		t.Fatalf("seed old row: %v", err)
	}
	raw.Close()

	// Open via looptap — migrate() must add the missing columns without losing the row.
	d, err := Open(path)
	if err != nil {
		t.Fatalf("open (upgrade): %v", err)
	}
	defer d.Close()

	cols, err := d.columnSet("sessions")
	if err != nil {
		t.Fatalf("columnSet: %v", err)
	}
	for _, want := range []string{"owner_user", "owner_team"} {
		if !cols[want] {
			t.Errorf("expected column %q after upgrade", want)
		}
	}

	// The pre-existing row survives and reads back as unattributed ("").
	var user, team sql.NullString
	if err := d.conn.QueryRow(`SELECT owner_user, owner_team FROM sessions WHERE id = 'old1'`).
		Scan(&user, &team); err != nil {
		t.Fatalf("query upgraded row: %v", err)
	}
	if user.Valid || team.Valid {
		t.Errorf("expected NULL attribution on legacy row, got user=%v team=%v", user, team)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idem.db")

	// Opening the same database three times must never error — migrate() and
	// ensureSessionColumns are run every time.
	for i := 0; i < 3; i++ {
		d, err := Open(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		d.Close()
	}
}
