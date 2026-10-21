package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func searchV5(t *testing.T, h *Handler, body map[string]any) map[string]any {
	t.Helper()
	code, out := doV5(t, h, "POST", "/ns/a/search", body)
	if code != 200 {
		t.Fatalf("search status = %d, body %v", code, out)
	}
	return out
}

func TestV5Search(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	body := searchV5(t, h, map[string]any{"query": "teal"})
	results := body["results"].([]any)
	if len(results) == 0 {
		t.Fatalf("no results")
	}
	first := results[0].(map[string]any)
	text, _ := first["memory"].(string)
	if text == "" {
		text, _ = first["chunk"].(string)
	}
	if !strings.Contains(strings.ToLower(text), "teal") {
		t.Errorf("top hit = %v", first)
	}
	sim := first["similarity"].(float64)
	if sim < 0 || sim > 1 {
		t.Errorf("similarity = %v", sim)
	}
	if st, ok := body["searchTime"].(float64); !ok || st < 0 {
		t.Errorf("searchTime = %v", body["searchTime"])
	}

	code, out := doV5(t, h, "POST", "/ns/a/search", map[string]any{"query": ""})
	if code != 400 {
		t.Fatalf("empty query status = %d", code)
	}
	item := errorItem(t, out)
	if item["code"] != "too_small" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "query" {
		t.Errorf("path = %v", path)
	}
	if item["minimum"] != float64(1) {
		t.Errorf("minimum = %v", item["minimum"])
	}

	code, out = doV5(t, h, "POST", "/ns/a/search", map[string]any{"query": "teal", "limit": 101})
	if code != 400 {
		t.Fatalf("limit 101 status = %d", code)
	}
	if item := errorItem(t, out); item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}

	// Chunk-only hits never carry a memory field.
	body = searchV5(t, h, map[string]any{"query": "teal", "searchMode": "chunks"})
	for _, raw := range body["results"].([]any) {
		hit := raw.(map[string]any)
		if _, ok := hit["memory"]; ok {
			t.Errorf("chunk mode returned memory: %v", hit)
		}
		if _, ok := hit["chunk"]; !ok {
			t.Errorf("chunk mode hit without chunk: %v", hit)
		}
	}

	body = searchV5(t, h, map[string]any{"query": "teal", "searchMode": "memories"})
	for _, raw := range body["results"].([]any) {
		hit := raw.(map[string]any)
		if _, ok := hit["chunk"]; ok {
			t.Errorf("memory mode returned chunk: %v", hit)
		}
		if _, ok := hit["memory"]; !ok {
			t.Errorf("memory mode hit without memory: %v", hit)
		}
	}

	body = searchV5(t, h, map[string]any{"query": "teal", "searchMode": "hybrid"})
	var last float64 = 1
	for _, raw := range body["results"].([]any) {
		hit := raw.(map[string]any)
		sim := hit["similarity"].(float64)
		if sim > last {
			t.Errorf("hybrid results are not sorted: %v > %v", sim, last)
		}
		last = sim
	}

	body = searchV5(t, h, map[string]any{"query": "teal", "threshold": 0.99})
	if len(body["results"].([]any)) != 0 {
		t.Errorf("threshold 0.99 returned %v", body["results"])
	}

	code, out = doV5(t, h, "POST", "/ns/a/search", map[string]any{"query": "teal", "rerank": "bogus"})
	if code != 400 {
		t.Fatalf("rerank status = %d", code)
	}
	if item := errorItem(t, out); item["code"] != "invalid_value" {
		t.Errorf("code = %v", item["code"])
	}

	body = searchV5(t, h, map[string]any{"query": "teal", "rerank": "aggregate"})
	agg := body["results"].([]any)[0].(map[string]any)
	if !strings.HasPrefix(agg["id"].(string), "aggregated_0_") {
		t.Errorf("aggregate id = %v", agg["id"])
	}
	if len(body["results"].([]any)) > 1 {
		next := body["results"].([]any)[1].(map[string]any)
		if agg["similarity"] != next["similarity"] {
			t.Errorf("aggregate similarity = %v, top = %v", agg["similarity"], next["similarity"])
		}
	}

	body = searchV5(t, h, map[string]any{"query": "teal", "include": map[string]any{"documents": true}})
	inc, _ := body["results"].([]any)[0].(map[string]any)["included"].(map[string]any)
	doc, _ := inc["document"].(map[string]any)
	if doc == nil || doc["id"] == "" {
		t.Errorf("included.document = %v", inc)
	}

	body = searchV5(t, h, map[string]any{"query": "teal", "include": map[string]any{"related": true}})
	inc, _ = body["results"].([]any)[0].(map[string]any)["included"].(map[string]any)
	related, _ := inc["related"].(map[string]any)
	if related == nil {
		t.Fatalf("missing related")
	}
	for _, key := range []string{"parents", "children", "siblings"} {
		if _, ok := related[key].([]any); !ok {
			t.Errorf("related.%s = %v", key, related[key])
		}
	}

	body = searchV5(t, h, map[string]any{
		"query":  "teal",
		"filter": map[string]any{"field": "suite", "operator": "eq", "value": "pref"},
	})
	for _, raw := range body["results"].([]any) {
		meta := raw.(map[string]any)["metadata"].(map[string]any)
		if meta["suite"] != "pref" {
			t.Errorf("filter leaked hit %v", raw)
		}
	}

	code, out = doV5(t, h, "POST", "/ns/a/search?limit=5", map[string]any{"query": "teal"})
	if code != 400 {
		t.Fatalf("query param status = %d", code)
	}
	if item := errorItem(t, out); item["code"] != "unrecognized_keys" {
		t.Errorf("code = %v", item["code"])
	}

	code, out = doV5(t, h, "POST", "/ns/other/search", map[string]any{"query": "teal"})
	if code != 200 || len(out["results"].([]any)) != 0 {
		t.Errorf("foreign namespace = %d %v", code, out["results"])
	}
}

func TestV5SearchQueryTooBig(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "POST", "/ns/a/search", map[string]any{
		"query": strings.Repeat("a", 1000001),
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d", code)
	}
	item := errorItem(t, body)
	if item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "query" {
		t.Errorf("path = %v", path)
	}
}

func TestV5SearchFilterBeforeLimit(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	for i := 0; i < 12; i++ {
		doV5(t, h, "POST", "/ns/f/document", map[string]any{
			"content": "alpha alpha alpha alpha alpha alpha.",
			"id":      fmt.Sprintf("hot-%d", i), "metadata": map[string]any{"suite": "other"},
		})
		doV5(t, h, "POST", "/ns/f/document", map[string]any{
			"content": "alpha beta gamma delta epsilon zeta.",
			"id":      fmt.Sprintf("pref-%d", i), "metadata": map[string]any{"suite": "pref"},
		})
	}
	processAll(t, h)

	code, out := doV5(t, h, "POST", "/ns/f/search", map[string]any{
		"query": "alpha", "limit": 10,
		"filter": map[string]any{"field": "suite", "operator": "eq", "value": "pref"},
	})
	if code != 200 {
		t.Fatalf("status = %d", code)
	}
	results := out["results"].([]any)
	if len(results) != 10 {
		t.Fatalf("results = %d, want 10", len(results))
	}
	for _, raw := range results {
		meta := raw.(map[string]any)["metadata"].(map[string]any)
		if meta["suite"] != "pref" {
			t.Errorf("unfiltered hit: %v", meta)
		}
	}
}
