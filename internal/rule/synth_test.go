package rule

import (
	"strings"
	"testing"
)

func TestSynthesize_ENOENT(t *testing.T) {
	p := Pattern{
		Signal:            "failure",
		Tool:              "Bash",
		ErrorClass:        "ENOENT",
		Summary:           "failure in Bash (ENOENT)",
		SessionCount:      7,
		ExampleSessionIDs: []string{"9ffb1c2d", "4d308a4c"},
	}
	examples := []ExampleTurn{
		{SessionID: "9ffb1c2dlongtail", TurnIdx: 42, ToolName: "Bash", IsError: true,
			Content: "cd packages/api && npm test\nbash: cd: packages/api: No such file or directory"},
	}

	card := Synthesize(p, examples)

	if card.ID != "failure-bash-enoent" {
		t.Errorf("id = %q, want failure-bash-enoent", card.ID)
	}
	if !strings.Contains(card.Rule.Snippet, "confirm the directory exists") {
		t.Errorf("snippet didn't pick the ENOENT template: %q", card.Rule.Snippet)
	}
	if card.Rule.Target != TargetAgents {
		t.Errorf("target = %q", card.Rule.Target)
	}
	if card.Rule.Confidence != "medium" { // 7 sessions => medium
		t.Errorf("confidence = %q, want medium", card.Rule.Confidence)
	}
	if card.Rule.Source != SourceTemplate {
		t.Errorf("source = %q", card.Rule.Source)
	}
	if !strings.Contains(card.Rule.Rationale, "seen in 7 sessions") {
		t.Errorf("rationale missing session tally: %q", card.Rule.Rationale)
	}
	if len(card.Evidence) != 1 {
		t.Fatalf("evidence count = %d, want 1", len(card.Evidence))
	}
	if card.Evidence[0].SessionID != "9ffb1c2d" { // shortened to 8 chars
		t.Errorf("evidence session id not shortened: %q", card.Evidence[0].SessionID)
	}
	if !card.Evidence[0].IsError {
		t.Error("evidence lost is_error")
	}
}

func TestSynthesize_ConfidenceLadder(t *testing.T) {
	cases := []struct {
		sessions int
		want     string
	}{
		{2, "low"},
		{5, "medium"},
		{9, "medium"},
		{10, "high"},
		{50, "high"},
	}
	for _, tc := range cases {
		p := Pattern{Signal: "failure", Tool: "Bash", ErrorClass: "exit-code", SessionCount: tc.sessions}
		if got := Synthesize(p, nil).Rule.Confidence; got != tc.want {
			t.Errorf("%d sessions: confidence = %q, want %q", tc.sessions, got, tc.want)
		}
	}
}

func TestSynthesize_GenericFallback(t *testing.T) {
	// A signal/error-class with no specific template must still yield a usable
	// card — never an empty rule.
	p := Pattern{Signal: "failure", Tool: "Glob", ErrorClass: "some weird novel error", SessionCount: 6}
	card := Synthesize(p, nil)

	if card.Rule.Title == "" || card.Rule.Snippet == "" {
		t.Errorf("generic fallback produced an empty rule: %+v", card.Rule)
	}
	if card.Rule.Source != SourceTemplate {
		t.Errorf("source = %q", card.Rule.Source)
	}
	if !strings.Contains(card.Rule.Title, "Glob") {
		t.Errorf("generic title should mention the tool: %q", card.Rule.Title)
	}
}

func TestSynthesize_EvidenceCappedAtThree(t *testing.T) {
	var examples []ExampleTurn
	for i := 0; i < 5; i++ {
		examples = append(examples, ExampleTurn{SessionID: "s", TurnIdx: i, Content: "boom"})
	}
	card := Synthesize(Pattern{Signal: "failure"}, examples)
	if len(card.Evidence) != maxEvidence {
		t.Errorf("evidence = %d, want %d", len(card.Evidence), maxEvidence)
	}
}

func TestShortID(t *testing.T) {
	if got := ShortID("9ffb1c2d4e5f6a7b"); got != "9ffb1c2d" {
		t.Errorf("ShortID long = %q", got)
	}
	if got := ShortID("alpha"); got != "alpha" {
		t.Errorf("ShortID short = %q", got)
	}
}
