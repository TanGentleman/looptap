package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeCode parses Claude Code JSONL transcripts.
// Each line is a JSON object with a "type" field — we care about "user" and "assistant",
// everything else is scenery.
type ClaudeCode struct{}

func (c *ClaudeCode) Name() string { return "claude-code" }

// CanParse checks if the file looks like a Claude Code transcript.
// Real path: ~/.claude/projects/<project-hash>/<session-id>.jsonl
// Also matches the older layout with a /sessions/ subdirectory, just in case.
// Skips subagent transcripts — those live in <session>/subagents/ and
// are someone else's problem for now.
func (c *ClaudeCode) CanParse(path string) bool {
	if filepath.Ext(path) != ".jsonl" {
		return false
	}
	normalized := filepath.ToSlash(path)
	if !strings.Contains(normalized, ".claude/projects/") {
		return false
	}
	// Subagent transcripts are interesting but not today's quest
	if strings.Contains(normalized, "/subagents/") {
		return false
	}
	return true
}

// Parse reads a JSONL transcript and returns a Session with all its Turns.
// Malformed lines and unknown types are silently skipped — life's too short
// to crash on someone else's schema changes.
func (c *ClaudeCode) Parse(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	// Hash the file contents while we read — two birds, one io.TeeReader
	hasher := sha256.New()
	reader := io.TeeReader(f, hasher)

	var (
		turns     []Turn
		turnIdx   int
		sessionID string
		cwd       string
		gitBranch string
		model     string
		firstTime time.Time
		lastTime  time.Time
	)

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // some transcripts get chatty

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var jl jsonLine
		if err := json.Unmarshal(line, &jl); err != nil {
			continue // ¯\_(ツ)_/¯
		}

		// Grab metadata from whatever line we're on
		if jl.SessionID != "" && sessionID == "" {
			sessionID = jl.SessionID
		}
		if jl.CWD != "" && cwd == "" {
			cwd = jl.CWD
		}
		if jl.GitBranch != "" {
			gitBranch = jl.GitBranch // take the latest — branches change mid-session
		}

		ts := parseTimestamp(jl.Timestamp)
		if !ts.IsZero() {
			if firstTime.IsZero() || ts.Before(firstTime) {
				firstTime = ts
			}
			if ts.After(lastTime) {
				lastTime = ts
			}
		}

		// We only speak "user" and "assistant" — the rest can wait in the lobby
		if jl.Type != "user" && jl.Type != "assistant" {
			continue
		}

		if jl.Message == nil {
			continue
		}

		var msg jsonMessage
		if err := json.Unmarshal(jl.Message, &msg); err != nil {
			continue
		}

		// Snag the model from the first assistant message that has one
		if model == "" && msg.Model != "" {
			model = msg.Model
		}

		newTurns := extractTurns(msg, ts, turnIdx)
		turns = append(turns, newTurns...)
		turnIdx += len(newTurns)
	}

	if err := scanner.Err(); err != nil {
		return Session{}, fmt.Errorf("scanning %s: %w", path, err)
	}

	// Drain any remaining bytes so the hash covers the full file
	io.Copy(io.Discard, reader)

	fileHash := fmt.Sprintf("%x", hasher.Sum(nil))
	id := computeID("claude-code", sessionID)

	return Session{
		ID:        id,
		Source:    "claude-code",
		Project:   cwd,
		SessionID: sessionID,
		StartedAt: firstTime,
		EndedAt:   lastTime,
		Model:     model,
		GitBranch: gitBranch,
		RawPath:   path,
		FileHash:  fileHash,
		Turns:     turns,
	}, nil
}

// extractTurns pulls Turn structs out of a parsed message.
// One JSONL line can produce multiple turns (e.g., an assistant message
// with both text and tool_use blocks).
func extractTurns(msg jsonMessage, ts time.Time, startIdx int) []Turn {
	idx := startIdx

	// First: is the content a plain string? (simple user messages)
	var contentStr string
	if err := json.Unmarshal(msg.Content, &contentStr); err == nil {
		return []Turn{{
			Idx:     idx,
			Role:    msg.Role,
			Content: contentStr,
			Time:    ts,
		}}
	}

	// Otherwise it's an array of content blocks — the interesting stuff
	var blocks []contentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil
	}

	var turns []Turn

	// For assistant messages, accumulate text blocks into one turn
	// (they're logically one response, just chunked)
	var textParts []string

	for _, block := range blocks {
		switch block.Type {
		case "text":
			if msg.Role == "assistant" {
				textParts = append(textParts, block.Text)
			} else {
				turns = append(turns, Turn{
					Idx:     idx,
					Role:    msg.Role,
					Content: block.Text,
					Time:    ts,
				})
				idx++
			}

		case "tool_use":
			// Flush any accumulated text first
			if len(textParts) > 0 {
				turns = append(turns, Turn{
					Idx:     idx,
					Role:    "assistant",
					Content: strings.Join(textParts, "\n"),
					Time:    ts,
				})
				idx++
				textParts = nil
			}
			// The input as JSON — signal detectors might want to peek inside
			inputJSON, _ := json.Marshal(block.Input)
			turns = append(turns, Turn{
				Idx:      idx,
				Role:     "tool_use",
				Content:  string(inputJSON),
				ToolName: block.Name,
				Time:     ts,
			})
			idx++

		case "tool_result":
			content := extractToolResultContent(block.ResultContent)
			turns = append(turns, Turn{
				Idx:     idx,
				Role:    "tool_result",
				Content: content,
				Time:    ts,
				IsError: block.IsError,
			})
			idx++

		case "thinking":
			// The inner monologue stays inner — not useful for signal detection

		default:
			// New block type? Cool, we'll catch up later
		}
	}

	// Flush any remaining text
	if len(textParts) > 0 {
		turns = append(turns, Turn{
			Idx:     idx,
			Role:    "assistant",
			Content: strings.Join(textParts, "\n"),
			Time:    ts,
		})
	}

	return turns
}

// extractToolResultContent handles the fun part where tool_result content
// can be either a string or an array of content blocks. Thanks, flexibility.
func extractToolResultContent(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}

	// Try string first — the common case
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Array of text blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}

	// Give up gracefully
	return string(raw)
}

func computeID(source, sessionID string) string {
	h := sha256.Sum256([]byte(source + sessionID))
	return fmt.Sprintf("%x", h[:])
}

func parseTimestamp(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Claude Code uses ISO 8601 with milliseconds
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
		time.RFC3339Nano,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// --- JSON unmarshaling types ---
// These mirror the JSONL structure just enough to extract what we need.
// We're not trying to model the full Claude Code schema — just the parts
// that matter for turning transcripts into turns.

type jsonLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	CWD       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	Message   json.RawMessage `json:"message"`
	UUID      string          `json:"uuid"`
}

type jsonMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

// contentBlock is a union type — only some fields are populated depending on Type.
// It's a little wasteful but a lot less code than separate structs per block type.
type contentBlock struct {
	Type          string          `json:"type"`
	Text          string          `json:"text"`
	Name          string          `json:"name"`
	Input         json.RawMessage `json:"input"`
	IsError       bool            `json:"is_error"`
	ResultContent json.RawMessage `json:"content"` // tool_result's nested content field
}
