package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// OpenAIProvider implements EmbeddingProvider using OpenAI-compatible embeddings API
// (e.g. Bifrost gateway, or self-hosted OpenAI-compatible services).
type OpenAIProvider struct {
	Model   string
	BaseURL string
	APIKey  string
	client  *http.Client
}

type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func NewOpenAIProvider(model, baseURL, apiKey string) *OpenAIProvider {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		Model:   model,
		BaseURL: baseURL,
		APIKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (o *OpenAIProvider) Dimension() int {
	// Qwen3-Embedding-0.6B-GGUF produces 1024-dimensional vectors
	if o.Model == "Sense/Qwen3-Embedding-0.6B-GGUF" {
		return 1024
	}
	return 1536 // default for text-embedding-3-small
}

func (o *OpenAIProvider) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	reqBody := openAIEmbedRequest{
		Model: o.Model,
		Input: texts,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/embeddings", o.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI embeddings API returned HTTP %d", resp.StatusCode)
	}

	var oresp openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&oresp); err != nil {
		return nil, err
	}

	if len(oresp.Data) != len(texts) {
		return nil, fmt.Errorf("unexpected response size: expected %d, got %d", len(texts), len(oresp.Data))
	}

	embeddings := make([][]float32, len(texts))
	for i, d := range oresp.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}
