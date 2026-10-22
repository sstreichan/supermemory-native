package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const bundleSizeGate = 50 * 1024 // Phase 8, Test 3: hard limit for local-console.js

func TestDashboardServed(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /: expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("GET /: expected text/html content type, got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "/local-console.js") {
		t.Error("GET /: index.html should reference /local-console.js")
	}
}

func TestDashboardJSAPIReferences(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/local-console.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /local-console.js: expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/javascript") {
		t.Errorf("GET /local-console.js: expected application/javascript, got %q", ct)
	}

	body := rr.Body.String()
	// Exact paths only: "/v3/documents" is a substring of "/v3/documents/documents".
	for _, path := range []string{
		"/v3/config",
		"/v3/documents",
		"/v3/documents/documents",
		"/v3/container-tags/list",
		"/v3/sync",
		"/v4/search",
		"/v4/profile",
		"/v4/memories",
	} {
		if !strings.Contains(body, path) {
			t.Errorf("bundle is missing API path %q", path)
		}
	}
}

// The dashboard must keep offering the v5 namespace surface: without these
// paths the third tab would render but silently do nothing.
func TestDashboardV5NamespacePaths(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/local-console.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /local-console.js: expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	for _, marker := range []string{
		"/ns/",
		"/namespaces",
		"/list/documents",
		"/document",
		"/document/file",
		"/search",
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("bundle is missing v5 path %q: the namespaces tab would be dead", marker)
		}
	}
}

func TestLocalConsoleBundleSize(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/local-console.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	if len(body) >= bundleSizeGate {
		t.Errorf("local-console.js is %d bytes, at or above the %d byte gate", len(body), bundleSizeGate)
	}
	// A bundle tree-shaken down to its IIFE wrapper is tiny but useless, so a
	// size-only gate is not enough: require real routing logic to be present.
	if !strings.Contains(body, "/v3/documents/documents") && !strings.Contains(body, "/v4/search") {
		t.Error("local-console.js looks empty: none of the expected API paths are present")
	}
}

// A browser reaching the daemon through a reverse proxy is not a loopback
// peer, so every console call is answered with 401 unless the bundle can
// present the API key. This guards the remote-dashboard fix: if the key
// handling is ever dropped from the bundle, the deployed dashboard breaks
// silently while all Go handler tests stay green.
func TestConsoleBundleCanAuthenticate(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/local-console.js", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	for _, marker := range []string{
		"x-supermemory-api-key", // header the daemon accepts
		"localStorage",          // where the key is kept between page loads
		"window.prompt",         // how the key is collected after a 401
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("bundle is missing %q: remote dashboards would get 401 on every call", marker)
		}
	}
}

// The dashboard shell must load without a key — a remote browser has no key
// until the page that asks for it arrived — while every API route stays
// gated. This is the state the captured HAR expects: document + bundle 200,
// API calls 401 until the console presents the key.
func TestDashboardShellReachableWithoutAPIKey(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()
	handler.apiKey = "sm_test_secret"

	// httptest defaults to a non-loopback RemoteAddr.
	for _, path := range []string{"/", "/local-console.js"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusOK {
			t.Errorf("GET %s without key: expected 200 so the console can load, got %d", path, rr.Code)
		}
	}

	for _, path := range []string{"/v3/container-tags/list"} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without key: expected 401, got %d", path, rr.Code)
		}
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("POST", "/v3/config", bytes.NewBufferString("{}")))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("POST /v3/config without key: expected 401, got %d", rr.Code)
	}
}

