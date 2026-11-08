package main

import (
	"bytes"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRootCommands(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"parse", true},
		{"signal", true},
		{"run", true},
		{"info", true},
		{"query", true},
		{"patterns", true},
		{"seed-contract-fixture", true},
		{"version", true},
		{"advise", false},
		{"analyze", false},
	}
	root := newRootCmd()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, err := root.Find([]string{tt.name})
			got := err == nil && cmd != root && cmd.Name() == tt.name
			if got != tt.want {
				t.Errorf("command %q registered = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func execRoot(t *testing.T, args ...string) string {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("looptap %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// The README's fixture example: run on the bundled transcripts, then query.
// Running twice must not duplicate sessions or signals.
func TestRunAndQueryFixtures(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	dbPath := filepath.Join(t.TempDir(), "demo.db")
	codex := "testdata/codex/rollout-2026-11-01T10-00-00-0199a000-0000-7000-8000-000000000001.jsonl"

	for range 2 {
		execRoot(t, "run", "testdata/claude_code", "testdata/codex", "--db", dbPath)
	}

	tests := []struct {
		signal string
		want   []string
	}{
		{"failure", []string{codex}},
		{"misalignment", []string{codex}},
		{"satisfaction", []string{"testdata/claude_code/meta_session.jsonl", "testdata/claude_code/simple_session.jsonl"}},
		{"loop", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.signal, func(t *testing.T) {
			got := strings.Fields(execRoot(t, "query", "--db", dbPath, "--signal", tt.signal, "--format", "paths"))
			sort.Strings(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("query --signal %s = %q, want %q", tt.signal, got, tt.want)
			}
		})
	}

	rows := strings.Split(strings.TrimSpace(execRoot(t, "query", "--db", dbPath, "--format", "tsv")), "\n")
	counts := map[string]string{}
	for _, row := range rows {
		f := strings.Split(row, "\t")
		if len(f) != 4 {
			t.Fatalf("tsv row %q: want 4 fields", row)
		}
		counts[f[1]] = f[2]
	}
	want := map[string]string{
		codex: "2",
		"testdata/claude_code/meta_session.jsonl":   "1",
		"testdata/claude_code/simple_session.jsonl": "1",
	}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("signal counts per session = %v, want %v", counts, want)
	}
}
