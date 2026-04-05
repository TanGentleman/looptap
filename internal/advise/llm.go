package advise

import (
	"context"
	"fmt"

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

// Generate sends a system+user prompt pair and returns the text response.
func (c *Client) Generate(ctx context.Context, system, user string) (string, error) {
	resp, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(user), &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(system, genai.RoleUser),
		Temperature:       genai.Ptr(float32(0.3)),
		ResponseMIMEType:  "application/json",
	})
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}
	return resp.Text(), nil
}
