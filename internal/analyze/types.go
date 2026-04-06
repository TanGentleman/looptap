package analyze

import "looptap/internal/advise"

// AnalyzeRequest controls what gets analyzed.
type AnalyzeRequest struct {
	FilePath string // path to the CLAUDE.md (or similar) file
	Project  string // optional — scope signal context to one project

	// Future: skills discovery, .claude/ directory scanning, etc.
}

// Finding is a single observation about the analyzed file.
type Finding struct {
	Title      string   `json:"title"`
	Body       string   `json:"body"`       // what's wrong and why it matters
	Severity   string   `json:"severity"`   // "high", "medium", "low", "info"
	Category   string   `json:"category"`   // "clarity", "completeness", "consistency", "structure"
	Suggestion string   `json:"suggestion"` // concrete rewrite or addition, if applicable
	Evidence   []string `json:"evidence"`   // lines or patterns that triggered the finding
}

// AnalyzeResult is the full analysis output.
type AnalyzeResult struct {
	FilePath string
	Findings []Finding
	Model    string
	Usage    *advise.Usage
}
