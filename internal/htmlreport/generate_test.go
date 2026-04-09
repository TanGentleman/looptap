package htmlreport

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	r := &Resolved{
		RepoPath:   "/path/to/repo",
		Branch:     "feature/x",
		BranchMode: BranchCustom,
	}
	html, err := Generate(r)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wants := []string{
		"<!doctype html>",
		"/path/to/repo",
		"feature/x",
		"custom",
		"looptap",
		"Branch report",
	}
	for _, w := range wants {
		if !strings.Contains(html, w) {
			t.Errorf("output missing %q", w)
		}
	}
}

func TestGenerate_NilSettings(t *testing.T) {
	if _, err := Generate(nil); err == nil {
		t.Error("expected error for nil settings")
	}
}

func TestGenerate_EscapesBranchName(t *testing.T) {
	// A malicious branch name shouldn't escape the template and inject raw HTML.
	r := &Resolved{
		RepoPath:   "/tmp/r",
		Branch:     "<script>alert(1)</script>",
		BranchMode: BranchCustom,
	}
	html, err := Generate(r)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("branch name not HTML-escaped")
	}
}
