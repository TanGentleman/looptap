package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"looptap/internal/advise"
)

// Run reads a CLAUDE.md file, sends it to the LLM for quality review, and returns findings.
func Run(ctx context.Context, req AnalyzeRequest, apiKey, model string) (*AnalyzeResult, error) {
	content, err := ReadFile(req.FilePath)
	if err != nil {
		return nil, err
	}

	userPrompt := BuildUserPrompt(content)

	// Reuse advise's client — same Gemini wrapper, no need to duplicate.
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

	findings, err := parseFindings(gen.Text)
	if err != nil {
		// Surface the raw text so the user can still see something useful,
		// but don't pretend it's a clarity finding.
		findings = []Finding{{
			Title:    "Unparseable LLM response",
			Body:     fmt.Sprintf("%s\n\nRaw output:\n%s", err, gen.Text),
			Severity: "info",
			Category: "info",
		}}
	}

	usage := &advise.Usage{
		Model:          model,
		PromptTokens:   gen.PromptTokens,
		ResponseTokens: gen.ResponseTokens,
		TotalTokens:    gen.TotalTokens,
		LatencyMs:      latency.Milliseconds(),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	return &AnalyzeResult{
		FilePath: req.FilePath,
		Findings: findings,
		Model:    model,
		Usage:    usage,
	}, nil
}

// parseFindings pulls a JSON array out of the model's response. The prompt asks
// for a ```json fenced block, but advise.ExtractJSONFence tolerates bare JSON
// too in case the model forgets the fences.
func parseFindings(raw string) ([]Finding, error) {
	body := advise.ExtractJSONFence(raw)

	var findings []Finding
	if err := json.Unmarshal([]byte(body), &findings); err != nil {
		return nil, fmt.Errorf("parsing JSON findings: %w", err)
	}
	return findings, nil
}
