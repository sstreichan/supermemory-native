package api

import (
	"encoding/json"
	"net/http"

	"github.com/supermemory-native/supermemory-native/internal/memory"
)

type Handler struct {
	Engine *memory.Engine
}

func NewHandler(eng *memory.Engine) *Handler {
	return &Handler{Engine: eng}
}

type addRequest struct {
	Content      string `json:"content"`
	ContainerTag string `json:"containerTag"`
}

type queryRequest struct {
	Query        string `json:"q"`
	ContainerTag string `json:"containerTag"`
}

type deleteRequest struct {
	ContainerTag string `json:"containerTag"`
	ID           string `json:"id"`
	Content      string `json:"content"`
	Reason       string `json:"reason"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Preflight CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, x-supermemory-api-key, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" && r.Method != "DELETE" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
		return
	}

	switch r.URL.Path {
	case "/v3/documents":
		h.handleAddDocument(w, r)
	case "/v4/profile":
		h.handleProfileQuery(w, r)
	case "/v3/search", "/v4/search":
		h.handleSearchQuery(w, r)
	case "/v3/sync":
		h.handleSync(w, r)
	case "/v4/memories":
		h.handleDeleteMemory(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "endpoint not found"}`))
	}
}

func (h *Handler) handleAddDocument(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	id, err := h.Engine.AddMemory(req.Content, req.ContainerTag)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      id,
	})
}

func (h *Handler) handleProfileQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	// Semantic search with a low threshold to find relevant facts
	results, err := h.Engine.QueryMemories(req.Query, req.ContainerTag, 0.3, 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	// Format matching official Supermemory profile response
	dynamicFacts := make([]string, len(results))
	searchResults := make([]map[string]interface{}, len(results))

	for i, res := range results {
		dynamicFacts[i] = res.Memory.Content
		searchResults[i] = map[string]interface{}{
			"id":         res.Memory.ID,
			"memory":     res.Memory.Content,
			"similarity": res.Similarity,
			"updatedAt":  res.Memory.CreatedAt,
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"profile": map[string]interface{}{
			"static":  []string{},
			"dynamic": dynamicFacts,
		},
		"searchResults": map[string]interface{}{
			"results": searchResults,
			"total":   len(results),
		},
	})
}

func (h *Handler) handleSearchQuery(w http.ResponseWriter, r *http.Request) {
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}

	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	results, err := h.Engine.QueryMemories(req.Query, req.ContainerTag, 0.4, 10)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	searchResults := make([]map[string]interface{}, len(results))
	for i, res := range results {
		searchResults[i] = map[string]interface{}{
			"id":         res.Memory.ID,
			"memory":     res.Memory.Content,
			"similarity": res.Similarity,
			"updatedAt":  res.Memory.CreatedAt,
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"results": searchResults,
		"total":   len(results),
	})
}

func (h *Handler) handleSync(w http.ResponseWriter, r *http.Request) {
	if err := h.Engine.SyncVault(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Vault synchronization completed successfully",
	})
}

func (h *Handler) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	var req deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "failed to decode body"}`))
		return
	}
	if req.ContainerTag == "" {
		req.ContainerTag = "default"
	}

	// If no ID given, resolve by content within the container tag
	if req.ID == "" && req.Content != "" {
		results, err := h.Engine.QueryMemories(req.Content, req.ContainerTag, 0.99, 10)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
			return
		}
		for _, res := range results {
			if err := h.Engine.DeleteMemory(res.Memory.ID, req.ContainerTag); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
				return
			}
		}
	} else if req.ID != "" {
		if err := h.Engine.DeleteMemory(req.ID, req.ContainerTag); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "` + err.Error() + `"}`))
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
