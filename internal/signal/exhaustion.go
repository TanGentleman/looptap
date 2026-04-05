package signal

import (
	"looptap/internal/parser"
	"strings"
)

// Exhaustion detects rate limits, context length, and timeout patterns.
type Exhaustion struct{}

func (e *Exhaustion) Name() string     { return "exhaustion" }
func (e *Exhaustion) Category() string { return "environment" }

var exhaustionPhrases = loadPhrases("exhaustion.txt")

func (e *Exhaustion) Detect(s parser.Session) []Signal {
	var signals []Signal

	for _, t := range s.Turns {
		if t.Role != "tool_result" && t.Role != "system" && t.Role != "assistant" {
			continue
		}

		lower := strings.ToLower(t.Content)
		for _, phrase := range exhaustionPhrases {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				idx := t.Idx
				signals = append(signals, Signal{
					SessionID:  s.ID,
					Type:       "exhaustion",
					Category:   "environment",
					TurnIdx:    &idx,
					Confidence: 0.7,
					Evidence:   phrase,
				})
				break
			}
		}
	}

	return signals
}
