package advise

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	start := time.Now()
	gen, err := client.Generate(ctx, systemPrompt, userPrompt)
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// 4. Parse the response — strip ```json fences if present.
	body := extractJSONFence(gen.Text)
	var recs []Recommendation
	if err := json.Unmarshal([]byte(body), &recs); err != nil {
		// If the model didn't return clean JSON, wrap the whole thing
		recs = []Recommendation{{
			Title:      "Raw advice",
			Body:       gen.Text,
			Confidence: "low",
		}}
	}

	usage := &Usage{
		Model:          model,
		PromptTokens:   gen.PromptTokens,
		ResponseTokens: gen.ResponseTokens,
		TotalTokens:    gen.TotalTokens,
		LatencyMs:      latency.Milliseconds(),
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Project:        req.Project,
	}

	// 5. Write usage stats to disk (best-effort, don't fail the command)
	if err := appendUsage(usage); err != nil {
		fmt.Fprintf(os.Stderr, "warning: couldn't write usage stats: %v\n", err)
	}

	return &AdviceResult{
		Recommendations: recs,
		Model:           model,
		Usage:           usage,
	}, nil
}

// appendUsage appends a JSONL line to ~/.looptap/advise-usage.jsonl.
func appendUsage(u *Usage) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".looptap", "advise-usage.jsonl")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
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
