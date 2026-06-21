package rule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// goldenCard is the exact Card from the tracers spec, read from the shared
// fixture so there is a single source of truth: testdata/contracts/ is what
// tracers copies, so it's what we pin against here too. If this stops
// round-tripping, looptap and tracers have drifted apart — the whole point of
// the shared record is gone. Treat a failure here as a contract break, not a
// test to "fix".
func goldenCard(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(contractsDir, "tracers.rule.v1.golden-card.json"))
	if err != nil {
		t.Fatalf("read golden card: %v", err)
	}
	return b
}

func TestCard_GoldenRoundTrip(t *testing.T) {
	golden := goldenCard(t)

	var c Card
	if err := json.Unmarshal(golden, &c); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	// Spot-check that the contract's field names landed where we expect — a
	// silent JSON-tag rename would scan into the zero value.
	if c.ID != "failure-bash-enoent" {
		t.Errorf("id = %q", c.ID)
	}
	if c.Pattern.ErrorClass != "ENOENT" {
		t.Errorf("pattern.error_class = %q", c.Pattern.ErrorClass)
	}
	if c.Pattern.SessionCount != 7 {
		t.Errorf("pattern.session_count = %d", c.Pattern.SessionCount)
	}
	if got := c.Pattern.ExampleSessionIDs; len(got) != 3 || got[0] != "9ffb1c2d" {
		t.Errorf("pattern.example_session_ids = %v", got)
	}
	if len(c.Evidence) != 1 || !c.Evidence[0].IsError || c.Evidence[0].TurnIdx != 42 {
		t.Errorf("evidence = %+v", c.Evidence)
	}
	if c.Rule.Target != TargetAgents || c.Rule.Confidence != "medium" || c.Rule.Source != SourceTemplate {
		t.Errorf("rule = %+v", c.Rule)
	}

	// Re-marshal and compare structurally (key order / whitespace agnostic):
	// our struct must reproduce the spec's shape with no extra or missing keys.
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var want, got map[string]any
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("round-trip drift:\n golden: %v\n  ours: %v", want, got)
	}
}

func TestNewBundle(t *testing.T) {
	var c Card
	if err := json.Unmarshal(goldenCard(t), &c); err != nil {
		t.Fatal(err)
	}

	b := NewBundle([]Card{c})
	if b.Schema != Schema {
		t.Errorf("schema = %q, want %q", b.Schema, Schema)
	}
	if _, err := time.Parse(time.RFC3339, b.GeneratedAt); err != nil {
		t.Errorf("generated_at %q not RFC3339: %v", b.GeneratedAt, err)
	}
	if len(b.Cards) != 1 {
		t.Fatalf("cards = %d, want 1", len(b.Cards))
	}
}

func TestMarshalBundle_NilCardsIsEmptyArray(t *testing.T) {
	// A bundle with nothing in it must serialize "cards": [] — never null —
	// so consumers don't have to special-case it.
	out, err := MarshalBundle(nil)
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Cards *[]Card `json:"cards"`
	}
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatal(err)
	}
	if b.Cards == nil {
		t.Fatal("cards was null; want []")
	}
	if len(*b.Cards) != 0 {
		t.Errorf("cards = %d, want 0", len(*b.Cards))
	}
}
