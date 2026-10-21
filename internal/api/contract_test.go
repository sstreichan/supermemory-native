//go:build contract

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func contractBaseURL() string {
	if u := os.Getenv("SM_TARGET_URL"); u != "" {
		return u
	}
	return "https://api.supermemory.ai"
}

func contractAPIKey(t *testing.T) string {
	key := os.Getenv("SUPERMEMORY_API_KEY")
	if key == "" {
		t.Skip("SUPERMEMORY_API_KEY not set — skipping live contract test")
	}
	return key
}

func contractContainer() string {
	return fmt.Sprintf("go-contract-%d", os.Getpid())
}

func contractCall(t *testing.T, method, path string, body interface{}) ([]byte, int) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		buf := new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = buf
	}
	req, _ := http.NewRequest(method, contractBaseURL()+path, reader)
	req.Header.Set("Authorization", "Bearer "+contractAPIKey(t))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HTTP error: %v", err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	return out.Bytes(), resp.StatusCode
}

func TestContract_AddDocument(t *testing.T) {
	body := map[string]interface{}{
		"content":      "My favorite color is teal",
		"containerTag": contractContainer(),
	}
	resp, status := contractCall(t, "POST", "/v3/documents", body)
	if status != 200 {
		t.Fatalf("add: expected 200, got %d. Body: %s", status, resp)
	}
	var result map[string]interface{}
	json.Unmarshal(resp, &result)
	if result["id"] == nil || result["id"] == "" {
		t.Fatal("add: response should contain non-empty id")
	}
	t.Logf("add response: %s", resp)
}

func TestContract_SearchMemories(t *testing.T) {
	tag := contractContainer()
	contractCall(t, "POST", "/v3/documents", map[string]interface{}{
		"content": "My favorite color is teal", "containerTag": tag,
	})
	resp, status := contractCall(t, "POST", "/v4/search", map[string]interface{}{
		"q": "favorite color", "containerTag": tag,
	})
	if status != 200 {
		t.Fatalf("search: expected 200, got %d. Body: %s", status, resp)
	}
	var result struct {
		Results []struct {
			ID         string  `json:"id"`
			Memory     string  `json:"memory"`
			Similarity float64 `json:"similarity"`
			UpdatedAt  string  `json:"updatedAt"`
		} `json:"results"`
		Total float64 `json:"total"`
	}
	json.Unmarshal(resp, &result)
	if len(result.Results) < 1 {
		t.Fatal("search: expected at least 1 result")
	}
	found := false
	for _, r := range result.Results {
		if r.Similarity <= 0 {
			t.Error("search: similarity should be > 0")
		}
		if r.UpdatedAt == "" {
			t.Error("search: updatedAt should be populated")
		}
		if bytes.Contains([]byte(r.Memory), []byte("teal")) {
			found = true
		}
	}
	if !found {
		t.Fatal("search: expected 'teal' in results")
	}
	t.Logf("search response: %s", resp)
}

func TestContract_Profile(t *testing.T) {
	tag := contractContainer()
	contractCall(t, "POST", "/v3/documents", map[string]interface{}{
		"content": "My favorite color is teal", "containerTag": tag,
	})
	resp, status := contractCall(t, "POST", "/v4/profile", map[string]interface{}{
		"q": "favorite color", "containerTag": tag,
	})
	if status != 200 {
		t.Fatalf("profile: expected 200, got %d. Body: %s", status, resp)
	}
	var result struct {
		Profile struct {
			Static  []string `json:"static"`
			Dynamic []string `json:"dynamic"`
		} `json:"profile"`
		SearchResults struct {
			Results []interface{} `json:"results"`
			Total   float64       `json:"total"`
		} `json:"searchResults"`
	}
	json.Unmarshal(resp, &result)
	allFacts := len(result.Profile.Static) + len(result.Profile.Dynamic)
	if allFacts < 1 {
		t.Fatal("profile: expected at least 1 static or dynamic fact")
	}
	t.Logf("profile response: %s", resp)
}

