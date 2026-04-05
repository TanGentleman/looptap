package signal

import (
	"looptap/internal/parser"
)

// Detector is the interface that signal detectors must implement.
type Detector interface {
	Name() string
	Category() string // "interaction", "execution", "environment"
	Detect(s parser.Session) []Signal
}

// All registered detectors.
var All = []Detector{
	&Misalignment{},
	&Stagnation{},
	&Disengagement{},
	&Satisfaction{},
	&Failure{},
	&Loop{},
	&Exhaustion{},
}

// RunAll runs every registered detector against a session.
func RunAll(s parser.Session) []Signal {
	var signals []Signal
	for _, d := range All {
		signals = append(signals, d.Detect(s)...)
	}
	return signals
}
