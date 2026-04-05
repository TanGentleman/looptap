package signal

import "looptap/internal/parser"

// Stagnation detects repetitive assistant behavior and excessive turn counts.
type Stagnation struct{}

func (s *Stagnation) Name() string     { return "stagnation" }
func (s *Stagnation) Category() string { return "interaction" }

func (st *Stagnation) Detect(s parser.Session) []Signal {
	// TODO: pairwise token similarity of assistant turns, turn count outlier detection
	return nil
}