func TestContainerTagsList(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	seed := []struct {
		content string
		tag     string
	}{
		{"alpha one", "alpha"},
		{"alpha two", "alpha"},
		{"beta one", "beta"},
	}
	for _, s := range seed {
		if _, err := handler.Engine.AddMemory(s.content, s.tag); err != nil {
			t.Fatalf("seed add: %v", err)
		}
	}

	req := httptest.NewRequest("GET", "/v3/container-tags/list", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET /v3/container-tags/list: expected 200, got %d. Body: %s", rr.Code, rr.Body.String())
	}

	var tags []struct {
		Name          string `json:"name"`
		ContainerTag  string `json:"containerTag"`
		DocumentCount int    `json:"documentCount"`
		MemoryCount   int    `json:"memoryCount"`
		Emoji         string `json:"emoji"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &tags); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	counts := map[string]int{}
	for _, tag := range tags {
		if tag.ContainerTag == "" {
			t.Error("containerTag should not be empty")
		}
		if tag.Name != tag.ContainerTag {
			t.Errorf("name %q should match containerTag %q", tag.Name, tag.ContainerTag)
		}
		if tag.Emoji == "" {
			t.Errorf("emoji should be populated for %q", tag.ContainerTag)
		}
		counts[tag.ContainerTag] = tag.MemoryCount
	}

	if counts["alpha"] != 2 {
		t.Errorf("alpha: expected 2 memories, got %d", counts["alpha"])
	}
	if counts["beta"] != 1 {
		t.Errorf("beta: expected 1 memory, got %d", counts["beta"])
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 container tags, got %d", len(tags))
	}
}

func TestDocumentsPagination(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		if _, err := handler.Engine.AddMemory("pagination memory number", "paging"); err != nil {
			t.Fatalf("seed add: %v", err)
		}
	}

	type documentsResponse struct {
		Documents []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			Summary       string `json:"summary"`
			ContainerTag  string `json:"containerTag"`
			MemoryEntries []struct {
				ID        string `json:"id"`
				Memory    string `json:"memory"`
				CreatedAt string `json:"createdAt"`
				IsStatic  bool   `json:"isStatic"`
			} `json:"memoryEntries"`
		} `json:"documents"`
		Pagination struct {
			TotalItems  int `json:"totalItems"`
			TotalPages  int `json:"totalPages"`
			CurrentPage int `json:"currentPage"`
		} `json:"pagination"`
	}

	fetch := func(page, limit int) documentsResponse {
		payload, _ := json.Marshal(listDocumentsRequest{Page: page, Limit: limit, Sort: "createdAt", Order: "desc"})
		req := httptest.NewRequest("POST", "/v3/documents/documents", bytes.NewBuffer(payload))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("page %d: expected 200, got %d. Body: %s", page, rr.Code, rr.Body.String())
		}
		var resp documentsResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("page %d: decode: %v", page, err)
		}
		return resp
	}

	first := fetch(1, 2)
	if len(first.Documents) != 2 {
		t.Errorf("page 1: expected 2 documents, got %d", len(first.Documents))
	}
	if first.Pagination.TotalItems != 5 {
		t.Errorf("page 1: expected totalItems 5, got %d", first.Pagination.TotalItems)
	}
	if first.Pagination.TotalPages != 3 {
		t.Errorf("page 1: expected totalPages 3, got %d", first.Pagination.TotalPages)
	}
	if first.Pagination.CurrentPage != 1 {
		t.Errorf("page 1: expected currentPage 1, got %d", first.Pagination.CurrentPage)
	}
	for _, doc := range first.Documents {
		if doc.ID == "" {
			t.Error("document id should not be empty")
		}
		if doc.Title == "" {
			t.Error("document title should not be empty")
		}
		if len(doc.MemoryEntries) != 1 {
			t.Fatalf("expected 1 memory entry, got %d", len(doc.MemoryEntries))
		}
		if doc.MemoryEntries[0].Memory == "" {
			t.Error("memory entry content should not be empty")
		}
	}

	last := fetch(3, 2)
	if len(last.Documents) != 1 {
		t.Errorf("page 3: expected 1 document, got %d", len(last.Documents))
	}
	if last.Pagination.CurrentPage != 3 {
		t.Errorf("page 3: expected currentPage 3, got %d", last.Pagination.CurrentPage)
	}
}

func TestConfigEndpoint(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest("POST", "/v3/config", bytes.NewBuffer([]byte(`{}`)))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("POST /v3/config: expected 200, got %d", rr.Code)
	}

	var resp struct {
		Success bool `json:"success"`
		Port    int  `json:"port"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Error("expected success to be true")
	}
	if resp.Port <= 0 || resp.Port > 65535 {
		t.Errorf("expected a valid port, got %d", resp.Port)
	}

	// The handler must report the port it was constructed with.
	handler.port = 7001
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest("POST", "/v3/config", bytes.NewBuffer([]byte(`{}`))))
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Port != 7001 {
		t.Errorf("expected port 7001, got %d", resp.Port)
	}
}

func TestGetOnApiRoutesStill405(t *testing.T) {
	handler, cleanup := setupTestHandler(t)
	defer cleanup()

	// GET on data routes must stay 405, not 404 and not 200.
	for _, path := range []string{"/v3/documents", "/v4/search", "/v4/profile", "/v3/sync", "/v4/memories"} {
		req := httptest.NewRequest("GET", path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: expected 405, got %d", path, rr.Code)
		}
	}

	// The container-tag list is the one API route that legitimately allows GET.
	req := httptest.NewRequest("GET", "/v3/container-tags/list", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /v3/container-tags/list: expected 200, got %d", rr.Code)
	}
}
