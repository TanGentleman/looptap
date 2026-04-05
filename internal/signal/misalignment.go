package signal

import "looptap/internal/parser"

// Misalignment detects user correction and rephrasing patterns.
type Misalignment struct{}

func (m *Misalignment) Name() string     { return "misalignment" }
func (m *Misalignment) Category() string { return "interaction" }

var misalignmentPhrases = loadPhrases("misalignment.txt")

func (m *Misalignment) Detect(s parser.Session) []Signal {
	var signals []Signal

	userTurns := filterRole(s.Turns, "user")

	for _, t := range userTurns {
		// Correction phrases from the phrase list
		matched, phrase := MatchPhrases(t.Content, misalignmentPhrases, 1)
		if matched {
			idx := t.Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "misalignment",
				Category:   "interaction",
				TurnIdx:    &idx,
				Confidence: 0.8,
				Evidence:   phrase,
			})
		}
	}

	// Consecutive user turns with high similarity = rephrasing the same request
	for i := 1; i < len(userTurns); i++ {
		sim := TokenSimilarity(userTurns[i-1].Content, userTurns[i].Content)
		if sim > 0.7 {
			idx := userTurns[i].Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "misalignment",
				Category:   "interaction",
				TurnIdx:    &idx,
				Confidence: sim,
				Evidence:   "user rephrased previous turn",
			})
		}
	}

	return signals
}
