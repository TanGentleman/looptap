package analyze

import (
	"fmt"
	"strings"

	"looptap/internal/advise"
)

const systemPrompt = `You are looptap's CLAUDE.md quality reviewer. You analyze CLAUDE.md files — the configuration files that shape how AI coding assistants behave in a project — and identify problems, gaps, and improvements.

Evaluate the file across these dimensions:
- **Clarity**: Are rules specific and unambiguous? Would an AI agent know exactly what to do?
- **Completeness**: Are there obvious gaps? Missing error handling guidance, testing expectations, style rules?
- **Consistency**: Do any rules contradict each other? Are there duplicates saying the same thing differently?
- **Structure**: Is it well-organized? Could it be grouped better? Are there walls of text that should be broken up?
- **Actionability**: Are rules concrete enough to follow, or are they vague aspirations like "write clean code"?

Output a JSON array of findings. Each element:
{
  "title": "short title",
  "body": "1-2 sentences explaining the issue and why it matters",
  "severity": "high|medium|low|info",
  "category": "clarity|completeness|consistency|structure",
  "suggestion": "concrete rewrite or addition — ready to paste, or empty string if not applicable",
  "evidence": ["quoted line or pattern from the file that triggered this"]
}

Rules:
- Only reference content actually present (or clearly absent) in the file. Do not hallucinate.
- Be specific. "This rule is vague" is not useful. "This rule says 'be careful with errors' but doesn't specify whether to use error returns, panics, or log-and-continue" is useful.
- Severity guide: high = actively causes bad agent behavior, medium = missed opportunity, low = nitpick, info = observation.
- If the file is well-written, return fewer findings. Don't manufacture problems.
- Aim for 3-8 findings unless the file warrants more or fewer.
- An empty file should get a single "high" finding about completeness.`

// BuildUserPrompt assembles the CLAUDE.md content (and optionally signal context) into a prompt.
func BuildUserPrompt(fileContent string, signals *advise.SignalContext) string {
	var b strings.Builder

	b.WriteString("## CLAUDE.md Contents\n\n")
	b.WriteString("```markdown\n")
	b.WriteString(fileContent)
	if !strings.HasSuffix(fileContent, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n")

	// Signal context — wired up later. When present, the LLM can cross-reference
	// "your rules say X" against "but your sessions show Y".
	if signals != nil && len(signals.Summary) > 0 {
		b.WriteString("## Behavioral Signals from Agent Transcripts\n\n")
		b.WriteString("The following signals were detected from actual coding sessions. ")
		b.WriteString("Cross-reference these against the CLAUDE.md rules above — ")
		b.WriteString("flag rules that don't match observed behavior, and gaps where signals suggest a missing rule.\n\n")

		fmt.Fprintf(&b, "Sessions analyzed: %d\n\n", signals.SessionCount)

		b.WriteString("### Signal Summary\n")
		for _, r := range signals.Summary {
			fmt.Fprintf(&b, "- %s: %d occurrences (avg confidence %.2f)\n", r.Type, r.Count, r.AvgConfidence)
		}
		b.WriteString("\n")

		if len(signals.Failures) > 0 {
			b.WriteString("### Top Failures\n")
			for _, r := range signals.Failures {
				fmt.Fprintf(&b, "- conf=%.2f evidence=%q\n", r.Confidence, r.Evidence)
			}
			b.WriteString("\n")
		}

		if len(signals.Misalignments) > 0 {
			b.WriteString("### Top Misalignments\n")
			for _, r := range signals.Misalignments {
				fmt.Fprintf(&b, "- conf=%.2f evidence=%q\n", r.Confidence, r.Evidence)
			}
			b.WriteString("\n")
		}

		if len(signals.Loops) > 0 {
			b.WriteString("### Top Loops\n")
			for _, r := range signals.Loops {
				fmt.Fprintf(&b, "- conf=%.2f tool=%s evidence=%q\n", r.Confidence, r.ToolName, r.Evidence)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}
