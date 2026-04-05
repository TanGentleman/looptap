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

// Usage tracks what the LLM call cost us.
type Usage struct {
	Model           string  `json:"model"`
	PromptTokens    int32   `json:"prompt_tokens"`
	ResponseTokens  int32   `json:"response_tokens"`
	TotalTokens     int32   `json:"total_tokens"`
	LatencyMs       int64   `json:"latency_ms"`
	Timestamp       string  `json:"timestamp"`
	Project         string  `json:"project,omitempty"`
}

// AdviceResult is what comes back from the LLM.
type AdviceResult struct {
	Recommendations []Recommendation
	Model           string
	Usage           *Usage
}
