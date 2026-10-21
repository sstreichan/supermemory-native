package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestV5NamespaceListingAndImplicitCreate(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	for _, ns := range []string{"alpha", "beta", "gamma"} {
		createNamespace(t, h, ns)
	}

	code, body := doV5(t, h, "GET", "/namespaces", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	list, ok := body["namespaces"].([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("namespaces = %v", body["namespaces"])
	}
	var last string
	for _, raw := range list {
		ns := raw.(map[string]any)
		for _, field := range []string{"id", "namespace", "description", "documentCount", "memoryCount", "system"} {
			if _, ok := ns[field]; !ok {
				t.Errorf("namespace element missing %q: %v", field, ns)
			}
		}
		system := ns["system"].(map[string]any)
		updated, _ := system["updatedAt"].(string)
		if last != "" && updated > last {
			t.Errorf("namespaces are not sorted by updatedAt desc: %q after %q", updated, last)
		}
		last = updated
	}

	code, body = doV5(t, h, "GET", "/ns/alpha", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if body["supportingContext"] != "" {
		t.Errorf("supportingContext = %v, want empty", body["supportingContext"])
	}

	code, _ = doV5(t, h, "GET", "/ns/does-not-exist", nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown namespace status = %d, want 404", code)
	}

	code, body = doV5(t, h, "POST", "/ns/implicit/document", map[string]any{"content": "implicit namespace document"})
	if code != http.StatusOK {
		t.Fatalf("create status = %d, body %v", code, body)
	}
	code, _ = doV5(t, h, "GET", "/ns/implicit", nil)
	if code != http.StatusOK {
		t.Fatalf("implicit namespace not created: %d", code)
	}
}

func TestV5NamespaceLifecycle(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()

	createNamespace(t, h, "life")

	code, body := doV5(t, h, "PATCH", "/ns/life", map[string]any{"supportingContext": "ctx"})
	if code != http.StatusOK {
		t.Fatalf("patch status = %d", code)
	}
	if body["supportingContext"] != "ctx" {
		t.Errorf("supportingContext = %v", body["supportingContext"])
	}

	code, body = doV5(t, h, "PATCH", "/ns/life", map[string]any{"supportingContext": nil})
	if code != http.StatusOK {
		t.Fatalf("patch null status = %d", code)
	}
	if body["supportingContext"] != "" {
		t.Errorf("supportingContext = %v, want empty", body["supportingContext"])
	}

	code, body = doV5(t, h, "PATCH", "/ns/life", map[string]any{})
	if code != http.StatusBadRequest {
		t.Fatalf("empty patch status = %d", code)
	}
	item := errorItem(t, body)
	if path := errorPath(t, item); len(path) != 1 || path[0] != "supportingContext" {
		t.Errorf("path = %v", path)
	}

	long := strings.Repeat("x", 1501)
	code, body = doV5(t, h, "PATCH", "/ns/life", map[string]any{"supportingContext": long})
	if code != http.StatusBadRequest {
		t.Fatalf("long patch status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}

	code, body = doV5(t, h, "DELETE", "/ns/life", nil)
	if code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
	if body["status"] != "deleted" || body["namespace"] != "life" {
		t.Errorf("delete body = %v", body)
	}
	if _, ok := body["deletedDocumentsCount"]; !ok {
		t.Errorf("missing deletedDocumentsCount")
	}
	if _, ok := body["deletedMemoriesCount"]; !ok {
		t.Errorf("missing deletedMemoriesCount")
	}
	code, _ = doV5(t, h, "GET", "/ns/life", nil)
	if code != http.StatusNotFound {
		t.Errorf("namespace still readable: %d", code)
	}

	code, body = doV5(t, h, "DELETE", "/ns/sm_project_default", nil)
	if code != http.StatusConflict {
		t.Fatalf("protected delete status = %d", code)
	}
	if body["error"] == "" || body["error"] == nil {
		t.Errorf("expected error message")
	}
}

func TestV5ListNamespacesPageOverflow(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	createNamespace(t, h, "a")

	code, body := doV5(t, h, "GET", "/namespaces?page=9223372036854775807&limit=100", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %v", code, body)
	}
	if len(body["namespaces"].([]any)) != 0 {
		t.Errorf("namespaces = %v", body["namespaces"])
	}
	if body["pagination"].(map[string]any)["totalItems"] != float64(1) {
		t.Errorf("pagination = %v", body["pagination"])
	}
}
