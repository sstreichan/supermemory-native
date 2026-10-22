package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/supermemory-native/supermemory-native/internal/db"
)

func TestV5FusionHelpers(t *testing.T) {
	vectorHits := []db.Hit{
		{ID: "a", Text: "alpha beta", Similarity: 0.9},
		{ID: "b", Text: "alpha", Similarity: 0.8},
	}
	lexical := []db.Hit{{ID: "b", Text: "alpha", Similarity: 0.8}}
	fused := fuseRRF(vectorHits, lexical, 0)
	if len(fused) != 2 || fused[0].ID != "b" {
		t.Fatalf("RRF should promote the lexical match, got %+v", fused)
	}

	scores := lexicalScores("zebra", []db.Hit{
		{ID: "z", Text: "a zebra crossing"},
		{ID: "o", Text: "an alpaca farm"},
	})
	if scores["z"] <= 0 {
		t.Fatalf("expected zebra to match, got %v", scores)
	}
	if _, ok := scores["o"]; ok {
		t.Fatalf("alpaca must not match zebra: %v", scores)
	}
}

func TestV5SearchRewriteQuery(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "retrieval_rewrite"
	code, body := doV5(t, h, "POST", "/ns/"+ns+"/document", map[string]any{
		"id": "d1", "content": "A quokka lives on Rottnest Island.",
	})
	if code != http.StatusOK {
		t.Fatalf("create: %d %v", code, body)
	}
	processAll(t, h)

	var sawRewrite atomic.Bool
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []llmMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if len(payload.Messages) > 0 && strings.Contains(payload.Messages[0].Content, "Rewrite the user's search query") {
			sawRewrite.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "quokka"}}},
		})
	}))
	defer llm.Close()

	query := map[string]any{"query": "unrelatedword"}
	code, plain := doV5(t, h, "POST", "/ns/"+ns+"/search", query)
	if code != http.StatusOK {
		t.Fatalf("plain search: %d %v", code, plain)
	}
	if n := len(plain["results"].([]any)); n != 0 {
		t.Fatalf("expected no matches before rewrite, got %d", n)
	}

	h.LLM = &llmConfig{BaseURL: llm.URL, Model: "stub", Client: llm.Client()}
	query["rewriteQuery"] = true
	code, rewritten := doV5(t, h, "POST", "/ns/"+ns+"/search", query)
	if code != http.StatusOK {
		t.Fatalf("rewritten search: %d %v", code, rewritten)
	}
	if n := len(rewritten["results"].([]any)); n == 0 {
		t.Fatal("rewritten query should match the quokka document")
	}
	if !sawRewrite.Load() {
		t.Fatal("LLM rewrite endpoint was not called with the rewrite prompt")
	}
}

func TestV5SearchRerankWithLLM(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "retrieval_rerank"
	for _, doc := range []struct{ id, content string }{
		{"a", "alpha"},
		{"b", "alpha beta"},
	} {
		code, body := doV5(t, h, "POST", "/ns/"+ns+"/document", map[string]any{"id": doc.id, "content": doc.content})
		if code != http.StatusOK {
			t.Fatalf("create %s: %d %v", doc.id, code, body)
		}
	}
	processAll(t, h)

	code, plain := doV5(t, h, "POST", "/ns/"+ns+"/search", map[string]any{"query": "alpha", "rerank": "order"})
	if code != http.StatusOK {
		t.Fatalf("plain order search: %d %v", code, plain)
	}
	plainResults := plain["results"].([]any)
	if len(plainResults) < 2 {
		t.Fatalf("need at least two hits, got %v", plainResults)
	}

	var calls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload struct {
			Messages []llmMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		user := ""
		if len(payload.Messages) > 1 {
			user = payload.Messages[1].Content
		}
		ids := []string{}
		for _, line := range strings.Split(user, "\n") {
			if strings.HasPrefix(line, "ID: ") {
				ids = append(ids, strings.TrimPrefix(line, "ID: "))
			}
		}
		for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
			ids[i], ids[j] = ids[j], ids[i]
		}
		encoded, _ := json.Marshal(ids)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": string(encoded)}}},
		})
	}))
	defer llm.Close()
	h.LLM = &llmConfig{BaseURL: llm.URL, Model: "stub", Client: llm.Client()}

	code, reranked := doV5(t, h, "POST", "/ns/"+ns+"/search", map[string]any{"query": "alpha", "rerank": "order"})
	if code != http.StatusOK {
		t.Fatalf("llm order search: %d %v", code, reranked)
	}
	got := reranked["results"].([]any)
	if len(got) != len(plainResults) {
		t.Fatalf("result count changed: %d vs %d", len(got), len(plainResults))
	}
	if hitID(got[0]) != hitID(plainResults[len(plainResults)-1]) {
		t.Fatalf("LLM order not applied: got first %q want %q", hitID(got[0]), hitID(plainResults[len(plainResults)-1]))
	}

	// A malformed model answer must leave the deterministic order untouched.
	badLLM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "not json"}}}})
	}))
	defer badLLM.Close()
	h.LLM = &llmConfig{BaseURL: badLLM.URL, Model: "stub", Client: badLLM.Client()}
	code, fallback := doV5(t, h, "POST", "/ns/"+ns+"/search", map[string]any{"query": "alpha", "rerank": "order"})
	if code != http.StatusOK {
		t.Fatalf("fallback search: %d %v", code, fallback)
	}
	fallbackResults := fallback["results"].([]any)
	for i := range plainResults {
		if hitID(fallbackResults[i]) != hitID(plainResults[i]) {
			t.Fatalf("fallback order differs at %d: %q vs %q", i, hitID(fallbackResults[i]), hitID(plainResults[i]))
		}
	}
}

