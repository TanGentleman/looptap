package signal

import "looptap/internal/parser"

// Failure detects tool execution errors and failure patterns.
type Failure struct{}

func (f *Failure) Name() string     { return "failure" }
func (f *Failure) Category() string { return "execution" }

func (f *Failure) Detect(s parser.Session) []Signal {
	// TODO: scan tool_result turns for errors, match error patterns
	return nil
}
