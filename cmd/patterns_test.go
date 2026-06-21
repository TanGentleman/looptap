package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"looptap/internal/db"
	"looptap/internal/parser"
	"looptap/internal/rule"
	"looptap/internal/signal"
)

// seedDB writes the same two-cluster fixture the patterns engine test uses, but
// here we drive it through the actual cobra command to prove the gate.
func seedDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cmd.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	add := func(id string, when time.Time, content string) {
		s := parser.Session{
			ID: id, Source: "claude-code", Project: "/repo/app",
			SessionID: id, StartedAt: when, EndedAt: when.Add(time.Hour),
			RawPath: "/tmp/" + id + ".jsonl", FileHash: "h-" + id,
			Turns: []parser.Turn{
				{Idx: 0, Role: "user", Content: "go"},
				{Idx: 1, Role: "tool_result", ToolName: "Bash", IsError: true, Content: content},
			},
		}
		if err := d.InsertSession(s); err != nil {
			t.Fatal(err)
		}
		turn := 1
		if err := d.InsertSignals(id, []signal.Signal{
			{SessionID: id, Type: "failure", Category: "execution", Confidence: 0.9, TurnIdx: &turn},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 6; i++ {
		add(fmt.Sprintf("enoent-%d", i), base.Add(time.Duration(i)*time.Hour),
			"cd packages/api: No such file or directory")
	}
	for i := 0; i < 2; i++ {
		add(fmt.Sprintf("conn-%d", i), base.Add(time.Duration(10+i)*time.Hour),
			"dial tcp: connection refused")
	}
	return path
}

func runPatterns(t *testing.T, dbPath string, args ...string) (stdout, stderr string) {
	t.Helper()
	dbp := dbPath
	cmd := NewPatternsCmd(&dbp)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return out.String(), errBuf.String()
}

func TestPatternsCmd_JSONGate(t *testing.T) {
	path := seedDB(t)
	out, _ := runPatterns(t, path, "--format", "json")

	var bundle rule.Bundle
	if err := json.Unmarshal([]byte(out), &bundle); err != nil {
		t.Fatalf("bundle isn't valid json: %v\n%s", err, out)
	}
	if bundle.Schema != rule.Schema {
		t.Errorf("schema = %q, want %q", bundle.Schema, rule.Schema)
	}
	if bundle.GateMinSessions != 5 {
		t.Errorf("gate_min_sessions = %d, want 5", bundle.GateMinSessions)
	}
	// Only the 6-session ENOENT cluster clears the default gate of 5.
	if len(bundle.Cards) != 1 {
		t.Fatalf("got %d cards, want 1 (gate should drop the 2-session cluster)", len(bundle.Cards))
	}
	if bundle.Cards[0].Pattern.ErrorClass != "ENOENT" {
		t.Errorf("surviving card = %q", bundle.Cards[0].Pattern.ErrorClass)
	}
	if bundle.Cards[0].Pattern.SessionCount != 6 {
		t.Errorf("session count = %d", bundle.Cards[0].Pattern.SessionCount)
	}
}

func TestPatternsCmd_IncludeBelowGate(t *testing.T) {
	path := seedDB(t)
	out, stderr := runPatterns(t, path, "--format", "json", "--include-below-gate")

	var bundle rule.Bundle
	if err := json.Unmarshal([]byte(out), &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.Cards) != 2 {
		t.Errorf("got %d cards, want 2 with --include-below-gate", len(bundle.Cards))
	}
	wantWarn := "WARN: emitting sub-gate clusters; tracers will skip these on ingest unless session_count >= gate_min_sessions"
	if !strings.Contains(stderr, wantWarn) {
		t.Errorf("stderr missing warning:\n%s", stderr)
	}
	if strings.Contains(out, "WARN:") {
		t.Errorf("warning leaked to stdout:\n%s", out)
	}
}

func TestPatternsCmd_LowerGateLetsBothThrough(t *testing.T) {
	path := seedDB(t)
	out, _ := runPatterns(t, path, "--format", "json", "--min-sessions", "3")

	var bundle rule.Bundle
	if err := json.Unmarshal([]byte(out), &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.GateMinSessions != 3 {
		t.Errorf("gate_min_sessions = %d, want 3", bundle.GateMinSessions)
	}
	if len(bundle.Cards) != 1 {
		t.Errorf("got %d cards, want 1 at --min-sessions 3 (only 6-session cluster clears)", len(bundle.Cards))
	}
}

func TestPatternsCmd_LowerGateTwoSessions(t *testing.T) {
	path := seedDB(t)
	out, _ := runPatterns(t, path, "--format", "json", "--min-sessions", "2")

	var bundle rule.Bundle
	if err := json.Unmarshal([]byte(out), &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.Cards) != 2 {
		t.Errorf("got %d cards, want 2 at --min-sessions 2", len(bundle.Cards))
	}
}

func TestPatternsCmd_TextShowsBothWithGateMark(t *testing.T) {
	path := seedDB(t)
	out, _ := runPatterns(t, path) // text is the default

	if !strings.Contains(out, "ENOENT") || !strings.Contains(out, "ECONNREFUSED") {
		t.Errorf("text output should list both clusters:\n%s", out)
	}
	if !strings.Contains(out, "below gate") {
		t.Errorf("the 2-session cluster should be marked below gate:\n%s", out)
	}
}

// TestPatternsCmd_MatchesGoldenBundleShape drives the real command and checks
// its JSON envelope against the golden bundle fixture key-for-key. The values
// differ (seeded sessions, live timestamp) but the *shape* is the contract: if
// the command sprouts or drops a field relative to testdata/contracts, tracers'
// parser breaks. generated_at and scalar values are intentionally ignored.
func TestPatternsCmd_MatchesGoldenBundleShape(t *testing.T) {
	path := seedDB(t)
	out, _ := runPatterns(t, path, "--format", "json")

	raw, err := os.ReadFile(filepath.Join("..", "testdata", "contracts", "tracers.rule.v1.golden-bundle.json"))
	if err != nil {
		t.Fatalf("read golden bundle: %v", err)
	}

	var golden, got map[string]any
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("command output isn't valid json: %v\n%s", err, out)
	}

	if !sameJSONShape(golden, got) {
		t.Errorf("patterns json drifted from golden bundle shape:\n golden: %s\n   got: %s", raw, out)
	}
}

// sameJSONShape compares two decoded-JSON values by structure only: objects
// must share a key set (recursively), arrays must share an element shape;
// scalar values are ignored. (A sibling of internal/rule's sameShape, kept here
// because cmd can't import a test from another package.)
func sameJSONShape(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, va := range av {
			vb, ok := bv[k]
			if !ok || !sameJSONShape(va, vb) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return false
		}
		if len(av) == 0 || len(bv) == 0 {
			return true
		}
		for _, e := range av {
			if !sameJSONShape(e, bv[0]) {
				return false
			}
		}
		for _, e := range bv {
			if !sameJSONShape(e, av[0]) {
				return false
			}
		}
		return true
	default:
		switch b.(type) {
		case map[string]any, []any:
			return false
		}
		return true
	}
}

func TestPatternsCmd_UnknownFormat(t *testing.T) {
	path := seedDB(t)
	dbp := path
	cmd := NewPatternsCmd(&dbp)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--format", "yaml"})
	if err := cmd.Execute(); err == nil {
		t.Error("expected an error for --format yaml")
	}
}
