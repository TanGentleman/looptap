package advise

// AdviceRequest controls what the advisor looks at.
type AdviceRequest struct {
	Project string // scope to one project (empty = all)
}

// Recommendation is a single piece of actionable advice.
type Recommendation struct {
	Title      string   `json:"title"`
	Body       string   `json:"body"`       // why this matters
	Snippet    string   `json:"snippet"`    // ready-to-paste CLAUDE.md text
	Confidence string   `json:"confidence"` // high, medium, low
	Evidence   []string `json:"evidence"`   // signal types + session IDs
}

// AdviceResult is what comes back from the LLM.
type AdviceResult struct {
	Recommendations []Recommendation
	Model           string
}
