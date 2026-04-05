package signal

import "looptap/internal/parser"

// Satisfaction detects positive user feedback patterns.
type Satisfaction struct{}

func (sa *Satisfaction) Name() string     { return "satisfaction" }
func (sa *Satisfaction) Category() string { return "interaction" }

var satisfactionPhrases = loadPhrases("satisfaction.txt")

func (sa *Satisfaction) Detect(s parser.Session) []Signal {
	var signals []Signal

	// Look at the last 3 user turns for gratitude/success phrases
	userTurns := filterRole(s.Turns, "user")
	start := len(userTurns) - 3
	if start < 0 {
		start = 0
	}

	for _, t := range userTurns[start:] {
		matched, phrase := MatchPhrases(t.Content, satisfactionPhrases, 1)
		if matched {
			idx := t.Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "satisfaction",
				Category:   "interaction",
				TurnIdx:    &idx,
				Confidence: 0.8,
				Evidence:   phrase,
			})
		}
	}

	return signals
}

func filterRole(turns []parser.Turn, role string) []parser.Turn {
	var out []parser.Turn
	for _, t := range turns {
		if t.Role == role {
			out = append(out, t)
		}
	}
	return out
}