func hitID(v any) string {
	m, _ := v.(map[string]any)
	id, _ := m["id"].(string)
	return id
}

func TestV5RelatedParentsAndChildren(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "retrieval_related"
	now := time.Now().UTC().Add(-time.Hour)
	vecs, err := bagProvider{}.GenerateEmbeddings([]string{"shared semantic content"})
	if err != nil {
		t.Fatal(err)
	}
	records := []db.MemoryRecord{
		{ID: "parent1", Namespace: ns, DocumentID: "d0", Content: "parent fact", Metadata: map[string]any{}, Vector: vecs[0], IsLatest: true, Version: 1, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "self1", Namespace: ns, DocumentID: "d1", Content: "self fact", Metadata: map[string]any{}, Vector: vecs[0], IsLatest: true, Version: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "child1", Namespace: ns, DocumentID: "d2", Content: "child fact", Metadata: map[string]any{}, Vector: vecs[0], IsLatest: true, Version: 1, CreatedAt: now.Add(2 * time.Hour), UpdatedAt: now.Add(2 * time.Hour)},
	}
	if err := h.Engine.DB.InsertMemories(records); err != nil {
		t.Fatal(err)
	}
	code, body := doV5(t, h, "GET", "/ns/"+ns+"/memories/self1?include=related", nil)
	if code != http.StatusOK {
		t.Fatalf("related: %d %v", code, body)
	}
	included, _ := body["included"].(map[string]any)
	related, _ := included["related"].(map[string]any)
	if related == nil {
		t.Fatalf("missing related block: %v", body)
	}
	parents, _ := related["parents"].([]any)
	children, _ := related["children"].([]any)
	if len(parents) != 1 || parents[0].(map[string]any)["id"] != "parent1" {
		t.Fatalf("unexpected parents: %v", parents)
	}
	if len(children) != 1 || children[0].(map[string]any)["id"] != "child1" {
		t.Fatalf("unexpected children: %v", children)
	}
}

func TestV5ProfileStaticWithLLM(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "retrieval_profile"
	createNamespace(t, h, ns)

	var calls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `["likes teal", "works on memory"]`}}},
		})
	}))
	defer llm.Close()
	h.LLM = &llmConfig{BaseURL: llm.URL, Model: "stub", Client: llm.Client()}

	for i := 0; i < 2; i++ {
		code, body := doV5(t, h, "POST", "/ns/"+ns+"/profile", map[string]any{})
		if code != http.StatusOK {
			t.Fatalf("profile: %d %v", code, body)
		}
		profile := body["profile"].(map[string]any)
		static, _ := profile["static"].([]any)
		if len(static) != 2 || static[0] != "likes teal" {
			t.Fatalf("unexpected static profile: %v", static)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("static profile should be cached, LLM calls=%d", calls.Load())
	}

	h.LLM = nil
	code, body := doV5(t, h, "POST", "/ns/"+ns+"/profile", map[string]any{
		"filter": map[string]any{"field": "suite", "operator": "eq", "value": "unmatched"},
	})
	if code != http.StatusOK {
		t.Fatalf("profile without llm: %d %v", code, body)
	}
	static := body["profile"].(map[string]any)["static"].([]any)
	if len(static) != 0 {
		t.Fatalf("static should be empty without LLM, got %v", static)
	}
}

func TestV5RelatedWireNeverNull(t *testing.T) {
	out := v5RelatedRecords("self", nil, nil, nil, 10)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"parents":[]`, `"children":[]`, `"siblings":[]`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("expected %s in %s", key, raw)
		}
	}
}

// Reranking must only see the candidates that can appear in the response;
// otherwise the prompt (and the synchronous latency) grows with the fetch
// window instead of the requested limit.
func TestV5SearchRerankRespectsLimit(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	ns := "retrieval_rerank_limit"
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("d%d", i)
		code, body := doV5(t, h, "POST", "/ns/"+ns+"/document", map[string]any{"id": id, "content": "alpha document " + id})
		if code != http.StatusOK {
			t.Fatalf("create %s: %d %v", id, code, body)
		}
	}
	processAll(t, h)

	var seen atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []llmMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		ids := []string{}
		for _, line := range strings.Split(payload.Messages[1].Content, "\n") {
			if strings.HasPrefix(line, "ID: ") {
				ids = append(ids, strings.TrimPrefix(line, "ID: "))
			}
		}
		seen.Store(int32(len(ids)))
		for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
			ids[i], ids[j] = ids[j], ids[i]
		}
		encoded, _ := json.Marshal(ids)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": string(encoded)}}},
		})
	}))
	defer llm.Close()
	h.LLM = &llmConfig{BaseURL: llm.URL, Model: "stub", Client: llm.Client()}

	code, body := doV5(t, h, "POST", "/ns/"+ns+"/search", map[string]any{"query": "alpha", "rerank": "order", "limit": 2})
	if code != http.StatusOK {
		t.Fatalf("search: %d %v", code, body)
	}
	if got := seen.Load(); got != 2 {
		t.Fatalf("rerank saw %d candidates, want 2", got)
	}
	if results := body["results"].([]any); len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
}
