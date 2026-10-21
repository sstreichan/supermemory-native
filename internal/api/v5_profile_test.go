package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestV5Profile(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "POST", "/ns/a/profile", map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	profile := body["profile"].(map[string]any)
	if len(profile["static"].([]any)) != 0 {
		t.Errorf("static = %v", profile["static"])
	}
	dynamic := profile["dynamic"].([]any)
	if len(dynamic) == 0 {
		t.Fatalf("dynamic profile is empty")
	}
	entry := dynamic[0].(map[string]any)
	mem, _ := entry["memory"].(string)
	if !strings.HasPrefix(mem, "[") || len(mem) < len("[2006-01-02] ") {
		t.Errorf("memory prefix = %q", mem)
	}
	if _, ok := entry["id"]; !ok {
		t.Errorf("entry missing id")
	}
	buckets := profile["buckets"].(map[string]any)
	if _, ok := buckets["preferences"]; !ok {
		t.Errorf("missing preferences bucket")
	}
	if _, ok := buckets["tech_stack"]; !ok {
		t.Errorf("missing tech_stack bucket")
	}

	_, body = doV5(t, h, "POST", "/ns/a/profile", map[string]any{"buckets": []string{"preferences"}})
	profile = body["profile"].(map[string]any)
	if len(profile["buckets"].(map[string]any)) != 1 {
		t.Errorf("buckets = %v", profile["buckets"])
	}
	if len(profile["dynamic"].([]any)) == 0 {
		t.Errorf("dynamic disappeared with bucket filter")
	}

	_, body = doV5(t, h, "POST", "/ns/a/profile", map[string]any{"buckets": []string{"unbekannt"}})
	if len(body["profile"].(map[string]any)["buckets"].(map[string]any)) != 0 {
		t.Errorf("unknown bucket = %v", body["profile"])
	}

	code, body = doV5(t, h, "POST", "/ns/a/profile", map[string]any{"buckets": []any{1}})
	if code != http.StatusBadRequest {
		t.Fatalf("numeric bucket status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "invalid_type" {
		t.Errorf("code = %v", item["code"])
	}

	_, body = doV5(t, h, "POST", "/ns/a/profile", map[string]any{
		"filter": map[string]any{"field": "suite", "operator": "eq", "value": "pref"},
	})
	for _, raw := range body["profile"].(map[string]any)["dynamic"].([]any) {
		mem := raw.(map[string]any)["memory"].(string)
		if !strings.Contains(strings.ToLower(mem), "teal") {
			t.Errorf("filter leaked memory %q", mem)
		}
	}

	tooMany := make([]string, 51)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("bucket-%d", i)
	}
	code, body = doV5(t, h, "POST", "/ns/a/profile", map[string]any{"buckets": tooMany})
	if code != http.StatusBadRequest {
		t.Fatalf("51 buckets status = %d", code)
	}
	if item := errorItem(t, body); item["code"] != "too_big" {
		t.Errorf("code = %v", item["code"])
	}

	code, _ = doV5(t, h, "GET", "/ns/a/profile", nil)
	if code != http.StatusNotFound {
		t.Errorf("GET profile status = %d, want 404", code)
	}
}

