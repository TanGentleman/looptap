package patterns

import (
	"path/filepath"
	"testing"

	"looptap/internal/db"
)

// seedPatterns builds a db with the canonical two-cluster fixture: an ENOENT
// cluster across 6 sessions (above a gate of 5) and a connection-refused cluster
// across 2 (below it). It shares SeedContractFixture with the
// `looptap seed-contract-fixture` subcommand so the engine tests and any live
// capture of the binary's output assert against the exact same shape.
func seedPatterns(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	if err := SeedContractFixture(d, false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return d
}

func TestFind(t *testing.T) {
	d := seedPatterns(t)

	clusters, err := Find(d.Conn(), Options{Signals: []string{"failure"}})
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2: %+v", len(clusters), clusters)
	}

	// Sorted by session_count desc: ENOENT (6) before ECONNREFUSED (2).
	top := clusters[0]
	if top.ErrorClass != "ENOENT" || top.Tool != "Bash" || top.Signal != "failure" {
		t.Errorf("top cluster = %s/%s/%s", top.Signal, top.Tool, top.ErrorClass)
	}
	if top.SessionCount() != 6 {
		t.Errorf("top session count = %d, want 6", top.SessionCount())
	}
	if got := top.Summary; got != "failure in Bash (ENOENT)" {
		t.Errorf("summary = %q", got)
	}
	if len(top.Examples) == 0 {
		t.Error("top cluster has no example turns")
	}

	if clusters[1].ErrorClass != "ECONNREFUSED" || clusters[1].SessionCount() != 2 {
		t.Errorf("second cluster = %s (%d)", clusters[1].ErrorClass, clusters[1].SessionCount())
	}
}

func TestFind_CardSynthesis(t *testing.T) {
	d := seedPatterns(t)
	clusters, err := Find(d.Conn(), Options{})
	if err != nil {
		t.Fatal(err)
	}

	card := clusters[0].Card() // ENOENT, 6 sessions
	if card.ID != "failure-bash-enoent" {
		t.Errorf("card id = %q", card.ID)
	}
	if card.Pattern.SessionCount != 6 {
		t.Errorf("card session count = %d", card.Pattern.SessionCount)
	}
	if card.Rule.Confidence != "medium" { // 6 => medium
		t.Errorf("confidence = %q", card.Rule.Confidence)
	}
	if len(card.Pattern.ExampleSessionIDs) == 0 {
		t.Error("no example session ids on card")
	}
	if len(card.Evidence) == 0 || card.Evidence[0].Excerpt == "" {
		t.Error("card carries no evidence excerpt")
	}
}

func TestFind_ProjectFilterAndDefaultSignal(t *testing.T) {
	d := seedPatterns(t)

	// Default signal is "failure"; a project that matches nothing yields nothing.
	none, err := Find(d.Conn(), Options{Project: "does-not-exist"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("project filter leaked %d clusters", len(none))
	}

	some, err := Find(d.Conn(), Options{Project: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(some) != 2 {
		t.Errorf("project substring matched %d clusters, want 2", len(some))
	}
}

func TestFind_Limit(t *testing.T) {
	d := seedPatterns(t)
	clusters, err := Find(d.Conn(), Options{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(clusters) != 1 || clusters[0].ErrorClass != "ENOENT" {
		t.Errorf("limit 1 should keep the biggest cluster, got %+v", clusters)
	}
}
