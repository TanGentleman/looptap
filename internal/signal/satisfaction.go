package signal

import "looptap/internal/parser"

// Satisfaction detects positive user feedback patterns.
type Satisfaction struct{}

func (sa *Satisfaction) Name() string     { return "satisfaction" }
func (sa *Satisfaction) Category() string { return "interaction" }

func (sa *Satisfaction) Detect(s parser.Session) []Signal {
	// TODO: match gratitude/success phrases in final user turns
	return nil
}
