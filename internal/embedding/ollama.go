package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type OllamaProvider struct {
	Model   string
	BaseURL string
	client  *http.Client
}

type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func NewOllamaProvider(model string) *OllamaProvider {
	if model == "" {
		model = "nomic-embed-text"
	}
	return &OllamaProvider{
		Model:   model,
		BaseURL: "http://127.0.0.1:11434",
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (o *OllamaProvider) Dimension() int {
	if o.Model == "mxbai-embed-large" {
		return 1024
	}
	return 768 // nomic-embed-text is 768 dimensions
}

func (o *OllamaProvider) GenerateEmbeddings(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	embeddings := make([][]float32, len(texts))
	var wgMutex sync.Mutex
	var firstErr error

	var wgStd sync.WaitGroup
	for i, text := range texts {
		wgStd.Add(1)
		go func(index int, t string) {
			defer wgStd.Done()

			emb, err := o.embedSingle(t)
			if err != nil {
				wgMutex.Lock()
				if firstErr == nil {
					firstErr = err
				}
				wgMutex.Unlock()
				return
			}

			wgMutex.Lock()
			embeddings[index] = emb
			wgMutex.Unlock()
		}(i, text)
	}

	wgStd.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return embeddings, nil
}

func (o *OllamaProvider) embedSingle(text string) ([]float32, error) {
	embedReq := ollamaEmbedRequest{
		Model:  o.Model,
		Prompt: text,
	}

	payload, err := json.Marshal(embedReq)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/embeddings", o.BaseURL)
	resp, err := doWithRetry(o.client, func() (*http.Request, error) {
		req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned HTTP %d", resp.StatusCode)
	}

	var oResp ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
		return nil, err
	}

	return oResp.Embedding, nil
}
