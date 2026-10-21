package api

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestV5CreateAndReadDocument(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "My favorite color is teal.", "id": "d1"})
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if body["id"] != "d1" || body["status"] != "queued" {
		t.Fatalf("body = %v", body)
	}
	code, body = doV5(t, h, "GET", "/ns/a/document/d1", nil)
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	if body["system"].(map[string]any)["status"] != "queued" {
		t.Errorf("status = %v", body["system"])
	}

	// Identical re-ingestion must not requeue.
	processAll(t, h)
	code, body = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "My favorite color is teal.", "id": "d1"})
	if code != http.StatusOK {
		t.Fatalf("repost status = %d", code)
	}
	if body["status"] != "done" {
		t.Errorf("repost status = %v", body["status"])
	}
	_, doc := doV5(t, h, "GET", "/ns/a/document/d1?include=memories", nil)
	memCount := len(doc["memories"].([]any))
	if memCount == 0 {
		t.Fatalf("expected memories")
	}
	doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "My favorite color is teal.", "id": "d1"})
	_, doc = doV5(t, h, "GET", "/ns/a/document/d1?include=memories", nil)
	if len(doc["memories"].([]any)) != memCount {
		t.Errorf("identical re-ingestion duplicated memories")
	}

	// Appending different content keeps the old and the new marker.
	code, body = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "NEU-9921", "id": "d1"})
	if code != http.StatusOK || body["status"] != "queued" {
		t.Fatalf("append = %d %v", code, body)
	}
	processAll(t, h)
	_, doc = doV5(t, h, "GET", "/ns/a/document/d1", nil)
	content, _ := doc["content"].(string)
	if !strings.Contains(content, "teal") || !strings.Contains(content, "NEU-9921") || !strings.Contains(content, "\n") {
		t.Errorf("appended content = %q", content)
	}
}

func TestV5DocumentValidation(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "x", "id": strings.Repeat("a", 256)})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d", code)
	}
	item := errorItem(t, body)
	if item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "id" {
		t.Errorf("path = %v", path)
	}

	code, body = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": 123})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d", code)
	}
	item = errorItem(t, body)
	if item["code"] != "invalid_type" {
		t.Errorf("code = %v", item["code"])
	}
	if msg, _ := item["message"].(string); !strings.Contains(strings.ToLower(msg), "expected string") {
		t.Errorf("message = %q", msg)
	}

	code, _ = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "x", "metadata": map[string]any{"suite": 123}})
	if code != http.StatusOK {
		t.Fatalf("numeric metadata status = %d", code)
	}

	code, body = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "x", "date": "2025-01-15"})
	if code != http.StatusOK {
		t.Fatalf("date status = %d", code)
	}
	_, doc := doV5(t, h, "GET", "/ns/a/document/"+body["id"].(string), nil)
	meta := doc["metadata"].(map[string]any)
	tc, _ := meta["temporalContext"].(map[string]any)
	if tc["documentDate"] != "2025-01-15" {
		t.Errorf("temporalContext = %v", meta)
	}

	code, body = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "x", "date": "15.01.2025"})
	if code != http.StatusBadRequest {
		t.Fatalf("bad date status = %d", code)
	}
	item = errorItem(t, body)
	if item["code"] != "invalid_value" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "date" {
		t.Errorf("path = %v", path)
	}
}

