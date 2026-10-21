package api

import (
	"fmt"
	"net/http"
	"testing"
)

func seedListData(t *testing.T, h *Handler) {
	t.Helper()
	doV5(t, h, "POST", "/ns/a/document", map[string]any{
		"content": "My favorite color is teal.", "id": "d1", "metadata": map[string]any{"suite": "pref"},
	})
	doV5(t, h, "POST", "/ns/a/document", map[string]any{
		"content": "My second document is about rust.", "id": "d2", "metadata": map[string]any{"suite": "other"},
	})
	processAll(t, h)
}

func TestV5ListDocuments(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "POST", "/ns/a/list/documents?limit=100", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	docs := body["documents"].([]any)
	if len(docs) != 2 {
		t.Fatalf("documents = %v", docs)
	}
	if len(body["chunks"].([]any)) != 0 || len(body["memories"].([]any)) != 0 {
		t.Errorf("unexpected chunks/memories in document listing")
	}
	if body["pagination"].(map[string]any)["totalItems"] != float64(2) {
		t.Errorf("pagination = %v", body["pagination"])
	}
	first := docs[0].(map[string]any)
	for _, field := range []string{"id", "title", "type", "summary", "metadata", "url", "system"} {
		if _, ok := first[field]; !ok {
			t.Errorf("document missing %q", field)
		}
	}
	if _, ok := first["content"]; ok {
		t.Errorf("list leaked content")
	}
	if first["url"] != nil {
		t.Errorf("url = %v, want null", first["url"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents", map[string]any{"limit": 5})
	if code != http.StatusBadRequest {
		t.Fatalf("body limit status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "unrecognized_keys" {
		t.Errorf("code = %v", item["code"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents?limit=101", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("limit 101 status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}

	_, page1 := doV5(t, h, "POST", "/ns/a/list/documents?limit=1&page=1", nil)
	code, page2 := doV5(t, h, "POST", "/ns/a/list/documents?limit=1&page=2", nil)
	if code != http.StatusOK {
		t.Fatalf("page 2 status = %d", code)
	}
	id1 := page1["documents"].([]any)[0].(map[string]any)["id"]
	id2 := page2["documents"].([]any)[0].(map[string]any)["id"]
	if id1 == id2 {
		t.Errorf("pages returned the same document")
	}
	if page2["pagination"].(map[string]any)["currentPage"] != float64(2) {
		t.Errorf("currentPage = %v", page2["pagination"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents?sort=createdAt&order=asc", nil)
	if code != http.StatusOK {
		t.Fatalf("sort status = %d", code)
	}
	asc := body["documents"].([]any)
	t1 := asc[0].(map[string]any)["system"].(map[string]any)["createdAt"].(string)
	t2 := asc[1].(map[string]any)["system"].(map[string]any)["createdAt"].(string)
	if t1 > t2 {
		t.Errorf("ascending order violated: %s > %s", t1, t2)
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents?sort=bogus", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("bogus sort status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "invalid_value" {
		t.Errorf("code = %v", item["code"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents", map[string]any{
		"filter": map[string]any{"field": "suite", "operator": "eq", "value": "pref"},
	})
	if code != http.StatusOK {
		t.Fatalf("filter status = %d", code)
	}
	if docs := body["documents"].([]any); len(docs) != 1 || docs[0].(map[string]any)["id"] != "d1" {
		t.Errorf("filtered documents = %v", docs)
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents", map[string]any{
		"filter": map[string]any{"field": "suite", "operator": "like", "value": "p"},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("like status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "invalid_value" {
		t.Errorf("code = %v", item["code"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents", map[string]any{
		"filter": map[string]any{"suite": map[string]any{"value": "pref"}},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("malformed filter status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "invalid_union" {
		t.Errorf("code = %v", item["code"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents", map[string]any{
		"filter": map[string]any{"field": "suite", "operator": "eq", "value": 123},
	})
	if code != http.StatusOK || len(body["documents"].([]any)) != 0 {
		t.Errorf("numeric filter = %d %v", code, body["documents"])
	}

	code, body = doV5(t, h, "POST", "/ns/missing/list/documents", nil)
	if code != http.StatusOK {
		t.Fatalf("unknown namespace status = %d", code)
	}
	if body["pagination"].(map[string]any)["totalItems"] != float64(0) {
		t.Errorf("unknown namespace total = %v", body["pagination"])
	}
}

func TestV5ListChunksAndMemories(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "POST", "/ns/a/list/chunks?limit=100", nil)
	if code != http.StatusOK {
		t.Fatalf("chunks status = %d", code)
	}
	chunks := body["chunks"].([]any)
	if len(chunks) == 0 {
		t.Fatalf("no chunks")
	}
	chunk := chunks[0].(map[string]any)
	for _, field := range []string{"documentId", "position", "content", "type", "metadata", "system"} {
		if _, ok := chunk[field]; !ok {
			t.Errorf("chunk missing %q", field)
		}
	}
	if len(body["documents"].([]any)) != 0 || len(body["memories"].([]any)) != 0 {
		t.Errorf("unexpected documents/memories")
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/memories?limit=100", nil)
	if code != http.StatusOK {
		t.Fatalf("memories status = %d", code)
	}
	mems := body["memories"].([]any)
	if len(mems) == 0 {
		t.Fatalf("no memories")
	}
	mem := mems[0].(map[string]any)
	if mem["isForgotten"] != false {
		t.Errorf("isForgotten = %v", mem["isForgotten"])
	}
	memID := mem["id"].(string)
	memText := mem["memory"].(string)

	// Forget it and verify the default listing hides it.
	code, body = doV5(t, h, "DELETE", "/ns/a/memories", map[string]any{"ids": []string{memID}})
	if code != http.StatusOK || body["count"] != float64(1) {
		t.Fatalf("forget = %d %v", code, body)
	}
	_, body = doV5(t, h, "POST", "/ns/a/list/memories?limit=100", nil)
	for _, raw := range body["memories"].([]any) {
		if raw.(map[string]any)["id"] == memID {
			t.Errorf("forgotten memory still listed")
		}
	}
	_, body = doV5(t, h, "POST", "/ns/a/list/memories?limit=100", map[string]any{
		"include": map[string]any{"forgotten": true},
	})
	found := false
	for _, raw := range body["memories"].([]any) {
		m := raw.(map[string]any)
		if m["id"] == memID {
			found = true
			if m["isForgotten"] != true {
				t.Errorf("isForgotten = %v", m["isForgotten"])
			}
		}
	}
	if !found {
		t.Errorf("forgotten memory missing with include.forgotten=true (text %q)", memText)
	}
}

func TestV5MemoryCounts(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "POST", "/ns/a/list/documents/memory-counts", map[string]any{"ids": []string{"d1"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	counts := body["counts"].([]any)
	if len(counts) != 1 || counts[0].(map[string]any)["memoryCount"].(float64) < 1 {
		t.Errorf("counts = %v", counts)
	}

	code, body = doV5(t, h, "POST", "/ns/a/list/documents/memory-counts", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("missing ids status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "invalid_type" {
		t.Errorf("code = %v", item["code"])
	}

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	code, body = doV5(t, h, "POST", "/ns/a/list/documents/memory-counts", map[string]any{"ids": ids})
	if code != http.StatusBadRequest {
		t.Fatalf("101 ids status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}
}

func TestV5GetMemory(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	_, doc := doV5(t, h, "GET", "/ns/a/document/d1?include=memories", nil)
	mems := doc["memories"].([]any)
	if len(mems) == 0 {
		t.Fatalf("no memories")
	}
	memID := mems[0].(map[string]any)["id"].(string)

	code, body := doV5(t, h, "GET", "/ns/a/memories/"+memID, nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body["memory"] == "" || body["isForgotten"] != false {
		t.Errorf("memory = %v", body)
	}

	code, _ = doV5(t, h, "GET", "/ns/a/memories/nope", nil)
	if code != http.StatusNotFound {
		t.Errorf("unknown memory status = %d", code)
	}

	code, body = doV5(t, h, "GET", "/ns/a/memories/"+memID+"?include=documents", nil)
	if code != http.StatusOK {
		t.Fatalf("include documents status = %d", code)
	}
	inc, _ := body["included"].(map[string]any)
	incDoc, _ := inc["document"].(map[string]any)
	if incDoc == nil || incDoc["id"] != "d1" {
		t.Errorf("included.document = %v", inc)
	}

	code, body = doV5(t, h, "GET", "/ns/a/memories/"+memID+"?include=related", nil)
	if code != http.StatusOK {
		t.Fatalf("include related status = %d", code)
	}
	inc, _ = body["included"].(map[string]any)
	related, _ := inc["related"].(map[string]any)
	if related == nil {
		t.Fatalf("missing related")
	}
	for _, key := range []string{"parents", "children", "siblings"} {
		if _, ok := related[key].([]any); !ok {
			t.Errorf("related.%s = %v", key, related[key])
		}
	}
}

func TestV5FilterFieldValidation(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "POST", "/ns/a/list/documents", map[string]any{
		"filter": map[string]any{"field": "bad field", "operator": "eq", "value": "x"},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d", code)
	}
	item := errorItem(t, body)
	if item["code"] != "invalid_value" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 2 || path[0] != "filter" || path[1] != "field" {
		t.Errorf("path = %v", path)
	}
}

func TestV5ListPageOverflow(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "POST", "/ns/a/list/documents?page=9223372036854775807&limit=100", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if len(body["documents"].([]any)) != 0 {
		t.Errorf("documents = %v", body["documents"])
	}
}
