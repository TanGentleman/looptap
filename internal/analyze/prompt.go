package analyze

import "strings"

const systemPrompt = `You are looptap's CLAUDE.md quality reviewer. You analyze CLAUDE.md files — the configuration files that shape how AI coding assistants behave in a project — and identify problems, gaps, and improvements.

Evaluate the file across these dimensions:
- **Clarity**: Are rules specific and unambiguous? Would an AI agent know exactly what to do?
- **Completeness**: Are there gaps the file's own scope implies should be filled? Judge this against what the file is clearly trying to cover, not a generic checklist. A minimal file that knows what it is is not "incomplete".
- **Consistency**: Do any rules contradict each other? Are there duplicates saying the same thing differently?
- **Structure**: Is it well-organized? Could it be grouped better? Are there walls of text that should be broken up?
- **Actionability**: Are rules concrete enough to follow, or are they vague aspirations like "write clean code"?

Output a JSON array of findings, wrapped in a fenced code block tagged ` + "`json`" + `. Nothing outside the fence. Each element:
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
- Return only findings that clear the severity bar. Zero findings is a valid answer for a well-written file — do not manufacture problems to hit a quota.
- Do not suggest adding content that belongs in a different file. Global ~/.claude/CLAUDE.md is for cross-project preferences; per-project CLAUDE.md is for that project's conventions. If a suggestion would be better placed elsewhere, skip it.`

// BuildUserPrompt assembles the CLAUDE.md content into a prompt.
func BuildUserPrompt(fileContent string) string {
	var b strings.Builder

	b.WriteString("## CLAUDE.md Contents\n\n")
	b.WriteString("```markdown\n")
	b.WriteString(fileContent)
	if !strings.HasSuffix(fileContent, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n")

	return b.String()
}