func TestV5Batch(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "POST", "/ns/a/document/batch", map[string]any{
		"documents": []any{
			map[string]any{"content": "first batch document content", "id": "b1"},
			map[string]any{"content": "second batch document content", "id": "b2"},
		},
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body["count"] != float64(2) || body["failed"] != float64(0) {
		t.Errorf("body = %v", body)
	}
	if len(body["results"].([]any)) != 2 {
		t.Errorf("results = %v", body["results"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/document/batch", map[string]any{
		"documents": []any{
			map[string]any{"content": "duplicate one", "id": "dup"},
			map[string]any{"content": "duplicate two", "id": "dup"},
		},
	})
	if code != http.StatusOK || body["count"] != float64(2) || body["failed"] != float64(0) {
		t.Fatalf("duplicate batch = %d %v", code, body)
	}

	code, body = doV5(t, h, "POST", "/ns/a/document/batch", map[string]any{
		"documents": []any{map[string]any{"content": "x", "dreaming": "instant"}},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("item dreaming status = %d", code)
	}
	item := errorItem(t, body)
	if item["code"] != "unrecognized_keys" {
		t.Errorf("code = %v", item["code"])
	}
	keys, _ := item["keys"].([]any)
	if len(keys) != 1 || keys[0] != "dreaming" {
		t.Errorf("keys = %v", keys)
	}

	docs := make([]any, 601)
	for i := range docs {
		docs[i] = map[string]any{"content": "bulk document"}
	}
	code, body = doV5(t, h, "POST", "/ns/a/document/batch", map[string]any{"documents": docs})
	if code != http.StatusBadRequest {
		t.Fatalf("601 status = %d", code)
	}
	item = errorItem(t, body)
	if item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "documents" {
		t.Errorf("path = %v", path)
	}
}

func TestV5GetDocumentIncluded(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	createNamespace(t, h, "a")
	doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "My favorite color is teal.", "id": "d1"})
	processAll(t, h)

	code, body := doV5(t, h, "GET", "/ns/a/document/d1?include=chunks,memories", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	chunks := body["chunks"].([]any)
	if len(chunks) == 0 {
		t.Fatalf("no chunks")
	}
	chunk := chunks[0].(map[string]any)
	if !regexp.MustCompile(`^[0-9a-f]{24}$`).MatchString(chunk["id"].(string)) {
		t.Errorf("chunk id = %v", chunk["id"])
	}
	if chunk["type"] != "text" {
		t.Errorf("chunk type = %v", chunk["type"])
	}
	for _, field := range []string{"position", "content", "metadata", "system"} {
		if _, ok := chunk[field]; !ok {
			t.Errorf("chunk missing %q", field)
		}
	}
	if len(body["memories"].([]any)) == 0 {
		t.Errorf("no memories")
	}

	_, body = doV5(t, h, "GET", "/ns/a/document/d1", nil)
	if len(body["chunks"].([]any)) != 0 || len(body["memories"].([]any)) != 0 {
		t.Errorf("unrequested includes are not empty")
	}
	system := body["system"].(map[string]any)
	if _, ok := system["filepath"]; ok {
		t.Errorf("system leaks filepath")
	}
	if system["status"] != "done" {
		t.Errorf("status = %v", system["status"])
	}

	code, _ = doV5(t, h, "GET", "/ns/b/document/d1", nil)
	if code != http.StatusNotFound {
		t.Errorf("foreign namespace status = %d", code)
	}
}

func TestV5PatchDocument(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	createNamespace(t, h, "a")
	doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "My favorite color is teal.", "id": "d1"})
	processAll(t, h)

	code, body := doV5(t, h, "PATCH", "/ns/a/document/d1", map[string]any{"content": "neu"})
	if code != http.StatusOK || body["status"] != "queued" {
		t.Fatalf("patch = %d %v", code, body)
	}
	processAll(t, h)
	_, doc := doV5(t, h, "GET", "/ns/a/document/d1?include=chunks,memories", nil)
	if doc["content"] != "neu" {
		t.Errorf("content = %v", doc["content"])
	}
	if len(doc["memories"].([]any)) != 0 {
		t.Errorf("old memories were not replaced")
	}

	code, body = doV5(t, h, "PATCH", "/ns/a/document/d1", map[string]any{"metadata": map[string]any{"a": 1}})
	if code != http.StatusOK {
		t.Fatalf("metadata patch = %d", code)
	}
	_, doc = doV5(t, h, "GET", "/ns/a/document/d1", nil)
	if doc["system"].(map[string]any)["status"] != "done" {
		t.Errorf("metadata-only patch requeued the document")
	}
	meta := doc["metadata"].(map[string]any)
	if meta["a"] != float64(1) || len(meta) != 1 {
		t.Errorf("metadata = %v", meta)
	}

	code, body = doV5(t, h, "PATCH", "/ns/a/document/d1", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("empty patch = %d", code)
	}
	item := errorItem(t, body)
	if item["message"] != "At least one document field must be provided" {
		t.Errorf("message = %v", item["message"])
	}

	code, body = doV5(t, h, "PATCH", "/ns/a/document/d1", map[string]any{"group": map[string]any{"x": "y"}})
	if code != http.StatusBadRequest {
		t.Fatalf("group patch = %d", code)
	}
	item = errorItem(t, body)
	if item["message"] != "group only applies on re-ingestion; this update does not replace the document content" {
		t.Errorf("message = %v", item["message"])
	}

	code, _ = doV5(t, h, "PATCH", "/ns/a/document/nope", map[string]any{"metadata": map[string]any{"a": 1}})
	if code != http.StatusNotFound {
		t.Errorf("unknown patch status = %d", code)
	}
}

func TestV5BatchInvalidItem(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "POST", "/ns/a/document/batch", map[string]any{
		"documents": []any{
			map[string]any{"content": "valid batch document content"},
			map[string]any{"content": 42},
		},
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if body["count"] != float64(1) || body["failed"] != float64(1) {
		t.Errorf("body = %v", body)
	}
}

func TestV5AppendTooBig(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	full := strings.Repeat("a", 1000000)
	code, _ := doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": full, "id": "big"})
	if code != http.StatusOK {
		t.Fatalf("seed status = %d", code)
	}
	code, body := doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "b", "id": "big"})
	if code != http.StatusBadRequest {
		t.Fatalf("append status = %d", code)
	}
	item := errorItem(t, body)
	if item["code"] != "too_big" || errorPath(t, item)[0] != "content" {
		t.Errorf("error = %v", item)
	}
}

func TestV5NullMetadataAndGroup(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "My favorite color is teal.", "id": "d1"})

	for _, body := range []map[string]any{
		{"content": "x", "metadata": nil},
		{"content": "x", "group": nil},
	} {
		code, out := doV5(t, h, "POST", "/ns/a/document", body)
		if code != http.StatusBadRequest {
			t.Fatalf("status = %d for %v", code, body)
		}
		item := errorItem(t, out)
		if item["code"] != "invalid_type" {
			t.Errorf("code = %v", item["code"])
		}
	}

	code, out := doV5(t, h, "PATCH", "/ns/a/document/d1", map[string]any{"metadata": nil})
	if code != http.StatusBadRequest {
		t.Fatalf("patch status = %d", code)
	}
	if item := errorItem(t, out); item["code"] != "invalid_type" {
		t.Errorf("patch code = %v", item["code"])
	}
}
