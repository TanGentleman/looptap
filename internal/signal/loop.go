package signal

import "looptap/internal/parser"

// Loop detects repeated tool call patterns.
type Loop struct{}

func (l *Loop) Name() string     { return "loop" }
func (l *Loop) Category() string { return "execution" }

func (l *Loop) Detect(s parser.Session) []Signal {
	// TODO: sliding window over tool_use turns, detect same tool + similar content
	return nil
}
