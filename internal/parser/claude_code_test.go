package parser

import (
	"testing"
)

func TestClaudeCodeCanParse(t *testing.T) {
	c := &ClaudeCode{}

	yes := []string{
		"/home/user/.claude/projects/-hash/abc-123.jsonl",
		"/Users/dev/.claude/projects/foo/session.jsonl",
		"/home/user/.claude/projects/-hash/sessions/abc-123.jsonl", // old layout
	}
	no := []string{
		"/home/user/.claude/projects/-hash/abc-123.json",          // wrong extension
		"/home/user/.codex/sessions/abc.jsonl",                     // wrong agent
		"/random/path/file.jsonl",                                  // wrong location
		"/home/user/.claude/projects/-hash/sub/subagents/a.jsonl", // subagent
	}

	for _, p := range yes {
		if !c.CanParse(p) {
			t.Errorf("expected CanParse(%q) = true", p)
		}
	}
	for _, p := range no {
		if c.CanParse(p) {
			t.Errorf("expected CanParse(%q) = false", p)
		}
	}
}

func TestClaudeCodeParse(t *testing.T) {
	c := &ClaudeCode{}
	s, err := c.Parse("../../testdata/claude_code/simple_session.jsonl")
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Session metadata
	if s.Source != "claude-code" {
		t.Errorf("Source = %q, want %q", s.Source, "claude-code")
	}
	if s.SessionID != "test-session-001" {
		t.Errorf("SessionID = %q, want %q", s.SessionID, "test-session-001")
	}
	if s.Project != "/home/dev/myproject" {
		t.Errorf("Project = %q, want %q", s.Project, "/home/dev/myproject")
	}
	if s.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want %q", s.GitBranch, "main")
	}
	if s.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %q, want %q", s.Model, "claude-sonnet-4-20250514")
	}
	if s.ID == "" {
		t.Error("ID should not be empty")
	}
	if s.FileHash == "" {
		t.Error("FileHash should not be empty")
	}
	if s.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}

	// Turn breakdown from the fixture:
	// line 1: user text → 1 turn (user)
	// line 2: assistant thinking (skip) + text + tool_use → 2 turns (assistant, tool_use)
	// line 3: tool_result → 1 turn (tool_result)
	// line 4: assistant text → 1 turn (assistant)
	// line 5: user text → 1 turn (user)
	// Total: 6 turns
	if len(s.Turns) != 6 {
		t.Fatalf("got %d turns, want 6. Turns: %+v", len(s.Turns), s.Turns)
	}

	// Verify the turn sequence tells the right story
	expectations := []struct {
		role     string
		toolName string
		isError  bool
	}{
		{"user", "", false},
		{"assistant", "", false},
		{"tool_use", "Write", false},
		{"tool_result", "", false},
		{"assistant", "", false},
		{"user", "", false},
	}

	for i, exp := range expectations {
		turn := s.Turns[i]
		if turn.Role != exp.role {
			t.Errorf("turn[%d].Role = %q, want %q", i, turn.Role, exp.role)
		}
		if turn.ToolName != exp.toolName {
			t.Errorf("turn[%d].ToolName = %q, want %q", i, turn.ToolName, exp.toolName)
		}
		if turn.IsError != exp.isError {
			t.Errorf("turn[%d].IsError = %v, want %v", i, turn.IsError, exp.isError)
		}
		if turn.Idx != i {
			t.Errorf("turn[%d].Idx = %d, want %d", i, turn.Idx, i)
		}
	}

	// Spot-check content
	if s.Turns[0].Content != "write a hello world function in Go" {
		t.Errorf("first turn content = %q", s.Turns[0].Content)
	}
	if s.Turns[1].Content != "I'll create a hello world function for you." {
		t.Errorf("assistant text content = %q", s.Turns[1].Content)
	}
	if s.Turns[3].Content != "File written successfully" {
		t.Errorf("tool result content = %q", s.Turns[3].Content)
	}
	if s.Turns[5].Content != "perfect, thanks!" {
		t.Errorf("final user content = %q", s.Turns[5].Content)
	}
}

func TestClaudeCodeParseEmptyLines(t *testing.T) {
	// Shouldn't crash on a missing file
	c := &ClaudeCode{}
	_, err := c.Parse("../../testdata/claude_code/nonexistent.jsonl")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
