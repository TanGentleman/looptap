package analyze

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()

	good := filepath.Join(dir, "good.md")
	if err := os.WriteFile(good, []byte("# hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.md")

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr string // substring; "" means no error
	}{
		{"reads file", good, "# hello\nworld\n", ""},
		{"missing file", missing, "", "nothing to analyze"},
		{"empty file", empty, "", "is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFile(tt.path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.want {
					t.Errorf("got %q, want %q", got, tt.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDefaultClaudeMDPath(t *testing.T) {
	got, err := DefaultClaudeMDPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join(".claude", "CLAUDE.md")) {
		t.Errorf("path %q missing .claude/CLAUDE.md suffix", got)
	}
}
