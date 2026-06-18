package aggregate

import (
	"path/filepath"
	"testing"
	"time"

	"looptap/internal/db"
	"looptap/internal/parser"
	"looptap/internal/signal"
)

// fixture wires up a small but realistic fleet: two teams, three users, a mix
// of failed tool calls, a loop, and a misalignment — enough to exercise every
// rollup in one database.
func fixture(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "agg.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	// alice (payments): two Bash/Edit failures + a misalignment.
	insertSession(t, database, "alice", "payments", "2026-05-01T10:00:00Z",
		[]parser.Turn{
			{Idx: 0, Role: "tool_use", ToolName: "Bash", Content: `{"cmd":"go test"}`},
			{Idx: 1, Role: "tool_result", IsError: true, Content: "exit code 1"},
			{Idx: 2, Role: "user", Content: "no, that's wrong"},
		},
		[]signal.Signal{
			{Type: "failure", Category: "execution", TurnIdx: intPtr(1), Confidence: 0.9, Evidence: "tool_result with is_error=true"},
			{Type: "misalignment", Category: "interaction", TurnIdx: intPtr(2), Confidence: 0.8, Evidence: "correction: no"},
		})

	insertSession(t, database, "alice", "payments", "2026-05-02T10:00:00Z",
		[]parser.Turn{
			{Idx: 0, Role: "tool_use", ToolName: "Edit", Content: `{"path":"x"}`},
			{Idx: 1, Role: "tool_result", IsError: true, Content: "permission denied"},
		},
		[]signal.Signal{
			{Type: "failure", Category: "execution", TurnIdx: intPtr(1), Confidence: 0.9, Evidence: "tool_result with is_error=true"},
		})

	// bob (payments): a Bash loop. Loop signals point at a tool_use turn.
	insertSession(t, database, "bob", "payments", "2026-05-03T10:00:00Z",
		[]parser.Turn{
			{Idx: 0, Role: "tool_use", ToolName: "Bash", Content: `{"cmd":"ls"}`},
			{Idx: 1, Role: "tool_use", ToolName: "Bash", Content: `{"cmd":"ls"}`},
			{Idx: 2, Role: "tool_use", ToolName: "Bash", Content: `{"cmd":"ls"}`},
		},
		[]signal.Signal{
			{Type: "loop", Category: "execution", TurnIdx: intPtr(2), Confidence: 0.5, Evidence: "Bash called 3 times with similar args"},
		})

	// carol (search): a Bash failure — same evidence as alice, different team.
	insertSession(t, database, "carol", "search", "2026-05-04T10:00:00Z",
		[]parser.Turn{
			{Idx: 0, Role: "tool_use", ToolName: "Bash", Content: `{"cmd":"make"}`},
			{Idx: 1, Role: "tool_result", IsError: true, Content: "build failed"},
		},
		[]signal.Signal{
			{Type: "failure", Category: "execution", TurnIdx: intPtr(1), Confidence: 0.9, Evidence: "tool_result with is_error=true"},
		})

	return database
}

func TestRun_Cohort(t *testing.T) {
	r := mustRun(t, fixture(t), Filter{})

	eq(t, "sessions", 4, r.Cohort.Sessions)
	eq(t, "users", 3, r.Cohort.Users)
	eq(t, "teams", 2, r.Cohort.Teams)
	eq(t, "signals", 5, r.Cohort.Signals)
	if r.Cohort.EarliestSeen != "2026-05-01T10:00:00Z" {
		t.Errorf("earliest = %q", r.Cohort.EarliestSeen)
	}
	if r.Cohort.LatestSeen != "2026-05-04T10:00:00Z" {
		t.Errorf("latest = %q", r.Cohort.LatestSeen)
	}
}

func TestRun_FailingTools(t *testing.T) {
	r := mustRun(t, fixture(t), Filter{})

	byTool := map[string]ToolStat{}
	for _, ts := range r.FailingTools {
		byTool[ts.Tool] = ts
	}

	bash, ok := byTool["Bash"]
	if !ok {
		t.Fatal("Bash should appear in failing tools")
	}
	eq(t, "Bash failures", 2, bash.Failures) // alice s1 + carol s4
	eq(t, "Bash loops", 1, bash.Loops)       // bob's loop
	eq(t, "Bash sessions", 3, bash.SessionsAffected)
	eq(t, "Bash users", 3, bash.UsersAffected) // alice, bob, carol
	eq(t, "Bash teams", 2, bash.TeamsAffected) // payments, search

	edit, ok := byTool["Edit"]
	if !ok {
		t.Fatal("Edit should appear in failing tools")
	}
	eq(t, "Edit failures", 1, edit.Failures)
	eq(t, "Edit users", 1, edit.UsersAffected)

	// Bash is the worst, so it sorts first.
	if len(r.FailingTools) == 0 || r.FailingTools[0].Tool != "Bash" {
		t.Errorf("expected Bash first, got %+v", r.FailingTools)
	}
}

