package rule

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// goldenCard is the exact Card from the tracers spec (docs/rule-with-evidence.md).
// If this stops round-tripping, looptap and tracers have drifted apart — the
// whole point of the shared record is gone. Treat a failure here as a contract
// break, not a test to "fix".
const goldenCard = `{
  "id": "failure-bash-enoent",
  "pattern": {
    "signal": "failure",
    "tool": "Bash",
    "error_class": "ENOENT",
    "summary": "Bash commands fail with \"No such file or directory\" on paths the agent assumed existed",
    "session_count": 7,
    "example_session_ids": ["9ffb1c2d", "4d308a4c", "c8e2f10a"]
  },
  "evidence": [
    {
      "session_id": "9ffb1c2d",
      "turn_idx": 42,
      "tool_name": "Bash",
      "is_error": true,
      "excerpt": "cd packages/api && npm test\nbash: cd: packages/api: No such file or directory",
      "redactions": 0
    }
  ],
  "rule": {
    "title": "Verify a path exists before cd-ing into it",
    "snippet": "Before ` + "`cd <dir>`" + ` or running a command in a subdir, confirm the directory exists (e.g. ` + "`ls <dir>`" + `); don't assume a path from memory.",
    "rationale": "Bash steps repeatedly fail with ENOENT on directories the agent assumed were present, costing a retry loop.",
    "target": "AGENTS.md",
    "confidence": "medium",
    "source": "template"
  },
  "signature": ""
}`

func TestCard_GoldenRoundTrip(t *testing.T) {
	var c Card
	if err := json.Unmarshal([]byte(goldenCard), &c); err != nil {
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
	if err := json.Unmarshal([]byte(goldenCard), &want); err != nil {
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
	if err := json.Unmarshal([]byte(goldenCard), &c); err != nil {
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
