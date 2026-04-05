package advise

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are looptap's advisor. You analyze behavioral signals detected in coding agent transcripts and produce CLAUDE.md additions — the config files that shape how AI coding assistants behave in a project.

You will receive signal data from a SQLite database: signal types (misalignment, stagnation, disengagement, satisfaction, failure, loop, exhaustion), confidence scores (0-1), and evidence text.

Your job: turn patterns into concrete CLAUDE.md rules that would prevent the detected problems.

Output a JSON array of recommendations. Each element:
{
  "title": "short title",
  "body": "1-2 sentences explaining why this matters",
  "snippet": "exact text to add to CLAUDE.md — markdown formatted, ready to paste",
  "confidence": "high|medium|low",
  "evidence": ["signal_type: brief description", ...]
}

Rules:
- Only reference signals that were provided. Do not hallucinate data.
- Snippets should be specific and actionable, not generic advice.
- If the data is too sparse to draw conclusions, return an empty array [].
- Keep snippets concise — a CLAUDE.md rule should be 1-3 lines, not a paragraph.
- Aim for 2-5 recommendations unless the data warrants more or fewer.`

// BuildUserPrompt assembles the gathered signal context into a structured prompt.
func BuildUserPrompt(ctx *SignalContext) string {
	var b strings.Builder

	if ctx.ProjectFilter != "" {
		fmt.Fprintf(&b, "Project: %s\n", ctx.ProjectFilter)
	}
	fmt.Fprintf(&b, "Total sessions analyzed: %d\n\n", ctx.SessionCount)

	// Signal summary
	b.WriteString("## Signal Summary\n")
	if len(ctx.Summary) == 0 {
		b.WriteString("No signals detected.\n")
	}
	for _, r := range ctx.Summary {
		fmt.Fprintf(&b, "- %s: %d occurrences (avg confidence %.2f)\n", r.Type, r.Count, r.AvgConfidence)
	}
	b.WriteString("\n")

	// Failure details
	if len(ctx.Failures) > 0 {
		b.WriteString("## Failure Signals (top by confidence)\n")
		for _, r := range ctx.Failures {
			fmt.Fprintf(&b, "- session=%s turn=%d conf=%.2f evidence=%q",
				shortID(r.SessionID), r.TurnIdx, r.Confidence, r.Evidence)
			if r.ContentPreview != "" {
				fmt.Fprintf(&b, " content=%q", truncate(r.ContentPreview, 150))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Loop details
	if len(ctx.Loops) > 0 {
		b.WriteString("## Loop Signals (tool call death spirals)\n")
		for _, r := range ctx.Loops {
			fmt.Fprintf(&b, "- session=%s turn=%d conf=%.2f tool=%s evidence=%q",
				shortID(r.SessionID), r.TurnIdx, r.Confidence, r.ToolName, r.Evidence)
			if r.ContentPreview != "" {
				fmt.Fprintf(&b, " content=%q", truncate(r.ContentPreview, 150))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Misalignment details
	if len(ctx.Misalignments) > 0 {
		b.WriteString("## Misalignment Signals (user had to correct the agent)\n")
		for _, r := range ctx.Misalignments {
			fmt.Fprintf(&b, "- session=%s turn=%d conf=%.2f evidence=%q",
				shortID(r.SessionID), r.TurnIdx, r.Confidence, r.Evidence)
			if r.ContentPreview != "" {
				fmt.Fprintf(&b, " content=%q", truncate(r.ContentPreview, 150))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// shortID returns the first 12 chars of a session ID for readability.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
