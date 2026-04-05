package signal

import (
	"looptap/internal/parser"
	"strings"
)

// Disengagement detects user abandonment patterns.
type Disengagement struct{}

func (d *Disengagement) Name() string     { return "disengagement" }
func (d *Disengagement) Category() string { return "interaction" }

var disengagementPhrases = loadPhrases("disengagement.txt")

func (d *Disengagement) Detect(s parser.Session) []Signal {
	var signals []Signal

	userTurns := filterRole(s.Turns, "user")
	if len(userTurns) == 0 {
		return nil
	}

	last := userTurns[len(userTurns)-1]

	// Abandonment phrases anywhere in user turns
	for _, t := range userTurns {
		matched, phrase := MatchPhrases(t.Content, disengagementPhrases, 1)
		if matched {
			idx := t.Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "disengagement",
				Category:   "interaction",
				TurnIdx:    &idx,
				Confidence: 0.8,
				Evidence:   phrase,
			})
		}
	}

	// Short final user turn that isn't satisfaction — smells like giving up
	words := strings.Fields(last.Content)
	if len(words) <= 5 && len(words) > 0 {
		isSatisfied, _ := MatchPhrases(last.Content, satisfactionPhrases, 1)
		if !isSatisfied {
			idx := last.Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "disengagement",
				Category:   "interaction",
				TurnIdx:    &idx,
				Confidence: 0.5,
				Evidence:   "short final turn (not satisfaction)",
			})
		}
	}

	return signals
}
