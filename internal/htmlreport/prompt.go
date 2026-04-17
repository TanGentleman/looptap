package htmlreport

import (
	"fmt"
	"strings"
)

const systemAppend = `You are generating a one-shot HTML report for a development team.

Your ENTIRE response must be a single self-contained HTML document:
- Start with <!doctype html>, end with </html>. Nothing outside those tags.
- No markdown fences. No commentary. No preamble.
- No JavaScript. No external fonts, stylesheets, or images.`

func buildPrompt(r *Resolved) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Analyze the git branch `%s` in the current repository and produce a beautifully-designed, self-contained HTML page that tells the story of this branch to the rest of the dev team.\n\n", r.Branch)

	b.WriteString("## Steps\n\n")
	b.WriteString("1. Find the base branch: try `git symbolic-ref refs/remotes/origin/HEAD`, fall back to `main` or `master`.\n")
	b.WriteString("2. If the target branch IS the default branch, summarize the last ~20 commits instead of diffing against itself.\n")
	b.WriteString("3. Read the commit log, diff stats, and the actual changed files. Understand WHY each change was made — not just what lines moved.\n")
	b.WriteString("4. Draft a narrative: the problem, the approach, the tradeoffs, the risks, and what a reviewer should look at first.\n\n")

	b.WriteString("## Output requirements\n\n")
	b.WriteString("A single HTML document:\n")
	b.WriteString("- `<!doctype html>` at the top, `</html>` at the bottom. Nothing outside.\n")
	b.WriteString("- All CSS in a `<style>` tag. No external resources. No JavaScript.\n")
	b.WriteString("- `color-scheme: light dark` for both themes.\n")
	b.WriteString("- Header: branch name, repo name, generated timestamp.\n")
	b.WriteString("- Sections: narrative summary, key commits, files changed with per-file notes, risks and a review checklist.\n")
	b.WriteString("- Tone: confident, concise, engineer-to-engineer. Not an executive status report.\n")
	b.WriteString("- Only report facts from git. No invented commits, files, or contributors.\n")

	return b.String()
}
