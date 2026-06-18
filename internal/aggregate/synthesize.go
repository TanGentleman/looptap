package aggregate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"looptap/internal/advise"
)

// Synthesis is the optional LLM pass over a Report: fleet-wide recommendations
// plus what the call cost. It reuses advise's Gemini wrapper and output shape —
// same engine, a cohort-scoped prompt.
type Synthesis struct {
	Recommendations []advise.Recommendation `json:"recommendations"`
	Model           string                  `json:"model"`
	Usage           *advise.Usage           `json:"usage,omitempty"`
}

// Synthesize feeds a report to the LLM and returns fleet-level recommendations.
// The deterministic Report is the source of truth; this just narrates it.
func Synthesize(ctx context.Context, r *Report, apiKey, model string) (*Synthesis, error) {
	// Nothing to say about an empty cohort — don't spend a token on it.
	if r.Cohort.Signals == 0 {
		return &Synthesis{Model: model}, nil
	}

	client, err := advise.NewClient(ctx, apiKey, model)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	gen, err := client.Generate(ctx, systemPrompt, BuildSynthesisPrompt(r))
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	recs, err := parseRecommendations(gen.Text)
	if err != nil {
		// Don't pretend unparseable output is structured advice — surface it raw.
		recs = []advise.Recommendation{{
			Title:      "Raw fleet advice",
			Body:       gen.Text,
			Confidence: "low",
		}}
	}

	return &Synthesis{
		Recommendations: recs,
		Model:           model,
		Usage: &advise.Usage{
			Model:          model,
			PromptTokens:   gen.PromptTokens,
			ResponseTokens: gen.ResponseTokens,
			TotalTokens:    gen.TotalTokens,
			LatencyMs:      latency.Milliseconds(),
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

func parseRecommendations(raw string) ([]advise.Recommendation, error) {
	body := extractJSONFence(raw)
	var recs []advise.Recommendation
	if err := json.Unmarshal([]byte(body), &recs); err != nil {
		return nil, fmt.Errorf("parsing JSON recommendations: %w", err)
	}
	return recs, nil
}

// extractJSONFence returns the contents of the first ```json ... ``` block,
// falling back to the trimmed input if no fence is found.
func extractJSONFence(s string) string {
	const open = "```json"
	start := strings.Index(s, open)
	if start == -1 {
		return strings.TrimSpace(s)
	}
	rest := s[start+len(open):]
	end := strings.Index(rest, "```")
	if end == -1 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}
