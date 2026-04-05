package signal

import (
	"strings"
	"unicode"
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
