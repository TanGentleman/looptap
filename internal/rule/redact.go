package rule

import (
	"regexp"
	"strings"
)

// excerptCap bounds how much of a turn we carry as evidence. Long enough to
// show the failing line in context, short enough that nobody pastes a whole
// transcript into a card.
const excerptCap = 600

const redacted = "[REDACTED]"

// scrubber is one secret-shaped pattern and how to rewrite a hit. Most just
// vanish into [REDACTED]; the two with a meaningful prefix (a Bearer header, a
// NAME= assignment) keep the prefix so the evidence still reads as what it is.
type scrubber struct {
	re      *regexp.Regexp
	replace string // a regexp expansion template; "" means "replace whole match"
}

// This is a PRE-pass, not THE redactor. tracers/redact.zig is the single
// source of truth and re-redacts every excerpt at the share boundary (exactly
// as POST /inbox does). We scrub here only so that looptap's own local output
// — `looptap patterns --format json`, which may get piped around a machine —
// doesn't hand out raw secrets on the way. The rule of the road is therefore
// PRECISE OVER EXHAUSTIVE: a missed secret gets caught downstream, but a false
// positive eats real error text and makes the evidence useless. When in doubt,
// leave it alone. Do NOT grow this into an authoritative redactor.
var scrubbers = []scrubber{
	// Provider API keys with distinctive, hard-to-false-positive prefixes.
	{re: regexp.MustCompile(`sk-[A-Za-z0-9_-]{16,}`)},        // OpenAI-style
	{re: regexp.MustCompile(`gh[posru]_[A-Za-z0-9]{16,}`)},   // GitHub PAT / token
	{re: regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)}, // GitHub fine-grained PAT
	{re: regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},             // AWS access key id
	{re: regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,}`)}, // Slack token
	// Authorization: Bearer <token> — keep the header, drop the credential.
	{re: regexp.MustCompile(`(?i)(authorization:\s*bearer\s+)[A-Za-z0-9._-]+`), replace: "${1}" + redacted},
	// NAME=secret, but ONLY when the name looks secret-shaped, so ordinary
	// FOO=bar in error text survives untouched.
	{re: regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD)[A-Z0-9_]*)\s*=\s*('[^']*'|"[^"]*"|[^\s'"]+)`), replace: "${1}=" + redacted},
}

// Redact caps an evidence excerpt and scrubs the obvious secrets out of it,
// returning the cleaned text and how many secrets it caught.
func Redact(s string) (string, int) {
	s = capExcerpt(s)

	count := 0
	for _, sc := range scrubbers {
		s = sc.re.ReplaceAllStringFunc(s, func(m string) string {
			count++
			if sc.replace == "" {
				return redacted
			}
			return sc.re.ReplaceAllString(m, sc.replace)
		})
	}
	return s, count
}

const elision = "\n…\n"

// capExcerpt trims an over-long excerpt to the cap, keeping the head (where the
// command usually is) and the tail (where the error usually lands) and eliding
// the boring middle — the error token is almost always at one end.
func capExcerpt(s string) string {
	if len(s) <= excerptCap {
		return s
	}
	head := excerptCap * 2 / 3
	tail := excerptCap - head - len(elision)
	if tail < 0 {
		tail = 0
	}
	return strings.TrimSpace(s[:head]) + elision + strings.TrimSpace(s[len(s)-tail:])
}
