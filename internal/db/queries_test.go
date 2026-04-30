package db

import (
	"path/filepath"
	"testing"
	"time"

	"looptap/internal/parser"
	"looptap/internal/signal"
)

// seedQuery puts three sessions and a smattering of signals into a fresh db,
// enough to exercise every QueryFilter knob.
func seedQuery(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "q.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	mk := func(id, source, project string, started time.Time) parser.Session {
		return parser.Session{
			ID: id, Source: source, Project: project,
			SessionID: id, StartedAt: started, EndedAt: started.Add(time.Hour),
			RawPath: "/tmp/" + id + ".jsonl", FileHash: "h-" + id,
			Turns: []parser.Turn{{Idx: 0, Role: "user", Content: "x"}},
		}
	}

	jan := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	sessions := []parser.Session{
		mk("alpha", "claude-code", "/repo/foo", jan),
		mk("beta", "claude-code", "/repo/bar", feb),
		mk("gamma", "codex", "/repo/foo", mar),
	}
	for _, s := range sessions {
		if err := d.InsertSession(s); err != nil {
			t.Fatalf("insert session %s: %v", s.ID, err)
		}
	}

	turn := 7
	signals := map[string][]signal.Signal{
		"alpha": {
			{SessionID: "alpha", Type: "failure", Category: "execution", Confidence: 0.9, Evidence: "exit 1", TurnIdx: &turn},
			{SessionID: "alpha", Type: "misalignment", Category: "interaction", Confidence: 0.4},
		},
		"beta": {
			{SessionID: "beta", Type: "failure", Category: "execution", Confidence: 0.6},
		},
		"gamma": {
			{SessionID: "gamma", Type: "loop", Category: "execution", Confidence: 0.8},
		},
	}
	for id, sigs := range signals {
		if err := d.InsertSignals(id, sigs); err != nil {
			t.Fatalf("insert signals %s: %v", id, err)
		}
	}
	return d
}

func TestQuerySessions(t *testing.T) {
	d := seedQuery(t)

	cases := []struct {
		name     string
		filter   QueryFilter
		wantIDs  []string // session ids in expected order
		wantSigs map[string]int
	}{
		{
			name:     "no filter returns every session-with-signals",
			filter:   QueryFilter{},
			wantIDs:  []string{"gamma", "beta", "alpha"}, // started_at DESC
			wantSigs: map[string]int{"alpha": 2, "beta": 1, "gamma": 1},
		},
		{
			name:     "single signal filter",
			filter:   QueryFilter{Signals: []string{"failure"}},
			wantIDs:  []string{"beta", "alpha"},
			wantSigs: map[string]int{"alpha": 1, "beta": 1},
		},
		{
			name:     "OR across signals",
			filter:   QueryFilter{Signals: []string{"failure", "loop"}},
			wantIDs:  []string{"gamma", "beta", "alpha"},
			wantSigs: map[string]int{"alpha": 1, "beta": 1, "gamma": 1},
		},
		{
			name:     "min-confidence drops weak matches",
			filter:   QueryFilter{Signals: []string{"failure", "misalignment"}, MinConfidence: 0.7},
			wantIDs:  []string{"alpha"}, // beta failure 0.6 dropped, alpha misalignment 0.4 dropped, alpha failure 0.9 kept
			wantSigs: map[string]int{"alpha": 1},
		},
		{
			name:     "source filter",
			filter:   QueryFilter{Source: "codex"},
			wantIDs:  []string{"gamma"},
			wantSigs: map[string]int{"gamma": 1},
		},
		{
			name:     "project substring",
			filter:   QueryFilter{Project: "foo"},
			wantIDs:  []string{"gamma", "alpha"},
			wantSigs: map[string]int{"alpha": 2, "gamma": 1},
		},
		{
			name:     "since/until window",
			filter:   QueryFilter{Since: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)},
			wantIDs:  []string{"beta"},
			wantSigs: map[string]int{"beta": 1},
		},
		{
			name:     "limit caps sessions",
			filter:   QueryFilter{Limit: 2},
			wantIDs:  []string{"gamma", "beta"},
			wantSigs: map[string]int{"beta": 1, "gamma": 1},
		},
		{
			name:    "no match → empty slice, no error",
			filter:  QueryFilter{Signals: []string{"ghost"}},
			wantIDs: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.QuerySessions(tc.filter)
			if err != nil {
				t.Fatalf("QuerySessions: %v", err)
			}
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %d sessions, want %d (%v)", len(got), len(tc.wantIDs), tc.wantIDs)
			}
			for i, want := range tc.wantIDs {
				if got[i].SessionID != want {
					t.Errorf("position %d: got %s, want %s", i, got[i].SessionID, want)
				}
				if n := tc.wantSigs[want]; n != 0 && len(got[i].Signals) != n {
					t.Errorf("session %s: got %d signals, want %d", want, len(got[i].Signals), n)
				}
				if got[i].RawPath == "" {
					t.Errorf("session %s: raw_path missing — that's the whole point of this query", want)
				}
			}
		})
	}
}

func TestQuerySessionsPreservesTurnIdx(t *testing.T) {
	d := seedQuery(t)
	got, err := d.QuerySessions(QueryFilter{Signals: []string{"failure"}, Source: "claude-code", Project: "foo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "alpha" {
		t.Fatalf("setup wrong: %+v", got)
	}
	sigs := got[0].Signals
	if len(sigs) != 1 {
		t.Fatalf("want 1 failure signal, got %d", len(sigs))
	}
	if sigs[0].TurnIdx == nil || *sigs[0].TurnIdx != 7 {
		t.Errorf("turn_idx round-trip lost: %v", sigs[0].TurnIdx)
	}
}
