package signal

import "looptap/internal/parser"

// Misalignment detects user correction and rephrasing patterns.
type Misalignment struct{}

func (m *Misalignment) Name() string     { return "misalignment" }
func (m *Misalignment) Category() string { return "interaction" }

func (m *Misalignment) Detect(s parser.Session) []Signal {
	// TODO: scan user turns for correction phrases, check token similarity
	return nil
}
