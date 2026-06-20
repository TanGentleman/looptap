package signal

import (
	"regexp"
	"strings"
	"unicode"
)

// errorFamily maps a recognizable token in a chunk of command output to a
// stable class label. Ordered most-specific first: "module not found" must win
// over the generic "not found", and an errno and its English message collapse
// to the same family (ENOENT == "no such file or directory") so they cluster
// together. Substring match on the lowercased text.
var errorFamilies = []struct {
	needle string
	class  string
}{
	{"no such file or directory", "ENOENT"},
	{"enoent", "ENOENT"},
	{"not a directory", "ENOTDIR"},
	{"enotdir", "ENOTDIR"},
	{"permission denied", "EACCES"},
	{"eacces", "EACCES"},
	{"connection refused", "ECONNREFUSED"},
	{"econnrefused", "ECONNREFUSED"},
	{"cannot find module", "module-not-found"},
	{"module not found", "module-not-found"},
	{"modulenotfounderror", "module-not-found"},
	{"command not found", "command-not-found"},
	{"context deadline exceeded", "timeout"},
	{"etimedout", "timeout"},
	{"timed out", "timeout"},
	{"timeout", "timeout"},
	{"too many requests", "rate-limit"},
	{"rate limit", "rate-limit"},
	{"429", "rate-limit"},
	{"out of memory", "oom"},
	{"cannot allocate memory", "oom"},
	{"signal: killed", "oom"},
	{"no space left", "no-space"},
	{"segmentation fault", "segfault"},
	{"sigsegv", "segfault"},
	{"traceback (most recent call last)", "python-traceback"},
	{"panic:", "panic"},
	{"exit code", "exit-code"},
	{"exit status", "exit-code"},
	{"exited with", "exit-code"},
	{"command failed", "command-failed"},
}

var (
	reUUID      = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reHex       = regexp.MustCompile(`0x[0-9a-f]+|\b[0-9a-f]{12,}\b`)
	rePath      = regexp.MustCompile(`(?:[A-Za-z]:)?(?:/[\w.\-]+){2,}/?`)
	reNum       = regexp.MustCompile(`\d+`)
	reWhitespce = regexp.MustCompile(`\s+`)
)

// Normalize lowercases, strips punctuation, and collapses whitespace.
func Normalize(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// ErrorClass distills a chunk of messy command output or signal evidence down
// to a stable error family — the key `looptap patterns` clusters on so that the
// same failure in fifty sessions counts as one shape, not fifty.
//
// First it looks for a recognizable error token (ENOENT, "connection refused",
// an exit-code). Failing that it scrubs the volatile bits — paths, uuids, hex,
// digits — to placeholders and keeps the first few words as a generic family,
// so "/Users/x/api/foo.ts" and "/home/y/api/bar.ts" land in the same bucket.
// Empty or unrecognizable input returns "" (unclassified).
func ErrorClass(s string) string {
	lower := strings.ToLower(s)
	for _, f := range errorFamilies {
		if strings.Contains(lower, f.needle) {
			return f.class
		}
	}

	// No known token — fall back to a scrubbed prefix so structurally-identical
	// errors still cluster. Order matters: uuid before hex before digits.
	scrubbed := lower
	scrubbed = reUUID.ReplaceAllString(scrubbed, "<uuid>")
	scrubbed = reHex.ReplaceAllString(scrubbed, "<hex>")
	scrubbed = rePath.ReplaceAllString(scrubbed, "<path>")
	scrubbed = reNum.ReplaceAllString(scrubbed, "<n>")
	scrubbed = strings.TrimSpace(reWhitespce.ReplaceAllString(scrubbed, " "))
	if scrubbed == "" {
		return ""
	}

	words := strings.Fields(scrubbed)
	if len(words) > 8 {
		words = words[:8]
	}
	return strings.Join(words, " ")
}

// TokenSimilarity computes Jaccard similarity on whitespace-split tokens.
func TokenSimilarity(a, b string) float64 {
	tokA := tokenize(a)
	tokB := tokenize(b)
	if len(tokA) == 0 && len(tokB) == 0 {
		return 1.0
	}
	if len(tokA) == 0 || len(tokB) == 0 {
		return 0.0
	}

	setA := make(map[string]bool, len(tokA))
	for _, t := range tokA {
		setA[t] = true
	}
	setB := make(map[string]bool, len(tokB))
	for _, t := range tokB {
		setB[t] = true
	}

	intersection := 0
	for t := range setA {
		if setB[t] {
			intersection++
		}
	}

	union := len(setA)
	for t := range setB {
		if !setA[t] {
			union++
		}
	}

	return float64(intersection) / float64(union)
}

// MatchPhrases checks if any phrase matches the text within maxEditDist.
// Returns whether a match was found and the matching phrase.
func MatchPhrases(text string, phrases []string, maxEditDist int) (bool, string) {
	normalized := Normalize(text)
	for _, phrase := range phrases {
		np := Normalize(phrase)
		if np == "" {
			continue
		}
		if strings.Contains(normalized, np) {
			return true, phrase
		}
		if maxEditDist > 0 && editDistanceContains(normalized, np, maxEditDist) {
			return true, phrase
		}
	}
	return false, ""
}

func tokenize(s string) []string {
	return strings.Fields(Normalize(s))
}

// editDistanceContains checks if any substring of text is within maxDist of pattern.
// Simple implementation: slide a window of len(pattern)±maxDist over text.
func editDistanceContains(text, pattern string, maxDist int) bool {
	pWords := strings.Fields(pattern)
	tWords := strings.Fields(text)
	pLen := len(pWords)

	for windowSize := max(1, pLen-maxDist); windowSize <= pLen+maxDist && windowSize <= len(tWords); windowSize++ {
		for i := 0; i+windowSize <= len(tWords); i++ {
			window := strings.Join(tWords[i:i+windowSize], " ")
			if levenshtein(window, pattern) <= maxDist {
				return true
			}
		}
	}
	return false
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
