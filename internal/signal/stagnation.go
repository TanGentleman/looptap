package signal

import "looptap/internal/parser"

// Stagnation detects repetitive assistant behavior — saying the same thing in different fonts.
type Stagnation struct{}

func (st *Stagnation) Name() string     { return "stagnation" }
func (st *Stagnation) Category() string { return "interaction" }

const stagnationSimThresh = 0.8

func (st *Stagnation) Detect(s parser.Session) []Signal {
	var signals []Signal

	assistantTurns := filterRole(s.Turns, "assistant")

	// Pairwise consecutive similarity
	for i := 1; i < len(assistantTurns); i++ {
		sim := TokenSimilarity(assistantTurns[i-1].Content, assistantTurns[i].Content)
		if sim > stagnationSimThresh {
			idx := assistantTurns[i].Idx
			signals = append(signals, Signal{
				SessionID:  s.ID,
				Type:       "stagnation",
				Category:   "interaction",
				TurnIdx:    &idx,
				Confidence: sim,
				Evidence:   "assistant repeating itself",
			})
		}
	}

	return signals
}
