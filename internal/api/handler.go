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

	// Serve the official local console dashboard on GET.
	// The welcome page (index.html) live injects /local-console.js which calls
	// the dashboard's API endpoints.
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(welcomePage)
			return
		case "/local-console.js":
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
			_, _ = w.Write(localConsoleJS)
			return
		case "/v3/container-tags/list":
			h.handleListContainerTags(w, r)
			return
		}
	}

	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error": "method not allowed"}`))
		return
	}

	switch r.URL.Path {
	case "/v3/config":
		h.handleConfig(w, r)
	case "/v3/documents":
		h.handleAddDocument(w, r)
	case "/v3/documents/documents":
		h.handleListDocuments(w, r)
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

func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
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

type containerTagRow struct {
	Name          string `json:"name"`
	ContainerTag  string `json:"containerTag"`
	DocumentCount int    `json:"documentCount"`
	MemoryCount   int    `json:"memoryCount"`
	Emoji         string `json:"emoji"`
}

func (h *Handler) handleListContainerTags(w http.ResponseWriter, _ *http.Request) {
	stats, err := h.Engine.ListContainerTags()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	rows := make([]containerTagRow, 0, len(stats))
	for _, s := range stats {
		rows = append(rows, containerTagRow{
			Name:          s.Name,
			ContainerTag:  s.ContainerTag,
			DocumentCount: s.DocumentCount,
			MemoryCount:   s.MemoryCount,
			Emoji:         "🧠",
		})
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(rows)
}

type listDocumentsRequest struct {
	Page  int    `json:"page"`
	Limit int    `json:"limit"`
	Sort  string `json:"sort"`
	Order string `json:"order"`
}

func (h *Handler) handleListDocuments(w http.ResponseWriter, r *http.Request) {
	var req listDocumentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "failed to decode body"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 25
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	page, err := h.Engine.ListDocuments(req.Page, req.Limit)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	docs := make([]map[string]interface{}, 0, len(page.Items))
	for _, d := range page.Items {
		entries := make([]map[string]interface{}, 0, len(d.MemoryEntries))
		for _, e := range d.MemoryEntries {
			entries = append(entries, map[string]interface{}{
				"id":        e.ID,
				"memory":    e.Memory,
				"createdAt": e.CreatedAt,
				"isStatic":  e.IsStatic,
			})
		}
		docs = append(docs, map[string]interface{}{
			"id":            d.ID,
			"title":         d.Title,
			"summary":       d.Summary,
			"status":        d.Status,
			"createdAt":     d.CreatedAt,
			"updatedAt":     d.UpdatedAt,
			"memories":      d.Memories,
			"memoryEntries": entries,
		})
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"documents": docs,
		"pagination": map[string]int{
			"totalItems":  page.TotalItems,
			"totalPages":  page.TotalPages,
			"currentPage": page.CurrentPage,
		},
	})
}
