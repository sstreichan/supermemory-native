package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// llmConfig points at an OpenAI-compatible chat-completions endpoint. It is
// optional: without configuration every retrieval path stays deterministic.
type llmConfig struct {
	BaseURL string
	Model   string
	APIKey  string
	Client  *http.Client
}

func llmFromEnv() *llmConfig {
	model := strings.TrimSpace(os.Getenv("SUPERMEMORY_LLM_MODEL"))
	base := strings.TrimSpace(os.Getenv("SUPERMEMORY_LLM_BASE_URL"))
	if model == "" || base == "" {
		return nil
	}
	return &llmConfig{
		BaseURL: strings.TrimRight(base, "/"),
		Model:   model,
		APIKey:  strings.TrimSpace(os.Getenv("SUPERMEMORY_LLM_API_KEY")),
		Client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *llmConfig) endpoint() string {
	if strings.HasSuffix(c.BaseURL, "/chat/completions") {
		return c.BaseURL
	}
	return c.BaseURL + "/chat/completions"
}

type llmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chat sends one system/user turn and returns the assistant text.
func (c *llmConfig) chat(ctx context.Context, system, user string) (string, error) {
	if c == nil || c.Model == "" || c.BaseURL == "" {
		return "", fmt.Errorf("llm is not configured")
	}
	payload := map[string]any{
		"model":       c.Model,
		"temperature": 0,
		"messages": []llmMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("llm status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
