package signal

import (
	"looptap/internal/parser"
	"strconv"
)

// Loop detects repeated tool call patterns — same tool, same args, same hope.
type Loop struct{}

func (l *Loop) Name() string     { return "loop" }
func (l *Loop) Category() string { return "execution" }

const (
	loopWindow     = 6
	loopMinRepeats = 3
	loopSimThresh  = 0.8
)

func (l *Loop) Detect(s parser.Session) []Signal {
	var signals []Signal

	toolTurns := filterRole(s.Turns, "tool_use")
	if len(toolTurns) < loopMinRepeats {
		return nil
	}

	// Sliding window: flag when ≥loopMinRepeats turns in a window share
	// the same ToolName with content similarity > threshold
	for i := 0; i+loopWindow <= len(toolTurns); i++ {
		window := toolTurns[i : i+loopWindow]
		signals = append(signals, detectLoopInWindow(s.ID, window)...)
	}

	// Handle tail if fewer than loopWindow tool turns total
	if len(toolTurns) < loopWindow {
		signals = append(signals, detectLoopInWindow(s.ID, toolTurns)...)
	}

	return dedup(signals)
}

func detectLoopInWindow(sessionID string, window []parser.Turn) []Signal {
	// Group by tool name
	byTool := map[string][]parser.Turn{}
	for _, t := range window {
		byTool[t.ToolName] = append(byTool[t.ToolName], t)
	}

	var signals []Signal
	for toolName, turns := range byTool {
		if len(turns) < loopMinRepeats {
			continue
		}

		// Check pairwise similarity of content
		similarCount := 0
		for i := 1; i < len(turns); i++ {
			if TokenSimilarity(turns[0].Content, turns[i].Content) > loopSimThresh {
				similarCount++
			}
		}

		if similarCount >= loopMinRepeats-1 {
			idx := turns[len(turns)-1].Idx
			confidence := float64(similarCount+1) / float64(len(window))
			if confidence > 1.0 {
				confidence = 1.0
			}
			signals = append(signals, Signal{
				SessionID:  sessionID,
				Type:       "loop",
				Category:   "execution",
				TurnIdx:    &idx,
				Confidence: confidence,
				Evidence:   toolName + " called " + strconv.Itoa(similarCount+1) + " times with similar args",
			})
		}
	}
	return signals
}

// dedup removes signals pointing at the same turn index.
func dedup(signals []Signal) []Signal {
	seen := map[int]bool{}
	var out []Signal
	for _, s := range signals {
		if s.TurnIdx == nil {
			out = append(out, s)
			continue
		}
		if !seen[*s.TurnIdx] {
			seen[*s.TurnIdx] = true
			out = append(out, s)
		}
	}
	return out
}

