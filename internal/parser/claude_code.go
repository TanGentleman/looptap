package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ClaudeCode parses Claude Code JSONL transcripts.
type ClaudeCode struct{}

func (c *ClaudeCode) Name() string { return "claude-code" }

// CanParse checks if the file looks like a Claude Code transcript.
// Expected path pattern: ~/.claude/projects/<hash>/sessions/<id>.jsonl
func (c *ClaudeCode) CanParse(path string) bool {
	if filepath.Ext(path) != ".jsonl" {
		return false
	}
	// Normalize path separators and check for the expected directory structure.
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, ".claude/projects/") &&
		strings.Contains(normalized, "/sessions/")
}

func (c *ClaudeCode) Parse(path string) (Session, error) {
	return Session{}, fmt.Errorf("claude code parser not yet implemented: %s", path)
}
