package parser

import "time"

// Session represents a parsed agent transcript session.
type Session struct {
	ID        string // deterministic: sha256(Source + SessionID)
	Source    string // "claude-code", "codex"
	Project   string
	SessionID string // original ID from agent
	StartedAt time.Time
	EndedAt   time.Time
	Model     string
	GitBranch string
	RawPath   string
	FileHash  string // SHA-256 of file contents

	// Attribution applied at ingest, not read from the transcript. Parsers
	// leave these empty; the `parse`/`run` commands stamp them from flags or
	// config so a fleet-wide database knows who ran each session.
	User string
	Team string

	Turns []Turn
}

// Turn represents a single turn in a session transcript.
type Turn struct {
	Idx      int
	Role     string // "user", "assistant", "tool_use", "tool_result", "system"
	Content  string
	Time     time.Time
	ToolName string // non-empty for tool_use and tool_result
	IsError  bool
}