func TestV5Buckets(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	seedListData(t, h)

	code, body := doV5(t, h, "GET", "/ns/a/profile/buckets", nil)
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	buckets := body["buckets"].(map[string]any)
	for _, key := range []string{"preferences", "tech_stack"} {
		if v, _ := buckets[key].(string); v == "" {
			t.Errorf("bucket %q missing or empty", key)
		}
	}

	code, body = doV5(t, h, "PUT", "/ns/a/profile/buckets", map[string]any{
		"buckets": map[string]any{"audit_b": "Beschreibung"},
	})
	if code != http.StatusOK {
		t.Fatalf("put status = %d", code)
	}
	if body["buckets"].(map[string]any)["audit_b"] != "Beschreibung" {
		t.Errorf("put response = %v", body)
	}

	_, body = doV5(t, h, "GET", "/ns/a/profile/buckets", nil)
	buckets = body["buckets"].(map[string]any)
	if buckets["audit_b"] != "Beschreibung" {
		t.Errorf("stored bucket missing")
	}
	if buckets["preferences"] == nil {
		t.Errorf("defaults not merged")
	}

	code, body = doV5(t, h, "PUT", "/ns/a/profile/buckets", map[string]any{
		"buckets": map[string]any{"preferences": "x"},
	})
	if code != http.StatusConflict {
		t.Fatalf("protected put status = %d", code)
	}
	if body["error"] == nil || body["error"] == "" {
		t.Errorf("expected error message")
	}

	code, _ = doV5(t, h, "PUT", "/ns/a/profile/buckets", map[string]any{
		"buckets": map[string]any{"Bad Key": "x"},
	})
	if code != http.StatusBadRequest {
		t.Errorf("invalid key status = %d", code)
	}

	code, _ = doV5(t, h, "PUT", "/ns/a/profile/buckets", map[string]any{
		"buckets": map[string]any{"audit_b": 123},
	})
	if code != http.StatusBadRequest {
		t.Errorf("numeric value status = %d", code)
	}

	code, _ = doV5(t, h, "PUT", "/ns/nope/profile/buckets", map[string]any{
		"buckets": map[string]any{"audit_b": "x"},
	})
	if code != http.StatusNotFound {
		t.Errorf("unknown namespace status = %d", code)
	}

	code, body = doV5(t, h, "DELETE", "/ns/a/profile/buckets", map[string]any{
		"buckets": []string{"audit_b"},
	})
	if code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
	if _, ok := body["buckets"].(map[string]any)["audit_b"]; ok {
		t.Errorf("bucket not deleted")
	}

	code, _ = doV5(t, h, "DELETE", "/ns/a/profile/buckets", map[string]any{
		"buckets": []string{"preferences"},
	})
	if code != http.StatusConflict {
		t.Errorf("protected delete status = %d", code)
	}

	keys := make([]string, 51)
	for i := range keys {
		keys[i] = fmt.Sprintf("bucket-%d", i)
	}
	code, _ = doV5(t, h, "DELETE", "/ns/a/profile/buckets", map[string]any{"buckets": keys})
	if code != http.StatusBadRequest {
		t.Errorf("51 deletes status = %d", code)
	}
}

func TestV5BucketWritesTouchNamespace(t *testing.T) {
	h, cleanup := newV5Handler(t)
	defer cleanup()
	createNamespace(t, h, "a")

	_, before := doV5(t, h, "GET", "/ns/a", nil)
	beforeTS := before["system"].(map[string]any)["updatedAt"].(string)
	time.Sleep(10 * time.Millisecond)

	code, _ := doV5(t, h, "PUT", "/ns/a/profile/buckets", map[string]any{"buckets": map[string]any{"audit_b": "desc"}})
	if code != http.StatusOK {
		t.Fatalf("put status = %d", code)
	}
	_, after := doV5(t, h, "GET", "/ns/a", nil)
	afterTS := after["system"].(map[string]any)["updatedAt"].(string)
	if afterTS <= beforeTS {
		t.Errorf("updatedAt did not advance: %q -> %q", beforeTS, afterTS)
	}

	for _, body := range []map[string]any{
		{"buckets": nil},
	} {
		code, out := doV5(t, h, "PUT", "/ns/a/profile/buckets", body)
		if code != http.StatusBadRequest {
			t.Fatalf("null buckets status = %d, body %v", code, out)
		}
	}
	code, out := doV5(t, h, "DELETE", "/ns/a/profile/buckets", map[string]any{"buckets": nil})
	if code != http.StatusBadRequest {
		t.Fatalf("null delete status = %d, body %v", code, out)
	}
}
