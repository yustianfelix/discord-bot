package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const defaultModel = "gemini-1.5-flash"

// GeminiClient wraps the Google Generative AI client and exposes a safe prompt API.
type GeminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
	mu     sync.Mutex
}

func NewGeminiClient(apiKey string) (*GeminiClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("missing Gemini API key")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("create Gemini client: %w", err)
	}

	model := client.GenerativeModel(defaultModel)
	model.SetTemperature(0.7)

	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

func (c *GeminiClient) Ask(ctx context.Context, prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("generate content: %w", err)
	}

	var builder strings.Builder
	for _, candidate := range resp.Candidates {
		if candidate == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if text, ok := part.(genai.Text); ok {
				builder.WriteString(string(text))
			}
		}
	}

	result := strings.TrimSpace(builder.String())
	if result == "" {
		return "", fmt.Errorf("Gemini returned an empty response")
	}

	return result, nil
}

func (c *GeminiClient) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}
