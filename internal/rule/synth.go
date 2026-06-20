package rule

import (
	"fmt"
	"regexp"
	"strings"
)

// ExampleTurn is a representative turn pulled from the `turns` table for a
// cluster — the raw material Synthesize caps and redacts into Evidence.
type ExampleTurn struct {
	SessionID string
	TurnIdx   int
	ToolName  string
	IsError   bool
	Content   string
}

// maxEvidence caps how many example turns ride along on a card. Three is enough
// to convince a skeptic; more is just transcript.
const maxEvidence = 3

// Synthesize turns a clustered Pattern and its example turns into a Card,
// deterministically — NO LLM, NO API key. A per-(signal, error-class) template
// supplies the rule wording; a generic fallback guarantees every gated cluster
// still yields a usable card. The LLM path (advise) may later refine the
// wording, but it starts from exactly this.
func Synthesize(p Pattern, examples []ExampleTurn) Card {
	title, snippet, target := templateFor(p)
	return Card{
		ID:       slug(p.Signal, p.Tool, p.ErrorClass),
		Pattern:  p,
		Evidence: buildEvidence(examples),
		Rule: Rule{
			Title:      title,
			Snippet:    snippet,
			Rationale:  rationale(p),
			Target:     target,
			Confidence: confidenceFor(p.SessionCount),
			Source:     SourceTemplate,
		},
	}
}

// templateFor maps a pattern to rule wording. Specific (signal + error class)
// first, then signal-level, then a generic fallback — so a brand-new error
// class still produces something a human can act on.
func templateFor(p Pattern) (title, snippet, target string) {
	switch {
	case p.Signal == "failure" && p.ErrorClass == "ENOENT":
		return "Verify a path exists before using it",
			"Before `cd <dir>` or running a command in a subdirectory, confirm the directory exists (e.g. `ls <dir>`); don't assume a path from memory.",
			TargetAgents

	case p.Signal == "failure" && p.ErrorClass == "command-not-found":
		return "Check a command is available before invoking it",
			"Before calling a CLI tool, confirm it's installed (`command -v <tool>`); if it isn't, install it or choose an alternative instead of retrying.",
			TargetAgents

	case p.Signal == "failure" && p.ErrorClass == "module-not-found":
		return "Install dependencies before importing them",
			"When an import fails with a missing module, install or declare the dependency first; don't loop on the same failing import.",
			TargetAgents

	case p.Signal == "failure" && p.ErrorClass == "EACCES":
		return "Check permissions before writing or executing",
			"When an operation fails with permission denied, check ownership and mode (or pick a writable path) rather than retrying the same call.",
			TargetAgents

	case p.Signal == "failure" && p.ErrorClass == "exit-code":
		return "Read the error before retrying a failed command",
			"When a command exits non-zero, read its output and fix the cause before re-running; a blind retry usually fails the same way.",
			TargetAgents

	case p.Signal == "loop":
		return "Break the retry loop — change approach after two failures",
			"If the same tool call fails twice, stop repeating it. Re-read the error and change the inputs or the approach before a third attempt.",
			TargetAgents

	case p.Signal == "misalignment":
		return "Confirm intent before a large or destructive change",
			"When the user corrects you or the goal is ambiguous, restate the plan and confirm before making sweeping edits.",
			TargetAgents

	case p.Signal == "exhaustion":
		return "Work within resource limits",
			"When you hit a rate limit or run out of context, narrow or batch the work rather than retrying the same oversized request.",
			TargetAgents

	default:
		return genericTitle(p), genericSnippet(p), TargetAgents
	}
}

func genericTitle(p Pattern) string {
	where := p.Signal
	if p.Tool != "" {
		where = fmt.Sprintf("%s in %s", p.Signal, p.Tool)
	}
	return "Address recurring " + where
}

func genericSnippet(p Pattern) string {
	subject := p.Signal
	if p.Tool != "" {
		subject = p.Signal + " from " + p.Tool
	}
	if p.ErrorClass != "" {
		subject += " (" + p.ErrorClass + ")"
	}
	return fmt.Sprintf("Sessions repeatedly hit %s. Investigate the root cause and add a guardrail so it stops recurring.", subject)
}

// rationale is the cluster summary plus the evidence count — the "why" in one
// breath, with the session tally that earned the card its place.
func rationale(p Pattern) string {
	n := p.SessionCount
	plural := "s"
	if n == 1 {
		plural = ""
	}
	return fmt.Sprintf("%s (seen in %d session%s).", strings.TrimRight(p.Summary, "."), n, plural)
}

func confidenceFor(sessionCount int) string {
	switch {
	case sessionCount >= 10:
		return "high"
	case sessionCount >= 5:
		return "medium"
	default:
		return "low"
	}
}

func buildEvidence(examples []ExampleTurn) []Evidence {
	var ev []Evidence
	for _, e := range examples {
		if len(ev) >= maxEvidence {
			break
		}
		excerpt, n := Redact(e.Content)
		ev = append(ev, Evidence{
			SessionID:  ShortID(e.SessionID),
			TurnIdx:    e.TurnIdx,
			ToolName:   e.ToolName,
			IsError:    e.IsError,
			Excerpt:    excerpt,
			Redactions: n,
		})
	}
	return ev
}

// ShortID trims a full session id (sha256 hex) to the 8-char prefix the
// contract uses for display. Short ids stay as-is, so test fixtures read clean.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slug builds a stable card id from its parts, e.g. ("failure","Bash","ENOENT")
// -> "failure-bash-enoent". Empty parts drop out.
func slug(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(p), "-"), "-")
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "-")
}