func TestRun_RecurringPatterns(t *testing.T) {
	r := mustRun(t, fixture(t), Filter{})

	var errPattern *PatternStat
	for i := range r.RecurringPatterns {
		if r.RecurringPatterns[i].Evidence == "tool_result with is_error=true" {
			errPattern = &r.RecurringPatterns[i]
		}
		// Single-occurrence evidence (the misalignment, the loop) must not surface.
		if r.RecurringPatterns[i].Occurrences < 2 {
			t.Errorf("non-recurring evidence leaked: %+v", r.RecurringPatterns[i])
		}
	}
	if errPattern == nil {
		t.Fatal("the shared is_error evidence should cluster")
	}
	eq(t, "pattern occurrences", 3, errPattern.Occurrences) // alice x2 + carol
	eq(t, "pattern users", 2, errPattern.Users)             // alice, carol
	eq(t, "pattern teams", 2, errPattern.Teams)
}

func TestRun_TeamsAndUsers(t *testing.T) {
	r := mustRun(t, fixture(t), Filter{})

	byTeam := map[string]TeamStat{}
	for _, ts := range r.Teams {
		byTeam[ts.Team] = ts
	}
	payments := byTeam["payments"]
	eq(t, "payments sessions", 3, payments.Sessions)
	eq(t, "payments users", 2, payments.Users)
	eq(t, "payments signals", 4, payments.Signals) // alice's 3 + bob's 1

	search := byTeam["search"]
	eq(t, "search sessions", 1, search.Sessions)
	eq(t, "search signals", 1, search.Signals)

	// alice generated the most signal, so she leads the user list.
	if len(r.Users) == 0 || r.Users[0].User != "alice" {
		t.Fatalf("expected alice first, got %+v", r.Users)
	}
	eq(t, "alice signals", 3, r.Users[0].Signals)
	if r.Users[0].Team != "payments" {
		t.Errorf("alice team = %q", r.Users[0].Team)
	}
}

func TestRun_Filters(t *testing.T) {
	database := fixture(t)

	t.Run("team scope", func(t *testing.T) {
		r := mustRun(t, database, Filter{Team: "search"})
		eq(t, "sessions", 1, r.Cohort.Sessions)
		eq(t, "teams", 1, r.Cohort.Teams)
		eq(t, "users", 1, r.Cohort.Users)
	})

	t.Run("min-confidence drops the loop", func(t *testing.T) {
		// The loop signal is conf 0.5; everything else is >= 0.8.
		r := mustRun(t, database, Filter{MinConfidence: 0.8})
		eq(t, "signals", 4, r.Cohort.Signals) // the 0.5 loop is filtered out
		for _, ts := range r.FailingTools {
			if ts.Loops != 0 {
				t.Errorf("no loop should survive the 0.8 floor, got %+v", ts)
			}
		}
	})

	t.Run("since filter", func(t *testing.T) {
		since, _ := time.Parse(time.RFC3339, "2026-05-03T00:00:00Z")
		r := mustRun(t, database, Filter{Since: since})
		eq(t, "sessions", 2, r.Cohort.Sessions) // bob + carol
	})
}

func TestRun_EmptyDB(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	r := mustRun(t, database, Filter{})
	eq(t, "sessions", 0, r.Cohort.Sessions)
	if len(r.FailingTools) != 0 {
		t.Errorf("expected no failing tools, got %+v", r.FailingTools)
	}
	if len(r.RecurringPatterns) != 0 {
		t.Errorf("expected no patterns, got %+v", r.RecurringPatterns)
	}
	eq(t, "TopN default", defaultTopN, r.Filter.TopN)
}

// --- helpers ---

func mustRun(t *testing.T, database *db.DB, f Filter) *Report {
	t.Helper()
	r, err := Run(database.Conn(), f)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return r
}

func eq(t *testing.T, label string, want, got int) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %d, got %d", label, want, got)
	}
}

func insertSession(t *testing.T, database *db.DB, user, team, started string, turns []parser.Turn, signals []signal.Signal) {
	t.Helper()
	startedAt, err := time.Parse(time.RFC3339, started)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}

	id := user + "-" + started // unique enough for a fixture
	s := parser.Session{
		ID:        id,
		Source:    "claude-code",
		Project:   "demo",
		SessionID: id,
		StartedAt: startedAt,
		EndedAt:   startedAt,
		User:      user,
		Team:      team,
		Turns:     turns,
	}
	if err := database.InsertSession(s); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	for i := range signals {
		signals[i].SessionID = id
	}
	if err := database.InsertSignals(id, signals); err != nil {
		t.Fatalf("insert signals: %v", err)
	}
}

func intPtr(i int) *int { return &i }
