package signal

import "looptap/internal/parser"

// Disengagement detects user abandonment patterns.
type Disengagement struct{}

func (d *Disengagement) Name() string     { return "disengagement" }
func (d *Disengagement) Category() string { return "interaction" }

func (d *Disengagement) Detect(s parser.Session) []Signal {
	// TODO: check final user turn length, match abandonment phrases
	return nil
}
