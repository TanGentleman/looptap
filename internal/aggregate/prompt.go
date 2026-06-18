package aggregate

import (
	"fmt"
	"strings"
)

// systemPrompt frames the model as a fleet-level coach: it reads an aggregate
// report (not raw transcripts) and proposes changes that help everyone at once.
const systemPrompt = `You are looptap's fleet advisor. You read an aggregated report of behavioral signals collected from many coding-agent sessions across multiple users and teams, and you propose changes that improve agent behavior at scale.

You are NOT looking at one transcript. Every number you see is already rolled up across a cohort. Your job is to find the systemic problems — the ones worth fixing once, centrally (a shared CLAUDE.md rule, a tool guardrail, an onboarding note) — rather than one-off session quirks.

## What the signals mean
- failure — a tool call errored (the report attributes failures to the tool that ran)
- loop — the agent repeated the same tool call in a death spiral
- misalignment — the user had to correct the agent
- stagnation — the agent produced near-identical output turn after turn
- disengagement — the user gave up
- exhaustion — rate limits, context overflow, timeouts
- satisfaction — the user expressed approval (a good sign; don't "fix" it)

## How to prioritize
- A pattern that spans many users or teams beats a high raw count concentrated in one user.
- "Failing tools" and "recurring patterns" are your richest seams — they point at concrete, fixable behavior.
- Prefer recommendations an agent can follow mechanically over vague advice.

## Output format
Wrap your response in a ` + "`json`" + ` fenced code block: a JSON array of recommendations:
{
  "title": "short title",
  "body": "1-2 sentences: the systemic problem and who it affects",
  "snippet": "ready-to-paste CLAUDE.md text (1-3 lines), or a concrete process change",
  "confidence": "high|medium|low",
  "evidence": ["signal/tool/pattern: brief stat from the report", ...]
}

## Rules
- Only reference signals, tools, and patterns that appear in the report. Do not invent.
- Tie each recommendation to the breadth of impact (how many users/teams/sessions).
- Consolidate: if several stats share a root cause, make one recommendation.
- If the cohort is too small or too clean to draw fleet-level conclusions, return [].
- Aim for 2-5 recommendations.`

// BuildSynthesisPrompt renders the report into the compact, labeled text the
// model reads. It mirrors the on-screen report but trims to what's decision-useful.
func BuildSynthesisPrompt(r *Report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Fleet report\n")
	scope := describeScope(r.Filter)
	if scope != "" {
		fmt.Fprintf(&b, "Scope: %s\n", scope)
	}
	c := r.Cohort
	fmt.Fprintf(&b, "Cohort: %d sessions, %d users, %d teams, %d signals",
		c.Sessions, c.Users, c.Teams, c.Signals)
	if c.EarliestSeen != "" {
		fmt.Fprintf(&b, " (%s … %s)", c.EarliestSeen, c.LatestSeen)
	}
	b.WriteString("\n\n")

	b.WriteString("## Signal breakdown (occurrences / sessions affected / users affected / avg conf / affected rate)\n")
	for _, s := range r.SignalBreakdown {
		fmt.Fprintf(&b, "- %s [%s]: %d / %d / %d / %.2f / %.0f%%\n",
			s.Type, s.Category, s.Occurrences, s.SessionsAffected, s.UsersAffected,
			s.AvgConfidence, s.AffectedRate*100)
	}
	b.WriteString("\n")

	if len(r.FailingTools) > 0 {
		b.WriteString("## Failing tools (failures / loops / sessions / users / teams)\n")
		for _, t := range r.FailingTools {
			fmt.Fprintf(&b, "- %s: %d failures, %d loops, %d sessions, %d users, %d teams\n",
				t.Tool, t.Failures, t.Loops, t.SessionsAffected, t.UsersAffected, t.TeamsAffected)
		}
		b.WriteString("\n")
	}

	if len(r.Teams) > 0 {
		b.WriteString("## Teams (signals per session)\n")
		for _, t := range r.Teams {
			fmt.Fprintf(&b, "- %s: %.2f signals/session over %d sessions; top signal %s\n",
				t.Team, t.SignalsPerSession, t.Sessions, t.TopSignal)
		}
		b.WriteString("\n")
	}

	if len(r.RecurringPatterns) > 0 {
		b.WriteString("## Recurring patterns (occurrences across users/teams)\n")
		for _, p := range r.RecurringPatterns {
			fmt.Fprintf(&b, "- %s: %q — %d times, %d users, %d teams\n",
				p.Type, p.Evidence, p.Occurrences, p.Users, p.Teams)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func describeScope(f ReportFilter) string {
	var parts []string
	if f.Team != "" {
		parts = append(parts, "team="+f.Team)
	}
	if f.Project != "" {
		parts = append(parts, "project~"+f.Project)
	}
	if f.Source != "" {
		parts = append(parts, "source="+f.Source)
	}
	if f.Since != "" {
		parts = append(parts, "since="+f.Since)
	}
	if f.Until != "" {
		parts = append(parts, "until="+f.Until)
	}
	if f.MinConfidence > 0 {
		parts = append(parts, fmt.Sprintf("min-confidence=%.2f", f.MinConfidence))
	}
	return strings.Join(parts, " ")
}
