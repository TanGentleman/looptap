package patterns

import (
	"path/filepath"
	"strings"
	"testing"

	"looptap/internal/db"
	"looptap/internal/rule"
)

// openSeeded plants the contract fixture into a throwaway db and returns it.
func openSeeded(t *testing.T, leaky bool) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := SeedContractFixture(d, leaky); err != nil {
		t.Fatalf("seed(leaky=%t): %v", leaky, err)
	}
	return d
}

// clusterByClass finds the cluster for an error class, failing if it's absent.
func clusterByClass(t *testing.T, cs []Cluster, class string) Cluster {
	t.Helper()
	for _, c := range cs {
		if c.ErrorClass == class {
			return c
		}
	}
	t.Fatalf("no %s cluster in %+v", class, cs)
	return Cluster{}
}

// TestSeedContractFixture_Counts proves the shape both the engine tests and the
// live capture depend on: ENOENT clears the gate at 6 distinct sessions and the
// connection-refused shape sits at 2 — and --leaky does NOT move either count,
// because its extra turn rides an existing ENOENT session.
func TestSeedContractFixture_Counts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		leaky bool
	}{
		{"plain", false},
		{"leaky", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := openSeeded(t, tc.leaky)
			clusters, err := Find(d.Conn(), Options{})
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			if len(clusters) != 2 {
				t.Fatalf("got %d clusters, want 2: %+v", len(clusters), clusters)
			}

			enoent := clusterByClass(t, clusters, "ENOENT")
			if got := enoent.SessionCount(); got != 6 {
				t.Errorf("ENOENT session count = %d, want 6 (leaky must not add a session)", got)
			}
			conn := clusterByClass(t, clusters, "ECONNREFUSED")
			if got := conn.SessionCount(); got != 2 {
				t.Errorf("ECONNREFUSED session count = %d, want 2", got)
			}

			// 6 sessions lands the ENOENT card squarely in "medium" confidence.
			if got := enoent.Card().Rule.Confidence; got != "medium" {
				t.Errorf("ENOENT confidence = %q, want medium", got)
			}
		})
	}
}

// TestSeedContractFixture_Redaction is the point of --leaky: engine output must
// carry a scrubbed secret (and only when asked).
func TestSeedContractFixture_Redaction(t *testing.T) {
	t.Run("plain carries no secret", func(t *testing.T) {
		d := openSeeded(t, false)
		clusters, err := Find(d.Conn(), Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, ev := range clusterByClass(t, clusters, "ENOENT").Card().Evidence {
			if ev.Redactions != 0 {
				t.Errorf("plain evidence reported %d redactions, want 0: %q", ev.Redactions, ev.Excerpt)
			}
			if strings.Contains(ev.Excerpt, "[REDACTED]") {
				t.Errorf("plain evidence carries a placeholder it shouldn't: %q", ev.Excerpt)
			}
		}
	})

	t.Run("leaky scrubs the planted key in place", func(t *testing.T) {
		d := openSeeded(t, true)
		clusters, err := Find(d.Conn(), Options{})
		if err != nil {
			t.Fatal(err)
		}
		card := clusterByClass(t, clusters, "ENOENT").Card()

		var redacted []rule.Evidence
		for _, ev := range card.Evidence {
			// The raw key must never reach the wire — looptap's pre-pass eats it.
			if strings.Contains(ev.Excerpt, "sk-ant") {
				t.Fatalf("raw secret survived into evidence: %q", ev.Excerpt)
			}
			if ev.Redactions > 0 {
				redacted = append(redacted, ev)
			}
		}

		if len(redacted) != 1 {
			t.Fatalf("got %d redacted evidence rows, want exactly 1", len(redacted))
		}
		ev := redacted[0]
		if !strings.Contains(ev.Excerpt, "[REDACTED]") {
			t.Errorf("redacted row missing placeholder: %q", ev.Excerpt)
		}
		// The surrounding ENOENT context survives the scrub, so the row still
		// clusters as ENOENT and a consumer can find it by the leaked-key label.
		if !strings.Contains(ev.Excerpt, "ANTHROPIC_API_KEY") {
			t.Errorf("redacted row lost its context: %q", ev.Excerpt)
		}
		if !strings.Contains(ev.Excerpt, "No such file or directory") {
			t.Errorf("leaky turn no longer reads as ENOENT: %q", ev.Excerpt)
		}
	})
}
