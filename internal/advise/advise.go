package advise

import (
	"context"
	"encoding/json"
	"fmt"

	"looptap/internal/db"
)

// Run gathers signal context, asks the LLM for CLAUDE.md suggestions, and returns them.
func Run(ctx context.Context, database *db.DB, req AdviceRequest, apiKey, model string) (*AdviceResult, error) {
	// 1. Gather signal context from the database
	sigCtx, err := GatherContext(database.Conn(), req.Project)
	if err != nil {
		return nil, fmt.Errorf("gathering context: %w", err)
	}

	if len(sigCtx.Summary) == 0 {
		return &AdviceResult{Model: model}, nil
	}

	// 2. Build the prompt
	userPrompt := BuildUserPrompt(sigCtx)

	// 3. Call the LLM
	client, err := NewClient(ctx, apiKey, model)
	if err != nil {
		return nil, err
	}

	raw, err := client.Generate(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 4. Parse the response
	var recs []Recommendation
	if err := json.Unmarshal([]byte(raw), &recs); err != nil {
		// If the model didn't return clean JSON, wrap the whole thing
		recs = []Recommendation{{
			Title:      "Raw advice",
			Body:       raw,
			Confidence: "low",
		}}
	}

	return &AdviceResult{
		Recommendations: recs,
		Model:           model,
	}, nil
}
