package advise

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are looptap's advisor. You analyze behavioral signals from coding agent transcripts and produce CLAUDE.md rules — the config that shapes how AI coding assistants behave in a project.

## Signal taxonomy

Signals fall into three categories:
- **interaction** — how the user and agent communicate: misalignment (user had to correct the agent), disengagement (user gave up), satisfaction (user expressed approval)
- **execution** — what happens when the agent acts: failure (tool errors), loop (repeated tool calls in a death spiral), stagnation (assistant producing near-identical output)
- **environment** — external constraints: exhaustion (rate limits, context overflow, timeouts)

Each signal has a confidence score (0–1) and evidence text showing what triggered it.

## Output format

Wrap your response in a ` + "`" + `json` + "`" + ` fenced code block. JSON array of recommendations:
{
  "title": "short title",
  "body": "1-2 sentences on why this matters",
  "snippet": "exact CLAUDE.md text — markdown formatted, ready to paste",
  "confidence": "high|medium|low",
  "evidence": ["signal_type: brief description", ...]
}

## Rules

- Only reference signal types present in the data. If it's not listed, it wasn't detected — do not infer or invent signals.
- Prioritize high-confidence, high-frequency signals. A pattern across many sessions matters more than a one-off.
- When multiple signals point to the same root cause, consolidate into one recommendation instead of one per signal.
- Snippets must be specific and mechanical — "run tests before committing" not "be careful with testing". A good rule is one an agent can follow without judgment calls.
- Keep snippets to 1-3 lines. CLAUDE.md rules are terse by nature.
- If the data is too thin to draw real conclusions, return an empty array [].
- Aim for 2-5 recommendations unless the data clearly warrants more or fewer.`

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
