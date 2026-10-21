package api

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// bagProvider is a deterministic bag-of-words embedding (64 dimensions).
type bagProvider struct{}

func fnv32a(s string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func (bagProvider) Dimension() int { return 64 }

func (bagProvider) GenerateEmbeddings(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, 64)
		lower := strings.ToLower(text)
		var token strings.Builder
		flush := func() {
			if token.Len() == 0 {
				return
			}
			vec[fnv32a(token.String())%64]++
			token.Reset()
		}
		for j := 0; j < len(lower); j++ {
			c := lower[j]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
				token.WriteByte(c)
			} else {
				flush()
			}
		}
		flush()
		var sum float64
		for _, v := range vec {
			sum += float64(v) * float64(v)
		}
		if sum > 0 {
			norm := float32(math.Sqrt(sum))
			for j := range vec {
				vec[j] /= norm
			}
		}
		out[i] = vec
	}
	return out, nil
}

func newV5Handler(t *testing.T) (*Handler, func()) {
	t.Helper()
	handler, cleanup := setupTestHandler(t)
	handler.Engine.Provider = bagProvider{}
	handler.Processor.Provider = bagProvider{}
	return handler, cleanup
}

func doV5(t *testing.T, h *Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var out map[string]any
	if rr.Body.Len() > 0 {
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: response is not JSON: %v (%s)", method, path, err, rr.Body.String())
		}
	}
	return rr.Code, out
}

func processAll(t *testing.T, h *Handler) {
	t.Helper()
	for i := 0; i < 50; i++ {
		n, err := h.Processor.ProcessOnce(context.Background())
		if err != nil {
			t.Fatalf("process: %v", err)
		}
		if n == 0 {
			return
		}
	}
	t.Fatal("processor did not drain the queue")
}

func createNamespace(t *testing.T, h *Handler, ns string) {
	t.Helper()
	code, body := doV5(t, h, "POST", "/ns/"+ns+"/document", map[string]any{"content": "seed document for " + ns})
	if code != http.StatusOK {
		t.Fatalf("seed document: status %d body %v", code, body)
	}
	processAll(t, h)
}
