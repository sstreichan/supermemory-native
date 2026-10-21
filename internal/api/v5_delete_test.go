package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestV5DeleteDocuments(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "DELETE", "/ns/a/document", map[string]any{"ids": []string{"d1"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body["count"] != float64(1) || len(body["errors"].([]any)) != 0 {
		t.Errorf("body = %v", body)
	}
	code, _ = doV5(t, h, "GET", "/ns/a/document/d1", nil)
	if code != http.StatusNotFound {
		t.Errorf("document still readable")
	}
	_, counts := doV5(t, h, "POST", "/ns/a/list/documents/memory-counts", map[string]any{"ids": []string{"d1"}})
	if counts["counts"].([]any)[0].(map[string]any)["memoryCount"] != float64(0) {
		t.Errorf("memories survived: %v", counts)
	}

	// A queued document is still processing and cannot be deleted.
	doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "queued document content", "id": "q1"})
	code, body = doV5(t, h, "DELETE", "/ns/a/document", map[string]any{"ids": []string{"q1"}})
	if code != http.StatusOK {
		t.Fatalf("queued delete status = %d", code)
	}
	if body["count"] != float64(0) {
		t.Errorf("count = %v", body["count"])
	}
	errs := body["errors"].([]any)
	if len(errs) != 1 || errs[0].(map[string]any)["error"] != "Document is still processing" {
		t.Errorf("errors = %v", errs)
	}

	code, body = doV5(t, h, "DELETE", "/ns/a/document", map[string]any{"ids": []string{"nope"}})
	if code != http.StatusOK || len(body["errors"].([]any)) != 1 {
		t.Fatalf("missing delete = %d %v", code, body)
	}
	if body["errors"].([]any)[0].(map[string]any)["error"] != "Document not found" {
		t.Errorf("error = %v", body["errors"])
	}

	code, body = doV5(t, h, "DELETE", "/ns/a/document", map[string]any{"ids": []string{}})
	if code != http.StatusBadRequest {
		t.Fatalf("empty ids status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "too_small" {
		t.Errorf("code = %v", item["code"])
	}

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	code, body = doV5(t, h, "DELETE", "/ns/a/document", map[string]any{"ids": ids})
	if code != http.StatusBadRequest {
		t.Fatalf("101 ids status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}
}

func TestV5ForgetAndSemantic(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	_, doc := doV5(t, h, "GET", "/ns/a/document/d1?include=memories", nil)
	mems := doc["memories"].([]any)
	if len(mems) == 0 {
		t.Fatalf("no memories")
	}
	memID := mems[0].(map[string]any)["id"].(string)

	code, body := doV5(t, h, "DELETE", "/ns/a/memories", map[string]any{"ids": []string{memID}})
	if code != http.StatusOK || body["count"] != float64(1) {
		t.Fatalf("forget = %d %v", code, body)
	}
	matches := body["matches"].([]any)
	if len(matches) != 1 || matches[0].(map[string]any)["id"] != memID || matches[0].(map[string]any)["memory"] == "" {
		t.Errorf("matches = %v", matches)
	}

	_, body = doV5(t, h, "POST", "/ns/a/search", map[string]any{"query": "teal", "searchMode": "memories"})
	for _, raw := range body["results"].([]any) {
		if raw.(map[string]any)["id"] == memID {
			t.Errorf("forgotten memory still searchable")
		}
	}
	_, body = doV5(t, h, "POST", "/ns/a/search", map[string]any{"query": "teal", "searchMode": "chunks"})
	if len(body["results"].([]any)) == 0 {
		t.Errorf("chunk search lost results after forgetting a memory")
	}

	code, body = doV5(t, h, "DELETE", "/ns/a/memories", map[string]any{"ids": []string{memID}})
	if code != http.StatusOK || len(body["errors"].([]any)) != 1 {
		t.Fatalf("second forget = %d %v", code, body)
	}
	if body["errors"].([]any)[0].(map[string]any)["error"] != "Memory not found" {
		t.Errorf("error = %v", body["errors"])
	}

	// Dry run reports matches without changing state.
	code, body = doV5(t, h, "DELETE", "/ns/a/memories/semantic", map[string]any{"query": "rust", "dryRun": true})
	if code != http.StatusOK {
		t.Fatalf("semantic dry run status = %d", code)
	}
	if len(body["matches"].([]any)) == 0 {
		t.Fatalf("dry run matches = %v", body)
	}
	_, list := doV5(t, h, "POST", "/ns/a/list/memories?limit=100", nil)
	before := len(list["memories"].([]any))
	if before == 0 {
		t.Fatalf("dry run removed memories")
	}

	code, body = doV5(t, h, "DELETE", "/ns/a/memories/semantic", map[string]any{"query": "rust", "dryRun": false})
	if code != http.StatusOK {
		t.Fatalf("semantic status = %d", code)
	}
	if body["count"] != float64(len(body["matches"].([]any))) {
		t.Errorf("count = %v, matches = %v", body["count"], body["matches"])
	}
	_, list = doV5(t, h, "POST", "/ns/a/list/memories?limit=100", nil)
	if len(list["memories"].([]any)) != 0 {
		t.Errorf("memories survived semantic forget: %v", list["memories"])
	}

	code, body = doV5(t, h, "DELETE", "/ns/a/memories/semantic", map[string]any{"query": "rust"})
	if code != http.StatusBadRequest {
		t.Fatalf("missing dryRun status = %d", code)
	}
	item := errorItem(t, body)
	if item["code"] != "invalid_type" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "dryRun" {
		t.Errorf("path = %v", path)
	}
}

func TestV5NamespaceMove(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	doV5(t, h, "POST", "/ns/src/document", map[string]any{"content": "My favorite color is teal.", "id": "m1"})
	processAll(t, h)

	code, body := doV5(t, h, "DELETE", "/ns/src?moveTo=dst", nil)
	if code != http.StatusOK {
		t.Fatalf("move status = %d", code)
	}
	if body["deletedDocumentsCount"] != float64(1) {
		t.Errorf("body = %v", body)
	}
	code, _ = doV5(t, h, "GET", "/ns/src", nil)
	if code != http.StatusNotFound {
		t.Errorf("source namespace still exists")
	}
	code, body = doV5(t, h, "GET", "/ns/dst/document/m1", nil)
	if code != http.StatusOK {
		t.Fatalf("moved document unreadable: %d", code)
	}
	if !strings.Contains(body["content"].(string), "teal") {
		t.Errorf("content = %v", body["content"])
	}
}

func TestV5MoveToProtectedNamespace(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	doV5(t, h, "POST", "/ns/src/document", map[string]any{"content": "My favorite color is teal.", "id": "m1"})
	code, body := doV5(t, h, "DELETE", "/ns/src?moveTo=sm_project_default", nil)
	if code != http.StatusConflict {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if body["error"] == "" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestV5MoveReplacesTargetDocument(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	doV5(t, h, "POST", "/ns/src/document", map[string]any{"content": "source marker content long enough.", "id": "x"})
	doV5(t, h, "POST", "/ns/dst/document", map[string]any{"content": "target marker content long enough.", "id": "x"})
	processAll(t, h)

	code, body := doV5(t, h, "DELETE", "/ns/src?moveTo=dst", nil)
	if code != http.StatusOK {
		t.Fatalf("move status = %d, body %v", code, body)
	}
	code, doc := doV5(t, h, "GET", "/ns/dst/document/x?include=chunks,memories", nil)
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	if !strings.Contains(doc["content"].(string), "source marker") {
		t.Errorf("content = %v", doc["content"])
	}
	for _, raw := range doc["chunks"].([]any) {
		if strings.Contains(raw.(map[string]any)["content"].(string), "target marker") {
			t.Errorf("stale target chunk survived: %v", raw)
		}
	}
	for _, raw := range doc["memories"].([]any) {
		if strings.Contains(raw.(map[string]any)["memory"].(string), "target marker") {
			t.Errorf("stale target memory survived: %v", raw)
		}
	}
}

func TestV5DeleteNamespaceRemovesVaultFiles(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	if _, err := h.Engine.DB.EnsureNamespace("vaultns"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if _, err := h.Engine.AddMemory("legacy vault memory that is long enough", "vaultns"); err != nil {
		t.Fatalf("add legacy memory: %v", err)
	}

	code, body := doV5(t, h, "DELETE", "/ns/vaultns", nil)
	if code != http.StatusOK {
		t.Fatalf("delete status = %d, body %v", code, body)
	}
	if err := h.Engine.SyncVault(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	_, list := doV5(t, h, "POST", "/ns/vaultns/list/memories?limit=100", nil)
	if len(list["memories"].([]any)) != 0 {
		t.Errorf("memories resurrected after namespace delete: %v", list["memories"])
	}
}
