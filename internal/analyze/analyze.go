package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"looptap/internal/advise"
)

// Run reads a CLAUDE.md file, sends it to the LLM for quality review, and returns findings.
// When db is non-nil and signals are available, they enrich the review (not yet wired).
func Run(ctx context.Context, req AnalyzeRequest, apiKey, model string) (*AnalyzeResult, error) {
	// 1. Read the file
	content, err := ReadFile(req.FilePath)
	if err != nil {
		return nil, err
	}

	// 2. Gather signal context (future — pass nil for now)
	var signals *advise.SignalContext

	// 3. Build the prompt
	userPrompt := BuildUserPrompt(content, signals)

	// 4. Call the LLM (reuse advise's client — same Gemini wrapper)
	client, err := advise.NewClient(ctx, apiKey, model)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	gen, err := client.Generate(ctx, systemPrompt, userPrompt)
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 5. Parse the response
	var findings []Finding
	if err := json.Unmarshal([]byte(gen.Text), &findings); err != nil {
		findings = []Finding{{
			Title:    "Raw analysis",
			Body:     gen.Text,
			Severity: "info",
			Category: "clarity",
		}}
	}

	usage := &advise.Usage{
		Model:          model,
		PromptTokens:   gen.PromptTokens,
		ResponseTokens: gen.ResponseTokens,
		TotalTokens:    gen.TotalTokens,
		LatencyMs:      latency.Milliseconds(),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Project:        req.Project,
	}

	return &AnalyzeResult{
		FilePath: req.FilePath,
		Findings: findings,
		Model:    model,
		Usage:    usage,
	}, nil
}