func TestContract_Forget(t *testing.T) {
	tag := contractContainer()
	resp, _ := contractCall(t, "POST", "/v3/documents", map[string]interface{}{
		"content": "temporary memory to forget", "containerTag": tag,
	})
	var addResult struct {
		ID string `json:"id"`
	}
	json.Unmarshal(resp, &addResult)
	memID := addResult.ID
	if memID == "" {
		t.Fatal("add: failed to get memory ID")
	}

	_, status := contractCall(t, "DELETE", "/v4/memories", map[string]interface{}{
		"containerTag": tag, "id": memID,
	})
	if status != 200 {
		t.Fatalf("forget: expected 200, got %d", status)
	}

	// Verify the memory is gone
	sresp, _ := contractCall(t, "POST", "/v4/search", map[string]interface{}{
		"q": "temporary memory", "containerTag": tag,
	})
	var searchResult struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	json.Unmarshal(sresp, &searchResult)
	for _, r := range searchResult.Results {
		if r.ID == memID {
			t.Fatal("forget: memory should no longer be in search results")
		}
	}
	t.Logf("forget verified: memory %s no longer searchable", memID)
}

// TestContractV5Lifecycle exercises the v5 namespace surface end to end. Set
// SM_TARGET_URL to run it against a local daemon (with SUPERMEMORY_API_KEY set
// to that daemon's key).
func TestContractV5Lifecycle(t *testing.T) {
	ns := contractContainer() + "-v5"
	id := fmt.Sprintf("v5-doc-%d", os.Getpid())
	marker := fmt.Sprintf("v5-contract-marker-%d", os.Getpid())

	t.Cleanup(func() {
		_, _ = contractCall(t, "DELETE", "/ns/"+ns, nil)
	})

	body, status := contractCall(t, "POST", "/ns/"+ns+"/document", map[string]interface{}{
		"id": id, "content": "The contract marker is " + marker + ".",
	})
	if status != 200 {
		t.Fatalf("create: expected 200, got %d: %s", status, body)
	}

	waitDone := func() {
		for attempt := 0; attempt < 20; attempt++ {
			resp, code := contractCall(t, "GET", "/ns/"+ns+"/document/"+id+"?include=chunks,memories", nil)
			if code == 200 {
				var doc struct {
					System struct {
						Status string `json:"status"`
					} `json:"system"`
				}
				_ = json.Unmarshal(resp, &doc)
				if doc.System.Status == "done" {
					return
				}
			}
			time.Sleep(3 * time.Second)
		}
		t.Fatalf("document did not reach status done")
	}
	waitDone()

	resp, code := contractCall(t, "POST", "/ns/"+ns+"/list/documents", map[string]interface{}{})
	if code != 200 {
		t.Fatalf("list: expected 200, got %d: %s", code, resp)
	}
	if !strings.Contains(string(resp), id) {
		t.Fatalf("list does not contain %s: %s", id, resp)
	}

	resp, code = contractCall(t, "POST", "/ns/"+ns+"/search", map[string]interface{}{"query": marker})
	if code != 200 || !strings.Contains(string(resp), marker) {
		t.Fatalf("search did not find marker: %d %s", code, resp)
	}

	updated := marker + " updated"
	_, code = contractCall(t, "PATCH", "/ns/"+ns+"/document/"+id, map[string]interface{}{"content": updated})
	if code != 200 {
		t.Fatalf("patch: expected 200, got %d", code)
	}
	waitDone()
	resp, code = contractCall(t, "GET", "/ns/"+ns+"/document/"+id, nil)
	if code != 200 || !strings.Contains(string(resp), updated) {
		t.Fatalf("patched content missing: %d %s", code, resp)
	}

	_, code = contractCall(t, "DELETE", "/ns/"+ns+"/document", map[string]interface{}{"ids": []string{id}})
	if code != 200 {
		t.Fatalf("delete document: expected 200, got %d", code)
	}
	_, code = contractCall(t, "GET", "/ns/"+ns+"/document/"+id, nil)
	if code != 404 {
		t.Fatalf("deleted document still readable: %d", code)
	}

	_, code = contractCall(t, "DELETE", "/ns/"+ns, nil)
	if code != 200 {
		t.Fatalf("delete namespace: expected 200, got %d", code)
	}
	_, code = contractCall(t, "GET", "/ns/"+ns, nil)
	if code != 404 {
		t.Fatalf("deleted namespace still readable: %d", code)
	}
}
