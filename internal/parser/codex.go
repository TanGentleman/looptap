package parser

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Codex parses Codex CLI rollout files: one JSON object per line, each
// {"timestamp", "type", "payload"}. Types we read: session_meta,
// turn_context, event_msg (user_message) and response_item.
type Codex struct{}

func (c *Codex) Name() string { return "codex" }

// CanParse checks if the file looks like a Codex rollout.
// Real paths: ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl and
// ~/.codex/archived_sessions/; anything else gets sniffed for session_meta.
// Compressed .jsonl.zst rollouts aren't read yet.
func (c *Codex) CanParse(path string) bool {
	if filepath.Ext(path) != ".jsonl" {
		return false
	}
	normalized := filepath.ToSlash(path)
	if strings.Contains(normalized, ".codex/sessions/") || strings.Contains(normalized, ".codex/archived_sessions/") {
		return true
	}
	return sniffCodex(path)
}

func sniffCodex(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	if !scanner.Scan() {
		return false
	}
	var line codexLine
	return json.Unmarshal(scanner.Bytes(), &line) == nil && line.Type == "session_meta"
}

// Parse maps a rollout onto Turns:
//   - event_msg user_message -> user (what was typed; the matching
//     response_item also carries AGENTS.md and environment context)
//   - response_item message (assistant) -> assistant
//   - function_call, custom_tool_call, local_shell_call -> tool_use
//   - function_call_output, custom_tool_call_output -> tool_result,
//     IsError when the output reports a non-zero exit code
//
// Reasoning, developer messages and other events are skipped.
func (c *Codex) Parse(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	hasher := sha256.New()
	reader := io.TeeReader(f, hasher)

	var (
		turns     []Turn
		sessionID string
		cwd       string
		gitBranch string
		model     string
		firstTime time.Time
		lastTime  time.Time
		toolNames = map[string]string{} // call_id -> tool name
	)
	add := func(t Turn) {
		t.Idx = len(turns)
		turns = append(turns, t)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var line codexLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}

		ts := parseTimestamp(line.Timestamp)
		if !ts.IsZero() {
			if firstTime.IsZero() || ts.Before(firstTime) {
				firstTime = ts
			}
			if ts.After(lastTime) {
				lastTime = ts
			}
		}

		var p codexPayload
		if err := json.Unmarshal(line.Payload, &p); err != nil {
			continue
		}

		switch line.Type {
		case "session_meta":
			// Forked rollouts replay the parent's meta later on; first one wins.
			if sessionID == "" {
				sessionID = p.ID
				cwd = p.CWD
				if p.Git != nil {
					gitBranch = p.Git.Branch
				}
			}
		case "turn_context":
			if p.Model != "" {
				model = p.Model
			}
			if cwd == "" {
				cwd = p.CWD
			}
		case "event_msg":
			if p.Type == "user_message" && p.Message != "" {
				add(Turn{Role: "user", Content: p.Message, Time: ts})
			}
		case "response_item":
			switch p.Type {
			case "message":
				if p.Role == "assistant" {
					if text := codexText(p.Content); text != "" {
						add(Turn{Role: "assistant", Content: text, Time: ts})
					}
				}
			case "function_call":
				toolNames[p.CallID] = p.Name
				add(Turn{Role: "tool_use", ToolName: p.Name, Content: p.Arguments, Time: ts})
			case "custom_tool_call":
				toolNames[p.CallID] = p.Name
				add(Turn{Role: "tool_use", ToolName: p.Name, Content: p.Input, Time: ts})
			case "local_shell_call":
				toolNames[p.CallID] = "local_shell"
				add(Turn{Role: "tool_use", ToolName: "local_shell", Content: string(p.Action), Time: ts})
			case "function_call_output", "custom_tool_call_output":
				out := codexOutput(p.Output)
				add(Turn{
					Role:     "tool_result",
					ToolName: toolNames[p.CallID],
					Content:  out,
					Time:     ts,
					IsError:  codexFailed(out),
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return Session{}, fmt.Errorf("scanning %s: %w", path, err)
	}
	io.Copy(io.Discard, reader)

	if sessionID == "" {
		return Session{}, fmt.Errorf("no session_meta in %s", path)
	}

	return Session{
		ID:        computeID("codex", sessionID),
		Source:    "codex",
		Project:   cwd,
		SessionID: sessionID,
		StartedAt: firstTime,
		EndedAt:   lastTime,
		Model:     model,
		GitBranch: gitBranch,
		RawPath:   path,
		FileHash:  fmt.Sprintf("%x", hasher.Sum(nil)),
		Turns:     turns,
	}, nil
}

// codexText joins the text parts of a message's content items
// (output_text for the assistant, input_text for users).
func codexText(raw json.RawMessage) string {
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	var parts []string
	for _, it := range items {
		if strings.HasSuffix(it.Type, "text") && it.Text != "" {
			parts = append(parts, it.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// codexOutput flattens a tool output: a plain string, a list of content
// items, or the older {"output": ..., "metadata": {"exit_code": N}} JSON
// string, which is rewritten to the current "Exit code: N" header.
func codexOutput(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return codexText(raw)
	}
	var legacy struct {
		Output   *string `json:"output"`
		Metadata struct {
			ExitCode *int `json:"exit_code"`
		} `json:"metadata"`
	}
	if strings.HasPrefix(s, "{") && json.Unmarshal([]byte(s), &legacy) == nil && legacy.Output != nil && legacy.Metadata.ExitCode != nil {
		return fmt.Sprintf("Exit code: %d\n%s", *legacy.Metadata.ExitCode, *legacy.Output)
	}
	return s
}

// Shell tools put the exit code in a header line: "Exit code: N" (exec)
// or "Process exited with code N" (unified exec).
var codexExitCode = regexp.MustCompile(`(?m)^(?:Exit code: |Process exited with code )(-?\d+)\s*$`)

func codexFailed(out string) bool {
	m := codexExitCode.FindStringSubmatch(out)
	if m == nil {
		return false
	}
	code, err := strconv.Atoi(m[1])
	return err == nil && code != 0
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// codexPayload is the union of the payload fields we read.
type codexPayload struct {
	Type string `json:"type"`

	// session_meta
	ID  string `json:"id"`
	CWD string `json:"cwd"`
	Git *struct {
		Branch string `json:"branch"`
	} `json:"git"`

	// turn_context
	Model string `json:"model"`

	// event_msg user_message
	Message string `json:"message"`

	// response_item
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	CallID    string          `json:"call_id"`
	Action    json.RawMessage `json:"action"`
	Output    json.RawMessage `json:"output"`
}
