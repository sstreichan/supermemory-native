package embedding

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type GeminiProvider struct {
	APIKey  string
	Model   string
	BaseURL string
	client  *http.Client
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiEmbedRequest struct {
	Model   string        `json:"model"`
	Content geminiContent `json:"content"`
}

type geminiBatchRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiEmbedResponse struct {
	Embeddings []struct {
		Values []float32 `json:"values"`
	} `json:"embeddings"`
}

func NewGeminiProvider(apiKey string) *GeminiProvider {
	return &GeminiProvider{
		APIKey:  apiKey,
		Model:   "models/text-embedding-004",
		BaseURL: "https://generativelanguage.googleapis.com",
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (g *GeminiProvider) Dimension() int {
	return 768 // text-embedding-004 has 768 dimensions
}

func (g *GeminiProvider) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if g.APIKey == "" {
		return nil, errors.New("Gemini API key is not configured")
	}
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	batchReq := geminiBatchRequest{
		Requests: make([]geminiEmbedRequest, len(texts)),
	}

	for i, text := range texts {
		batchReq.Requests[i] = geminiEmbedRequest{
			Model: g.Model,
			Content: geminiContent{
				Parts: []geminiPart{{Text: text}},
			},
		}
	}

	payload, err := json.Marshal(batchReq)
	if err != nil {
		return nil, err
	}

	// The key travels in a header, never in the URL: query strings end up in
	// proxy logs, server logs and error messages.
	url := fmt.Sprintf("%s/v1beta/models/text-embedding-004:batchEmbedContents", g.BaseURL)
	resp, err := doWithRetry(g.client, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", g.APIKey)
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errorResponse)
		if errorResponse.Error.Message != "" {
			return nil, fmt.Errorf("Gemini API error (HTTP %d): %s", resp.StatusCode, errorResponse.Error.Message)
		}
		return nil, fmt.Errorf("Gemini API returned HTTP %d", resp.StatusCode)
	}

	var geminiResp geminiEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}

	if len(geminiResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("unexpected response size: expected %d, got %d", len(texts), len(geminiResp.Embeddings))
	}

	embeddings := make([][]float32, len(texts))
	for i, emb := range geminiResp.Embeddings {
		embeddings[i] = emb.Values
	}

	return embeddings, nil
}
