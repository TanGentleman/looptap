package analyze

import (
	"strings"
	"testing"
)

func TestBuildUserPrompt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string // substrings that must appear
	}{
		{
			"wraps content in markdown fence",
			"# rules\nbe nice\n",
			[]string{"## CLAUDE.md Contents", "```markdown\n# rules\nbe nice\n```"},
		},
		{
			"adds trailing newline when missing",
			"no newline",
			[]string{"```markdown\nno newline\n```"},
		},
		{
			"empty body still produces a fenced block",
			"",
			[]string{"```markdown\n\n```"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildUserPrompt(tt.content)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("prompt missing %q\n--- got ---\n%s", want, got)
				}
			}
		})
	}
}

func TestParseFindings(t *testing.T) {
	want := []Finding{{
		Title:    "vague rule",
		Body:     "rule X is fuzzy",
		Severity: "medium",
		Category: "clarity",
	}}

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{
			"fenced json block",
			"sure thing:\n```json\n[{\"title\":\"vague rule\",\"body\":\"rule X is fuzzy\",\"severity\":\"medium\",\"category\":\"clarity\",\"suggestion\":\"\",\"evidence\":null}]\n```\n",
			false,
		},
		{
			"bare json fallback",
			"[{\"title\":\"vague rule\",\"body\":\"rule X is fuzzy\",\"severity\":\"medium\",\"category\":\"clarity\",\"suggestion\":\"\",\"evidence\":null}]",
			false,
		},
		{
			"unterminated fence still parses",
			"```json\n[{\"title\":\"vague rule\",\"body\":\"rule X is fuzzy\",\"severity\":\"medium\",\"category\":\"clarity\",\"suggestion\":\"\",\"evidence\":null}]",
			false,
		},
		{
			"garbage in, error out",
			"sorry I cannot help with that",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFindings(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != 1 || got[0].Title != want[0].Title || got[0].Severity != want[0].Severity {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}
