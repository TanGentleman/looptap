package parser

import (
	"os"
	"path/filepath"
	"testing"
)

const codexFixture = "../../testdata/codex/rollout-2026-11-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl"

func TestCodexCanParse(t *testing.T) {
	dir := t.TempDir()
	notRollout := filepath.Join(dir, "other.jsonl")
	if err := os.WriteFile(notRollout, []byte(`{"type":"user","sessionId":"s1"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/home/u/.codex/sessions/2026/11/01/rollout-x.jsonl", true},
		{"/home/u/.codex/archived_sessions/rollout-x.jsonl", true},
		{"/home/u/.codex/sessions/2026/11/01/rollout-x.jsonl.zst", false},
		{"/home/u/.claude/projects/-hash/abc.jsonl", false},
		{codexFixture, true}, // sniffed
		{notRollout, false},
	}
	for _, tt := range tests {
		if got := (&Codex{}).CanParse(tt.path); got != tt.want {
			t.Errorf("CanParse(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
	if (&ClaudeCode{}).CanParse(codexFixture) {
		t.Error("ClaudeCode claims the Codex fixture")
	}
}

func TestCodexParse(t *testing.T) {
	s, err := (&Codex{}).Parse(codexFixture)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	meta := []struct{ name, got, want string }{
		{"Source", s.Source, "codex"},
		{"SessionID", s.SessionID, "0199a000-0000-7000-8000-000000000001"},
		{"Project", s.Project, "/home/dev/api"},
		{"GitBranch", s.GitBranch, "fix-tests"},
		{"Model", s.Model, "gpt-5.5"},
		{"ID", s.ID, computeID("codex", "0199a000-0000-7000-8000-000000000001")},
		{"StartedAt", s.StartedAt.Format("15:04:05"), "10:00:00"},
		{"EndedAt", s.EndedAt.Format("15:04:05"), "10:01:05"},
	}
	for _, m := range meta {
		if m.got != m.want {
			t.Errorf("%s = %q, want %q", m.name, m.got, m.want)
		}
	}
	if len(s.FileHash) != 64 {
		t.Errorf("FileHash = %q", s.FileHash)
	}

	want := []struct {
		role, tool string
		isError    bool
	}{
		{"user", "", false},
		{"tool_use", "exec_command", false},
		{"tool_result", "exec_command", true},
		{"tool_use", "apply_patch", false},
		{"tool_result", "apply_patch", false},
		{"tool_use", "exec_command", false},
		{"tool_result", "exec_command", false},
		{"assistant", "", false},
		{"user", "", false},
		{"tool_use", "local_shell", false},
		{"tool_result", "local_shell", false},
		{"assistant", "", false},
	}
	if len(s.Turns) != len(want) {
		for _, tr := range s.Turns {
			t.Logf("%d %s %s %q", tr.Idx, tr.Role, tr.ToolName, tr.Content)
		}
		t.Fatalf("got %d turns, want %d", len(s.Turns), len(want))
	}
	for i, w := range want {
		tr := s.Turns[i]
		if tr.Idx != i || tr.Role != w.role || tr.ToolName != w.tool || tr.IsError != w.isError {
			t.Errorf("turn %d = {%d %s %s err=%v}, want {%s %s err=%v}", i, tr.Idx, tr.Role, tr.ToolName, tr.IsError, w.role, w.tool, w.isError)
		}
	}
	if got := s.Turns[0].Content; got != "fix the failing test in store_test.go" {
		t.Errorf("first user turn = %q", got)
	}
	if got := s.Turns[10].Content; got != "Exit code: 0\n" {
		t.Errorf("legacy output = %q", got)
	}
}

func TestCodexOutput(t *testing.T) {
	tests := []struct {
		name, raw, want string
		failed          bool
	}{
		{"exec", `"Exit code: 2\nWall time: 0.1 seconds\nOutput:\nboom"`, "Exit code: 2\nWall time: 0.1 seconds\nOutput:\nboom", true},
		{"unified exec", `"Wall time: 1 seconds\nProcess exited with code 0\nOutput:\nok"`, "Wall time: 1 seconds\nProcess exited with code 0\nOutput:\nok", false},
		{"legacy json", `"{\"output\":\"nope\\n\",\"metadata\":{\"exit_code\":127}}"`, "Exit code: 127\nnope\n", true},
		{"content items", `[{"type":"input_text","text":"a"},{"type":"input_image","image_url":"x"},{"type":"input_text","text":"b"}]`, "a\nb", false},
		{"no exit code", `"Success. Updated the following files:\nM a.go"`, "Success. Updated the following files:\nM a.go", false},
		{"exit code mid-text", `"grep found: Exit code: 1 in log"`, "grep found: Exit code: 1 in log", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codexOutput([]byte(tt.raw))
			if got != tt.want {
				t.Errorf("codexOutput = %q, want %q", got, tt.want)
			}
			if f := codexFailed(got); f != tt.failed {
				t.Errorf("codexFailed = %v, want %v", f, tt.failed)
			}
		})
	}
}

func TestCodexParseNoMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"event_msg","payload":{"type":"user_message","message":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Codex{}).Parse(path); err == nil {
		t.Error("expected error for rollout without session_meta")
	}
}
