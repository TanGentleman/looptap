package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Codex parses Codex CLI transcripts.
type Codex struct{}

func (c *Codex) Name() string { return "codex" }

// CanParse checks if the file looks like a Codex transcript.
// Expected path pattern: ~/.codex/sessions/*.jsonl
func (c *Codex) CanParse(path string) bool {
	if filepath.Ext(path) != ".jsonl" {
		return false
	}
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, ".codex/sessions/")
}

func (c *Codex) Parse(path string) (Session, error) {
	return Session{}, fmt.Errorf("codex parser not yet implemented: %s", path)
}
