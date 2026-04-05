package signal

import "looptap/internal/parser"

// Exhaustion detects rate limits, context length, and timeout patterns.
type Exhaustion struct{}

func (e *Exhaustion) Name() string     { return "exhaustion" }
func (e *Exhaustion) Category() string { return "environment" }

func (e *Exhaustion) Detect(s parser.Session) []Signal {
	// TODO: match rate-limit, context-length, and timeout patterns
	return nil
}
