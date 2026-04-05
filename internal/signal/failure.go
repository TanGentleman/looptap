package signal

import (
	"looptap/internal/parser"
	"strings"
)

// Failure detects tool execution errors and failure patterns.
type Failure struct{}

func (f *Failure) Name() string     { return "failure" }
func (f *Failure) Category() string { return "execution" }

// Error patterns that show up in tool results and assistant messages.
var errorPatterns = []string{
	"command failed",
	"exit code",
	"exit status",
	"stack trace",
	"traceback",
	"panic:",
	"fatal error",
	"segmentation fault",
	"permission denied",
	"no such file",
	"compilation failed",
	"build failed",
	"syntax error",
	"undefined reference",
	"cannot find module",
}

func (f *Failure) Detect(s parser.Session) []Signal {
	var signals []Signal

	for _, t := range s.Turns {
		// Explicit error flag on tool results — high confidence
		if t.Role == "tool_result" && t.IsError {
			idx := t.Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "failure",
				Category:   "execution",
				TurnIdx:    &idx,
				Confidence: 0.9,
				Evidence:   "tool_result with is_error=true",
			})
			continue
		}

		// Pattern matching in tool results and assistant text
		if t.Role != "tool_result" && t.Role != "assistant" {
			continue
		}
		lower := strings.ToLower(t.Content)
		for _, pat := range errorPatterns {
			if strings.Contains(lower, pat) {
				idx := t.Idx
				signals = append(signals, Signal{
					SessionID:  s.ID,
					Type:       "failure",
					Category:   "execution",
					TurnIdx:    &idx,
					Confidence: 0.6,
					Evidence:   "pattern: " + pat,
				})
				break // one signal per turn
			}
		}
	}

	return signals
}
