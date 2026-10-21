package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func errorItem(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	errs, ok := body["error"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected error array, got %v", body)
	}
	item, ok := errs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %v", errs[0])
	}
	return item
}

func errorPath(t *testing.T, item map[string]any) []string {
	t.Helper()
	raw, ok := item["path"].([]any)
	if !ok {
		t.Fatalf("expected path array, got %v", item)
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i], _ = v.(string)
	}
	return out
}

func TestV5RouteNotFound(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "GET", "/ns/x/profile", nil)
	if code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", code)
	}
	if body["error"] != "Not found" {
		t.Errorf("error = %v", body["error"])
	}
	if body["message"] != "No route matches GET /ns/x/profile" {
		t.Errorf("message = %v", body["message"])
	}
	if body["documentation"] != "https://api.supermemory.ai/openapi.json" {
		t.Errorf("documentation = %v", body["documentation"])
	}
}

func TestV5Unauthorized(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	h.apiKey = "secret"

	req := httptest.NewRequest("POST", "/ns/a/search", strings.NewReader(`{"query":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "Unauthorized" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestV5ContentType(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	cases := []struct {
		name string
		body string
		ct   string
	}{
		{"text plain", `{"content":"x"}`, "text/plain"},
		{"missing", `{"content":"x"}`, ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", "/ns/a/document", strings.NewReader(tc.body))
		if tc.ct != "" {
			req.Header.Set("Content-Type", tc.ct)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", tc.name, rr.Code)
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s: decode: %v", tc.name, err)
		}
		item := errorItem(t, body)
		if item["code"] != "invalid_type" {
			t.Errorf("%s: code = %v", tc.name, item["code"])
		}
		if path := errorPath(t, item); len(path) != 1 || path[0] != "content" {
			t.Errorf("%s: path = %v", tc.name, path)
		}
	}
}

func TestV5RequiredContentAndKeys(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "POST", "/ns/a/document", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	item := errorItem(t, body)
	if item["code"] != "invalid_type" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "content" {
		t.Errorf("path = %v", path)
	}
	if body["data"] == nil {
		t.Errorf("expected data to mirror the request body")
	}

	code, body = doV5(t, h, "POST", "/ns/a/document", map[string]any{"content": "x", "foo": 1})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	item = errorItem(t, body)
	if item["code"] != "unrecognized_keys" {
		t.Errorf("code = %v", item["code"])
	}
	keys, _ := item["keys"].([]any)
	if len(keys) != 1 || keys[0] != "foo" {
		t.Errorf("keys = %v", keys)
	}

	code, body = doV5(t, h, "POST", "/ns/a/search?limit=5", map[string]any{"query": "x"})
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	item = errorItem(t, body)
	if item["code"] != "unrecognized_keys" {
		t.Errorf("code = %v", item["code"])
	}
	keys, _ = item["keys"].([]any)
	if len(keys) != 1 || keys[0] != "limit" {
		t.Errorf("keys = %v", keys)
	}
}

func TestV5PaginationValidation(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	code, body := doV5(t, h, "GET", "/namespaces?limit=101", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", code)
	}
	item := errorItem(t, body)
	if item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}
	if path := errorPath(t, item); len(path) != 1 || path[0] != "limit" {
		t.Errorf("path = %v", path)
	}

	for _, ns := range []string{"p1", "p2", "p3"} {
		createNamespace(t, h, ns)
	}
	code, body = doV5(t, h, "GET", "/namespaces?limit=1&page=2", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	pg, _ := body["pagination"].(map[string]any)
	if pg["currentPage"] != float64(2) || pg["limit"] != float64(1) {
		t.Errorf("pagination = %v", pg)
	}

	// GET /ns is an alias for the same listing.
	_, alias := doV5(t, h, "GET", "/ns", nil)
	aliasPage, _ := alias["pagination"].(map[string]any)
	if aliasPage["totalItems"] != pg["totalItems"] {
		t.Errorf("alias totalItems = %v, want %v", aliasPage["totalItems"], pg["totalItems"])
	}
}
