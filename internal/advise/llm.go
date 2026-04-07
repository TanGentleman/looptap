package advise

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// Client wraps a Gemini API connection.
// This is the single file that touches the LLM SDK — swap point for adk-go later.
type Client struct {
	client *genai.Client
	model  string
}

// NewClient creates a Gemini client. apiKey must be non-empty.
func NewClient(ctx context.Context, apiKey, model string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("no API key — set GOOGLE_API_KEY or configure [advise] api_key in config.toml")
	}
	c, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}
	return &Client{client: c, model: model}, nil
}

// GenerateResult holds the response text and token counts.
type GenerateResult struct {
	Text           string
	PromptTokens   int32
	ResponseTokens int32
	TotalTokens    int32
}

// Generate sends a system+user prompt pair and returns the text response + usage.
//
// Note: we do NOT set ResponseMIMEType=application/json here. Prompts that want
// structured output should ask for a ```json fenced block and use ExtractJSONFence
// to pull it out. JSON mode coupled us to Gemini's quirks and gave the model no
// room to explain itself — parsing a fence is a small tax for a lot of freedom.
func (c *Client) Generate(ctx context.Context, system, user string) (*GenerateResult, error) {
	resp, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(user), &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(system, genai.RoleUser),
		Temperature:       genai.Ptr(float32(0.3)),
	})
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	result := &GenerateResult{Text: resp.Text()}
	if u := resp.UsageMetadata; u != nil {
		result.PromptTokens = u.PromptTokenCount
		result.ResponseTokens = u.CandidatesTokenCount
		result.TotalTokens = u.TotalTokenCount
	}
	return result, nil
}

// ExtractJSONFence returns the contents of the first ```json ... ``` block in s.
// If no fence is found, the trimmed input is returned unchanged — callers can
// still try to unmarshal bare JSON, and the error message will be honest about
// what the model sent back.
func ExtractJSONFence(s string) string {
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
